package chainpause

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
)

// HTTPHandler exposes POST /teams-loop/chain-pause/decide over HTTP.
// A new endpoint was chosen over composing with POST /teams-loop/loops/{id}/approval
// because the chain-pause decision shape (verb / failed_loop_id / reason) is
// structurally different from the tool-approval shape (approve / reject / modify
// / modified_arguments). Composing them via a message-kind discriminator would
// require forking the upstream handler at the product shell — more invasive than
// a sibling endpoint. ADR-037 §"Approval surface" option (b).
type HTTPHandler struct {
	decisions *DecisionHandler
	logger    *slog.Logger
}

// NewHTTPHandler constructs an HTTPHandler.
func NewHTTPHandler(decisions *DecisionHandler, logger *slog.Logger) *HTTPHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPHandler{decisions: decisions, logger: logger}
}

// Register mounts the chain-pause endpoint on the given prefix. The framework
// calls RegisterHTTPHandlers on components; the product shell calls this
// function explicitly from middleware.go after the component layer mounts.
// Prefix must end with "/" to match the framework's prefix convention.
//
// Registered path: POST {prefix}chain-pause/decide
func (h *HTTPHandler) Register(prefix string, mux *http.ServeMux) {
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	path := "POST " + prefix + "chain-pause/decide"
	mux.HandleFunc(path, h.handleDecide)
	h.logger.Debug("chain-pause HTTP handler registered",
		slog.String("path", path))
}

// handleDecide handles POST /teams-loop/chain-pause/decide.
// Identity resolves via X-User-Id header (ADR-030 xUserIDIdentityMiddleware),
// with body user_id as fallback, then "http-user" as default.
func (h *HTTPHandler) handleDecide(w http.ResponseWriter, r *http.Request) {
	var req DecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	if req.FailedLoopID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed_loop_id is required",
		})
		return
	}
	if req.Verb == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "verb is required (retry | kill | defer)",
		})
		return
	}

	// Resolve identity: ctx (set by xUserIDIdentityMiddleware) > body > default.
	actor := resolveActor(r, req.UserID)

	if err := h.decisions.HandleDecision(r.Context(), req, actor); err != nil {
		// Verb validation errors are 400; dispatch errors are 500.
		// Decision handler returns structured errors for bad verbs.
		status := http.StatusInternalServerError
		if isVerbError(err) {
			status = http.StatusBadRequest
		}
		h.logger.ErrorContext(r.Context(), "chain-pause: decision failed",
			slog.String("failed_loop_id", req.FailedLoopID),
			slog.String("verb", req.Verb),
			slog.Int("status", status),
			slog.String("error", err.Error()))
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	h.logger.InfoContext(r.Context(), "chain-pause: decision accepted",
		slog.String("failed_loop_id", req.FailedLoopID),
		slog.String("verb", req.Verb),
		slog.String("actor", actor))

	writeJSON(w, http.StatusOK, DecisionResponse{
		FailedLoopID: req.FailedLoopID,
		Verb:         req.Verb,
		Accepted:     true,
		Message:      "chain decision accepted",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	})
}

// isVerbError returns true if the error originates from verb validation in
// DecisionHandler.HandleDecision. Uses errors.Is against sentinel error values
// so the check is not brittle to error message rewording (R4 fix).
func isVerbError(err error) bool {
	return errors.Is(err, ErrInvalidVerb) || errors.Is(err, ErrReservedVerb)
}

// resolveActor reads the identity from the request context via
// agenticdispatch.IdentityFromRequest (ADR-030). The xUserIDIdentityMiddleware
// that runs before this handler has already lifted X-User-Id into the context
// via agenticdispatch.WithIdentity, so IdentityFromRequest reads from ctx
// first, then falls back to bodyUserID, then to "http-user" (N4 fix).
func resolveActor(r *http.Request, bodyUserID string) string {
	return agenticdispatch.IdentityFromRequest(r, bodyUserID)
}

// writeJSON writes a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
