package harness

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// requestTimeout caps how long the handler will wait for a List
// against the KV bucket. Operators / UIs polling /harnesses
// shouldn't ever hang the request thread.
const requestTimeout = 5 * time.Second

// HTTPMiddleware returns a middleware that intercepts requests
// for `/harnesses` and `/harnesses/{name}` and serves the catalog
// read API directly. All other paths pass through to next.
//
// Middleware-style registration is used (rather than mux
// HandleFunc) because semstreams' service-manager owns the chain
// HTTP mux internally — `Manager.UseHTTPMiddleware` is the only
// product-shell hook that wraps the framework's handler chain.
// This is foundational for any future product-shell endpoint
// (e.g. R3.7.4 candidate-promotion) that needs to live outside
// the /teams-dispatch/* component-owned namespace.
//
// Path is `/harnesses` — operator/UI-facing read-only, distinct
// from the chain-internal /teams-dispatch/* surface because the
// LLM consultation path is the persona-fragment auto-render (see
// ADR-033 §addendum), not an HTTP query.
//
// The Manager argument may be nil — middleware then short-
// circuits to `next` (catalog endpoint absent, but boot succeeds).
func HTTPMiddleware(mgr *Manager, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mgr == nil {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/harnesses") {
				next.ServeHTTP(w, r)
				return
			}
			path := r.URL.Path
			ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
			defer cancel()
			switch {
			case path == "/harnesses" || path == "/harnesses/":
				writeListJSON(ctx, mgr, w, logger)
			case strings.HasPrefix(path, "/harnesses/"):
				name := strings.TrimPrefix(path, "/harnesses/")
				if strings.Contains(name, "/") {
					next.ServeHTTP(w, r)
					return
				}
				writeGetJSON(ctx, mgr, name, w, logger)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// listResponse wraps the catalog so adding metadata fields later
// (timestamps, total counts, deployment id) is additive, not a
// shape break for clients that already parse the array directly.
type listResponse struct {
	Harnesses []*Harness `json:"harnesses"`
}

func writeListJSON(ctx context.Context, mgr *Manager, w http.ResponseWriter, logger *slog.Logger) {
	entries, err := mgr.List(ctx)
	if err != nil {
		logger.Warn("harness catalog HTTP: List failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "list catalog: "+err.Error())
		return
	}
	if entries == nil {
		entries = []*Harness{}
	}
	writeJSON(w, http.StatusOK, listResponse{Harnesses: entries})
}

func writeGetJSON(ctx context.Context, mgr *Manager, name string, w http.ResponseWriter, logger *slog.Logger) {
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name path parameter required")
		return
	}
	h, err := mgr.Get(ctx, name)
	if err != nil {
		// Distinguish "not found" from transport errors so operators see
		// the right status. Get wraps jetstream.ErrKeyNotFound when the
		// entry is missing.
		if isNotFound(err) {
			writeJSONError(w, http.StatusNotFound, "harness not found: "+name)
			return
		}
		logger.Warn("harness catalog HTTP: Get failed", "name", name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "get harness: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// errKeyNotFound is the sentinel used by tests that fake out the
// Manager surface; production paths flow through
// natsclient.IsKVNotFoundError when the underlying KV reports a
// missing key.
var errKeyNotFound = errors.New("key not found")

// isNotFound matches the jetstream.ErrKeyNotFound that natsclient
// surfaces via Manager.Get plus the test-only sentinel above.
func isNotFound(err error) bool {
	return err != nil && (errors.Is(err, errKeyNotFound) || natsclient.IsKVNotFoundError(err))
}
