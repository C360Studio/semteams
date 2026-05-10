package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// taskMutex serialises operations on a single task's workspace.
// Per ADR-032 Decision #3 (R3.6.1.1 conformance: chainID → taskID for
// upstream wire alignment) we use a per-task mutex map even though
// today's contention is ~zero — costs ~10 lines and dodges "what if
// we add concurrent tasks later" cleanly.
type taskMutex struct {
	mu sync.Mutex
}

// workspaceMeta holds per-task workspace metadata. Today: only
// read_only_paths. In-memory only; on sandbox restart, on-disk chmod
// is the source of truth so lost metadata doesn't reduce enforcement
// (already-frozen paths stay frozen at the filesystem layer).
type workspaceMeta struct {
	ReadOnlyPaths []string
}

// ServerConfig groups the knobs NewServer takes.
type ServerConfig struct {
	WorkspaceRoot      string
	MaxFileSize        int64
	MaxReadBytes       int
	MaxOutputBytes     int
	DefaultExecTimeout time.Duration
	MaxExecTimeout     time.Duration
	// DockerMode is the resolved Docker access mode. Defaults to
	// dockerModeNone when zero-valued. The request-path consults it
	// when a future harness or tool gate needs to refuse a chain that
	// requires testcontainer execution under mode=none; today no such
	// gate exists, but the field is plumbed so adding one doesn't
	// require widening the constructor surface. Also feeds ops-facing
	// diagnostics and the future DinD lifecycle hook (R3.7.2.d′-dind).
	DockerMode dockerMode
	Logger     *slog.Logger
}

// Server handles sandbox HTTP API requests.
type Server struct {
	workspaceRoot      string
	maxFileSize        int64
	maxReadBytes       int
	maxOutputBytes     int
	defaultExecTimeout time.Duration
	maxExecTimeout     time.Duration
	dockerMode         dockerMode
	logger             *slog.Logger

	mu          sync.Mutex
	taskMutexes map[string]*taskMutex
	taskMeta    map[string]*workspaceMeta
}

// NewServer constructs a Server from cfg. WorkspaceRoot must exist
// before handlers are called; main.go ensures this on boot.
//
// MaxFileSize caps the content size accepted by write_file. MaxReadBytes
// caps the content returned by read_file (larger files are truncated;
// the response includes a flag so callers know the file was truncated).
// MaxOutputBytes caps stdout/stderr captured per exec call.
// DefaultExecTimeout / MaxExecTimeout bound exec request timeouts.
//
// Zero-valued cfg fields fall back to safe defaults so test harnesses
// that don't exercise a particular endpoint can leave its knobs unset
// without silently breaking.
func NewServer(cfg ServerConfig) *Server {
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 1 * 1024 * 1024
	}
	if cfg.MaxReadBytes == 0 {
		cfg.MaxReadBytes = 256 * 1024
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 100 * 1024
	}
	if cfg.DefaultExecTimeout == 0 {
		cfg.DefaultExecTimeout = 30 * time.Second
	}
	if cfg.MaxExecTimeout == 0 {
		cfg.MaxExecTimeout = 5 * time.Minute
	}
	if cfg.DockerMode == "" {
		cfg.DockerMode = dockerModeNone
	}
	return &Server{
		workspaceRoot:      cfg.WorkspaceRoot,
		maxFileSize:        cfg.MaxFileSize,
		maxReadBytes:       cfg.MaxReadBytes,
		maxOutputBytes:     cfg.MaxOutputBytes,
		defaultExecTimeout: cfg.DefaultExecTimeout,
		maxExecTimeout:     cfg.MaxExecTimeout,
		dockerMode:         cfg.DockerMode,
		logger:             cfg.Logger,
		taskMutexes:        make(map[string]*taskMutex),
		taskMeta:           make(map[string]*workspaceMeta),
	}
}

