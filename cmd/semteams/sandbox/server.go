package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// chainMutex serialises operations on a single chain's workspace.
// Per ADR-032 Decision #3 we use a per-chain mutex map even though
// today's contention is ~zero — costs ~10 lines of API and dodges
// "what if we add concurrent chains later" cleanly.
type chainMutex struct {
	mu sync.Mutex
}

// ServerConfig groups the knobs NewServer takes. Adding fields stays
// non-breaking for callers that build it positionally; tests can use
// zero-values for anything they don't exercise.
type ServerConfig struct {
	WorkspaceRoot      string
	MaxFileSize        int64
	MaxReadBytes       int
	MaxOutputBytes     int
	DefaultExecTimeout time.Duration
	MaxExecTimeout     time.Duration
	Logger             *slog.Logger
}

// Server handles sandbox HTTP API requests.
type Server struct {
	workspaceRoot      string
	maxFileSize        int64
	maxReadBytes       int
	maxOutputBytes     int
	defaultExecTimeout time.Duration
	maxExecTimeout     time.Duration
	logger             *slog.Logger

	mu           sync.Mutex
	chainMutexes map[string]*chainMutex
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
// without silently breaking (e.g., zero MaxOutputBytes would discard
// every byte of stdout).
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
	return &Server{
		workspaceRoot:      cfg.WorkspaceRoot,
		maxFileSize:        cfg.MaxFileSize,
		maxReadBytes:       cfg.MaxReadBytes,
		maxOutputBytes:     cfg.MaxOutputBytes,
		defaultExecTimeout: cfg.DefaultExecTimeout,
		maxExecTimeout:     cfg.MaxExecTimeout,
		logger:             cfg.Logger,
		chainMutexes:       make(map[string]*chainMutex),
	}
}

// RegisterRoutes wires the sandbox HTTP API onto mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /workspace/{chainID}", s.handleCreateWorkspace)
	mux.HandleFunc("GET /workspace/{chainID}", s.handleZipWorkspace)
	mux.HandleFunc("DELETE /workspace/{chainID}", s.handleDeleteWorkspace)
	mux.HandleFunc("PUT /workspace/{chainID}/files", s.handleWriteFile)
	mux.HandleFunc("GET /workspace/{chainID}/files", s.handleReadFile)
	mux.HandleFunc("POST /workspace/{chainID}/exec", s.handleExec)
}

// =============================================================================
// REQUEST / RESPONSE TYPES
// =============================================================================

// fileWriteRequest is the JSON body for PUT /workspace/{chainID}/files.
type fileWriteRequest struct {
	Path    string `json:"path"`    // Workspace-relative path; absolute or .. traversal rejected.
	Content string `json:"content"` // File content; size capped by maxFileSize.
}

// fileReadResponse is returned from GET /workspace/{chainID}/files.
type fileReadResponse struct {
	Content   string `json:"content"`
	Size      int    `json:"size"`                 // Bytes returned (after truncation).
	Truncated bool   `json:"truncated,omitempty"`  // True if file was larger than maxReadBytes.
	TotalSize int64  `json:"total_size,omitempty"` // Real file size on disk; only set when truncated.
}

// execRequest is the JSON body for POST /workspace/{chainID}/exec.
type execRequest struct {
	Command   string `json:"command"`              // Passed to `bash -c`.
	TimeoutMs int    `json:"timeout_ms,omitempty"` // 0 → DefaultExecTimeout; capped at MaxExecTimeout.
}

