package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// chainMutex serialises operations on a single chain's workspace.
// Per ADR-032 Decision #3 we use a per-chain mutex map even though
// today's contention is ~zero — costs ~10 lines of API and dodges
// "what if we add concurrent chains later" cleanly.
type chainMutex struct {
	mu sync.Mutex
}

// Server handles sandbox HTTP API requests.
type Server struct {
	workspaceRoot string
	logger        *slog.Logger

	mu           sync.Mutex
	chainMutexes map[string]*chainMutex
}

// NewServer constructs a Server rooted at workspaceRoot. The directory
// must exist before handlers are called; main.go ensures this on boot.
func NewServer(workspaceRoot string, logger *slog.Logger) *Server {
	return &Server{
		workspaceRoot: workspaceRoot,
		logger:        logger,
		chainMutexes:  make(map[string]*chainMutex),
	}
}

// RegisterRoutes wires the sandbox HTTP API onto mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /workspace/{chainID}", s.handleCreateWorkspace)
	mux.HandleFunc("GET /workspace/{chainID}", s.handleZipWorkspace)
	mux.HandleFunc("DELETE /workspace/{chainID}", s.handleDeleteWorkspace)
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