// RegisterRoutes wires the sandbox HTTP API onto mux. Endpoints conform
// to the upstream sandbox.Client wire format (semstreams beta.36) so
// upstream BashExecutor reaches our sandbox via SANDBOX_URL without any
// adapter layer. The single product-shell extension is
// `GET /worktree/{taskID}/archive` for forensics zip (ADR-032 §10);
// upstream sandbox.Client doesn't try to use it.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /worktree", s.handleCreateWorktree)
	mux.HandleFunc("DELETE /worktree/{taskID}", s.handleDeleteWorktree)
	mux.HandleFunc("POST /exec", s.handleExec)
	mux.HandleFunc("PUT /file", s.handleWriteFile)
	mux.HandleFunc("GET /file", s.handleReadFile)
	mux.HandleFunc("GET /worktree/{taskID}/archive", s.handleZipWorkspace)
}

// =============================================================================
// REQUEST / RESPONSE TYPES
// =============================================================================

// createWorktreeRequest mirrors upstream sandbox.Client.CreateWorktree
// body shape. The read_only_paths extension (ADR-032 §addendum
// 2026-05-03) is deliberately additive: upstream's current client omits
// the field; future upstream PR will add it pass-through.
type createWorktreeRequest struct {
	TaskID        string   `json:"task_id"`
	ReadOnlyPaths []string `json:"read_only_paths,omitempty"`
}

// worktreeInfo is the response shape for POST /worktree, mirroring
// upstream sandbox.Client.WorktreeInfo (Status/Path/Branch).
type worktreeInfo struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// fileWriteRequest mirrors upstream sandbox.Client.WriteFile body shape.
type fileWriteRequest struct {
	TaskID  string `json:"task_id"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// fileReadResponse extends upstream's {content} with size/truncation
// hints so callers can tell when a large file was clipped. Upstream's
// json decoder ignores unknown fields, so the extension is wire-safe.
type fileReadResponse struct {
	Content   string `json:"content"`
	Size      int    `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
	TotalSize int64  `json:"total_size,omitempty"`
}

