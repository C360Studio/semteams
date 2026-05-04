package main

import (
	"log/slog"
	"net/http"
	"strings"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semteams/cmd/semteams/harness"
)

// xUserIDHeader is the product-shell convention for naming the
// authenticated caller in HTTP requests. Today it is unauthenticated —
// any client can send any value. Real auth (OAuth / JWT / mTLS) belongs
// in a follow-on middleware that runs OUTSIDE this one and overwrites
// or rejects the header before the framework consumes it. ADR-030 Phase 3
// upstream tracks the bypass-token threat model.
//
// Go's net/textproto canonicalises incoming header names, so
// `X-User-Id`, `x-user-id`, and `X-USER-ID` all map to the same key
// here — no manual case fold needed.
const xUserIDHeader = "X-User-Id"

// xUserIDIdentityMiddleware lifts X-User-Id into the agentic-dispatch
// identity context so beta.22's IdentityFromRequest helper resolves it
// before the body fallback. When the header is absent, the request
// passes through unchanged and downstream handlers fall back to the
// body's user_id field or the framework default ("http-user").
//
// Header values are sanitised — any control character or whitespace
// outside the printable ASCII range marks the header as untrusted and
// the request degrades to no-ctx (same as absent header). This is a
// cheap defence against header smuggling; real auth still belongs
// outside this seam.
//
// Multiple X-User-Id headers: Go's Header.Get returns the first value;
// any reverse proxy or auth layer that needs last-wins semantics must
// normalise upstream of this middleware.
func xUserIDIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get(xUserIDHeader)
		if id := sanitiseIdentity(raw); id != "" {
			r = r.WithContext(agenticdispatch.WithIdentity(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

// sanitiseIdentity returns the trimmed header value if every byte is
// printable ASCII (0x20–0x7E) and the trimmed length is > 0; otherwise
// the empty string. Length-capped at 256 to bound downstream log lines
// and metric label cardinality.
//
// Byte-wise scan is intentional: printable ASCII is single-byte, and
// any multibyte UTF-8 has bytes ≥ 0x80 which are already rejected.
// Don't "fix" this to `for _, r := range v` — runes hide the
// multibyte-rejection invariant.
func sanitiseIdentity(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > 256 {
		return ""
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7E {
			return ""
		}
	}
	return v
}

// productMiddleware returns the ordered chain semteams applies to
// every framework HTTP route. Outermost-first per service.HTTPMiddleware
// contract: index 0 sees the request first.
//
// Order:
//
//  1. xUserIDIdentityMiddleware — lift X-User-Id into ctx so downstream
//     handlers see the caller. Runs first so identity is bound before
//     any product handler that wants to log it.
//  2. harness HTTP middleware — intercepts /harnesses* GETs and serves
//     the operator/UI catalog read API. Pass-through for all other
//     paths. Nil-safe: a deployment without a harness manager (boot-
//     time KV failure) skips the intercept transparently.
//
// When extending:
//
//   - To add panic recovery: prepend (index 0) so panics in any later
//     middleware are caught.
//   - To add request logging that observes the resolved identity:
//     append after xUserIDIdentityMiddleware so ctx already carries
//     the identity by the time logging runs.
//   - To add real auth (OAuth/JWT/mTLS): prepend the authenticator
//     and have it overwrite the X-User-Id header (or call
//     agenticdispatch.WithIdentity directly). xUserIDIdentityMiddleware
//     trusts whatever header reaches it.
func productMiddleware(harnessMgr *harness.Manager, logger *slog.Logger) []service.HTTPMiddleware {
	return []service.HTTPMiddleware{
		xUserIDIdentityMiddleware,
		harness.HTTPMiddleware(harnessMgr, logger),
	}
}