// execResponse is returned from POST /workspace/{chainID}/exec.
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

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	created, err := s.createWorkspace(chainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create workspace: %v", err))
		return
	}

	status := "created"
	if !created {
		status = "exists"
	}
	s.logger.Info("workspace create", "chain_id", chainID, "status", status)
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := s.removeWorkspace(chainID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete workspace: %v", err))
		return
	}

	s.logger.Info("workspace deleted", "chain_id", chainID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleZipWorkspace(w http.ResponseWriter, r *http.Request) {
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !s.workspaceExists(chainID) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	dir := filepath.Join(s.workspaceRoot, chainID)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, chainID+".zip"))
	if err := zipDir(w, dir); err != nil {
		// Headers already flushed; log and bail. Client sees a truncated
		// zip — a forensics-helper failure is not load-bearing.
		s.logger.Error("zip workspace failed", "chain_id", chainID, "error", err)
	}
}

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	// Body cap accounts for JSON-escape worst-case 2x expansion of
	// content (every byte a `\"` / `\n` / `\\`) plus envelope overhead.
	// Stricter caps surface as a confusing 400 instead of the explicit
	// 413 at the size check below.
	var req fileWriteRequest
	if err := readJSON(r, &req, s.maxFileSize*2+4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !s.workspaceExists(chainID) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	absPath, err := s.resolveChainPath(chainID, req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Lstat-before-write: refuse to write through an existing symlink
	// or onto a directory. Defense-in-depth — resolveChainPath's
	// EvalSymlinks check already catches symlinks pointing outside the
	// workspace, but a symlink whose target is *inside* the workspace
	// is still a footgun (agent could be tricked into clobbering the
	// real file via a planted alias). The file API never honors
	// symlinks; that invariant matters more once R3.6.1.c bash exec
	// can plant them.
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write file: %v", err))
		return
	}

	s.logger.Info("file written", "chain_id", chainID, "path", req.Path, "size", len(req.Content))
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "size": len(req.Content)})
}

func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		writeError(w, http.StatusBadRequest, "path query param is required")
		return
	}

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !s.workspaceExists(chainID) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	absPath, err := s.resolveChainPath(chainID, relPath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// lstat-not-stat: refuse to read through a symlink even if its target
	// resolves inside the workspace. Defense in depth — resolveChainPath's
	// EvalSymlinks check covers symlink-escapes, but a symlink-to-inside
	// is also a footgun the file API has no business honouring (the agent
	// should write through the real path, not chase a symlink it didn't
	// create).
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
	chainID := r.PathValue("chainID")
	if !isValidChainID(chainID) {
		writeError(w, http.StatusBadRequest, "invalid chain ID")
		return
	}

	var req execRequest
	if err := readJSON(r, &req, 64*1024); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	cm := s.getChainMutex(chainID)
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !s.workspaceExists(chainID) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	workDir := filepath.Join(s.workspaceRoot, chainID)
	result := execCommand(r.Context(), workDir, req.Command, timeout, s.maxOutputBytes)

	s.logger.Info("command executed",
		"chain_id", chainID,
		"command", truncateForLog(req.Command, 100),
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
	)
	writeJSON(w, http.StatusOK, result)
}

// truncateForLog bounds a string for log-line readability only.
// Distinct from cappedWriter (in exec.go) which buffers streaming
// output with hard memory bounds; this helper assumes its input is
// already a complete in-memory string.
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// =============================================================================
// VALIDATION
// =============================================================================

// isValidChainID restricts the charset to alphanumerics, dot, hyphen,
// underscore — the same set semdragon's quest IDs allow. Entity IDs in
// the framework use dot-delimited segments (e.g.
// "c360.prod.app.chain.abc123"), so dots must be permitted.
//
// Defense-in-depth: explicitly reject "..", ".", and any embedded "..".
// filepath.Join would normalise these later, but rejecting at the
// validator surfaces the error closer to the caller and removes one
// class of bypass attempts.
//
// Note: net/http's ServeMux already URL-decodes path values before
// passing to PathValue, so URL-encoded bypass attempts (`%2F` → `/`,
// `%2E%2E` → `..`) reach this validator in their decoded form and are
// rejected by the charset whitelist / dotdot check.
func isValidChainID(id string) bool {
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

// getChainMutex returns the per-chain mutex, creating it lazily.
// The outer s.mu is held only briefly to look up / insert.
//
// The map grows without bound; for R3.6.1's single-process demo with
// short-lived chains this is irrelevant. R3.6.3 (production-readiness)
// can add ref-counted eviction if needed. The chainMutex struct
// wrapper exists so a future swap to sync.RWMutex (e.g., to let zip
// proceed while exec holds a write lock — see ADR-032 R3.6.1.c notes)
// doesn't churn callers.
func (s *Server) getChainMutex(chainID string) *chainMutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	cm, ok := s.chainMutexes[chainID]
	if !ok {
		cm = &chainMutex{}
		s.chainMutexes[chainID] = cm
	}
	return cm
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
// The cap should be slightly larger than any expected payload (e.g.,
// content + JSON envelope overhead) — if the body exceeds it, decode
// fails and the caller should return 413.
func readJSON(r *http.Request, v any, maxBytes int64) error {
	defer r.Body.Close()
	if maxBytes <= 0 {
		maxBytes = 2 * 1024 * 1024 // 2 MB default
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	return json.Unmarshal(body, v)
}
