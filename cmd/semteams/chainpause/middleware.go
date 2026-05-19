package chainpause

import (
	"log/slog"
	"net/http"
	"strings"
)

// chainPauseDecidePath is the request path this middleware intercepts.
// The path is nested under the teams-loop component prefix so the operator
// UI can POST to the same base URL as the approval endpoint.
const chainPauseDecidePath = "/teams-loop/chain-pause/decide"

// HTTPMiddleware returns a middleware that intercepts POST requests to
// /teams-loop/chain-pause/decide and dispatches to the DecisionHandler.
// All other paths pass through to next unchanged.
//
// Middleware-style registration is the product shell's only HTTP-mux
// hook: the upstream service-manager owns the mux and exposes
// UseHTTPMiddleware as the surface for product-shell handlers.
//
// handler may be nil — middleware then passes all requests through
// (boot still succeeds; POST /teams-loop/chain-pause/decide returns 404).
func HTTPMiddleware(handler *HTTPHandler, _ *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if handler == nil {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chain-pause/decide") {
				next.ServeHTTP(w, r)
				return
			}
			handler.handleDecide(w, r)
		})
	}
}