// execRequest mirrors upstream sandbox.Client.Exec body shape.
type execRequest struct {
	TaskID    string `json:"task_id"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// execResponse mirrors upstream sandbox.Client.ExecResult.
type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

// =============================================================================
// HANDLERS
// =============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateWorktree(w http.ResponseWriter, r *http.Request) {
	var req createWorktreeRequest
	if err := readJSON(r, &req, 64*1024); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isValidTaskID(req.TaskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	for _, p := range req.ReadOnlyPaths {
		if err := validateReadOnlyPath(p); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	tm := s.getTaskMutex(req.TaskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	created, err := s.createWorkspace(r.Context(), req.TaskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create worktree: %v", err))
		return
	}

	// Record / merge read_only_paths metadata. Re-create on an existing
	// workspace adds new paths to the existing set.
	s.mu.Lock()
	meta, ok := s.taskMeta[req.TaskID]
	if !ok {
		meta = &workspaceMeta{}
		s.taskMeta[req.TaskID] = meta
	}
	for _, p := range req.ReadOnlyPaths {
		clean := filepath.Clean(p)
		if !slices.Contains(meta.ReadOnlyPaths, clean) {
			meta.ReadOnlyPaths = append(meta.ReadOnlyPaths, clean)
		}
	}
	roPaths := slices.Clone(meta.ReadOnlyPaths)
	s.mu.Unlock()

	// Apply chmod sweep so any read_only_paths that already exist (e.g.
	// re-create after seed, or pre-populated workspace dir) are
	// immediately frozen. Intentionally runs regardless of `created` —
	// the caller might be re-creating an existing workspace to ADD
	// read_only_paths, or the workspace dir might have been pre-
	// populated out-of-band; both cases need the sweep.
	if err := s.applyReadOnlyChmod(req.TaskID, roPaths); err != nil {
		s.logger.Warn("read_only_paths chmod sweep had errors",
			"task_id", req.TaskID, "error", err)
	}

	status := "created"
	if !created {
		status = "exists"
	}
	s.logger.Info("worktree create",
		"task_id", req.TaskID,
		"status", status,
		"read_only_paths", len(roPaths))

	writeJSON(w, http.StatusOK, worktreeInfo{
		Status: status,
		Path:   filepath.Join(s.workspaceRoot, req.TaskID),
		Branch: defaultBranch,
	})
}

func (s *Server) handleDeleteWorktree(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if !isValidTaskID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}

	tm := s.getTaskMutex(taskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if err := s.removeWorkspace(taskID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete worktree: %v", err))
		return
	}
	s.mu.Lock()
	delete(s.taskMeta, taskID)
	s.mu.Unlock()

	s.logger.Info("worktree deleted", "task_id", taskID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleZipWorkspace(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if !isValidTaskID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}

	// Forward-flag (R3.6.3): zipDir streams directly under the task
	// mutex. A slow client draining a multi-MB archive blocks any
	// concurrent op on the same task. Acceptable today (zip is a
	// forensics op on a terminal task); once taskMutex graduates to
	// sync.RWMutex, switch to read-lock + buffer-then-stream.
	tm := s.getTaskMutex(taskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !s.workspaceExists(taskID) {
		writeError(w, http.StatusNotFound, "worktree not found")
		return
	}

	dir := filepath.Join(s.workspaceRoot, taskID)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, taskID+".zip"))
	if err := zipDir(w, dir, s.logger); err != nil {
		s.logger.Error("zip workspace failed", "task_id", taskID, "error", err)
	}
}

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	var req fileWriteRequest
	if err := readJSON(r, &req, s.maxFileSize*2+4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isValidTaskID(req.TaskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if int64(len(req.Content)) > s.maxFileSize {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("content exceeds max size (%d bytes)", s.maxFileSize))
		return
	}

	tm := s.getTaskMutex(req.TaskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !s.workspaceExists(req.TaskID) {
		writeError(w, http.StatusNotFound, "worktree not found")
		return
	}

	absPath, err := s.resolveTaskPath(req.TaskID, req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Lstat-before-write: refuse to write through an existing symlink
	// or onto a directory. Defense-in-depth on top of resolveTaskPath's
	// EvalSymlinks check; the file API never honors symlinks.
	if existing, statErr := os.Lstat(absPath); statErr == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			writeError(w, http.StatusForbidden, "refusing to write through symlink")
			return
		}
		if existing.IsDir() {
			writeError(w, http.StatusBadRequest, "path is a directory")
			return
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create parent dir: %v", err))
		return
	}
	if err := os.WriteFile(absPath, []byte(req.Content), 0o644); err != nil {
		// EACCES from os.WriteFile is the load-bearing signal that the
		// path is read-only (chmod'd by a prior read_only_paths sweep).
		// Map to 403 with a clear message so callers don't have to
		// re-derive it from a generic 500.
		if os.IsPermission(err) {
			writeError(w, http.StatusForbidden,
				fmt.Sprintf("path %q is read-only (read_only_paths)", req.Path))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write file: %v", err))
		return
	}

	// After-write chmod sweep: any read_only_paths that exist now (this
	// write may have just created files inside one) get frozen.
	roPaths := s.snapshotReadOnlyPaths(req.TaskID)
	if len(roPaths) > 0 {
		if sweepErr := s.applyReadOnlyChmod(req.TaskID, roPaths); sweepErr != nil {
			s.logger.Warn("read_only_paths chmod sweep had errors",
				"task_id", req.TaskID, "error", sweepErr)
		}
	}

	s.logger.Info("file written", "task_id", req.TaskID, "path", req.Path, "size", len(req.Content))
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "size": len(req.Content)})
}

func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	if !isValidTaskID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		writeError(w, http.StatusBadRequest, "path query param is required")
		return
	}

	tm := s.getTaskMutex(taskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !s.workspaceExists(taskID) {
		writeError(w, http.StatusNotFound, "worktree not found")
		return
	}

	absPath, err := s.resolveTaskPath(taskID, relPath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("stat: %v", err))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		writeError(w, http.StatusForbidden, "refusing to read through symlink")
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	totalSize := info.Size()
	data, err := os.ReadFile(absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read: %v", err))
		return
	}

	truncated := false
	if len(data) > s.maxReadBytes {
		data = data[:s.maxReadBytes]
		truncated = true
	}

	resp := fileReadResponse{
		Content: string(data),
		Size:    len(data),
	}
	if truncated {
		resp.Truncated = true
		resp.TotalSize = totalSize
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var req execRequest
	if err := readJSON(r, &req, 64*1024); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isValidTaskID(req.TaskID) {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	timeout := s.defaultExecTimeout
	if req.TimeoutMs > 0 {
		requested := time.Duration(req.TimeoutMs) * time.Millisecond
		if s.maxExecTimeout > 0 && requested > s.maxExecTimeout {
			requested = s.maxExecTimeout
		}
		timeout = requested
	}

	tm := s.getTaskMutex(req.TaskID)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !s.workspaceExists(req.TaskID) {
		writeError(w, http.StatusNotFound, "worktree not found")
		return
	}

	workDir := filepath.Join(s.workspaceRoot, req.TaskID)
	result := execCommand(r.Context(), workDir, req.Command, timeout, s.maxOutputBytes)

	// After-exec chmod sweep. A bash command that just created files
	// inside a read_only_path triggers this freeze; subsequent calls
	// hit EACCES naturally.
	roPaths := s.snapshotReadOnlyPaths(req.TaskID)
	if len(roPaths) > 0 {
		if sweepErr := s.applyReadOnlyChmod(req.TaskID, roPaths); sweepErr != nil {
			s.logger.Warn("read_only_paths chmod sweep had errors",
				"task_id", req.TaskID, "error", sweepErr)
		}
	}

	s.logger.Info("command executed",
		"task_id", req.TaskID,
		"command", truncateForLog(req.Command, 100),
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
	)
	writeJSON(w, http.StatusOK, result)
}

// truncateForLog bounds a string for log-line readability only.
// Distinct from cappedWriter (in exec.go) which buffers streaming
// output with hard memory bounds.
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// =============================================================================
// READ_ONLY_PATHS METADATA
// =============================================================================

// snapshotReadOnlyPaths returns a copy of the task's read_only_paths
// list. Returns nil for tasks with no metadata. Holding the result
// across the per-task mutex release is safe (slice is a copy).
func (s *Server) snapshotReadOnlyPaths(taskID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.taskMeta[taskID]
	if !ok || len(meta.ReadOnlyPaths) == 0 {
		return nil
	}
	return slices.Clone(meta.ReadOnlyPaths)
}

// validateReadOnlyPath rejects entries that would escape the workspace
// or aren't workspace-relative. Same shape as resolveTaskPath's
// validation but without needing taskID context (this runs at create
// time before the workspace exists).
func validateReadOnlyPath(p string) error {
	if p == "" {
		return fmt.Errorf("read_only_path entry is empty")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("read_only_path %q must be workspace-relative", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("read_only_path %q escapes workspace", p)
	}
	return nil
}

// =============================================================================
// VALIDATION
// =============================================================================

// isValidTaskID restricts the charset to alphanumerics, dot, hyphen,
// underscore — matches semdragon's quest IDs and upstream's task IDs.
// Defense-in-depth: explicitly reject "..", ".", and any embedded "..".
//
// Note: net/http's ServeMux already URL-decodes path values before
// PathValue, so URL-encoded bypass attempts (`%2F` → `/`, `%2E%2E` →
// `..`) reach this validator decoded and are rejected by the charset
// whitelist / dotdot check.
func isValidTaskID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return id != "." && id != ".." && !strings.Contains(id, "..")
}

// getTaskMutex returns the per-task mutex, creating it lazily.
//
// The map grows without bound; for R3.6.1's single-process demo with
// short-lived tasks this is irrelevant. R3.6.3 (production-readiness)
// can add ref-counted eviction. The struct wrapper around sync.Mutex
// makes a future swap to sync.RWMutex non-churn for callers.
func (s *Server) getTaskMutex(taskID string) *taskMutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	tm, ok := s.taskMutexes[taskID]
	if !ok {
		tm = &taskMutex{}
		s.taskMutexes[taskID] = tm
	}
	return tm
}

// =============================================================================
// JSON HELPERS
// =============================================================================

// errorResponse is the JSON shape returned on error.
type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// readJSON decodes the request body into v with a per-request size cap.
// Stricter caps than the actual content size surface as a confusing
// 400 instead of the explicit 413 at the size check; sizing the cap is
// the caller's responsibility.
func readJSON(r *http.Request, v any, maxBytes int64) error {
	defer r.Body.Close()
	if maxBytes <= 0 {
		maxBytes = 2 * 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	return json.Unmarshal(body, v)
}
