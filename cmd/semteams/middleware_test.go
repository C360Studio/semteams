package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/service"
)

func TestSanitiseIdentity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "happy path", in: "alice@example.com", want: "alice@example.com"},
		{name: "trims whitespace", in: "  bob  ", want: "bob"},
		{name: "rejects empty", in: "", want: ""},
		{name: "rejects whitespace only", in: "   ", want: ""},
		{name: "rejects newline", in: "alice\nbob", want: ""},
		{name: "rejects carriage return", in: "alice\rbob", want: ""},
		{name: "rejects tab", in: "alice\tbob", want: ""},
		{name: "rejects null", in: "alice\x00bob", want: ""},
		{name: "rejects high bytes", in: "alice\xffbob", want: ""},
		{name: "rejects unicode", in: "alíce", want: ""},
		{name: "accepts at and dot", in: "user@host.local", want: "user@host.local"},
		{name: "accepts uuid", in: "00000000-0000-0000-0000-000000000000", want: "00000000-0000-0000-0000-000000000000"},
		{name: "rejects > 256 bytes", in: longString(257), want: ""},
		{name: "accepts exactly 256 bytes", in: longString(256), want: longString(256)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitiseIdentity(tc.in); got != tc.want {
				t.Fatalf("sanitiseIdentity(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// TestXUserIDIdentityMiddleware_SetsCtxFromHeader asserts the canonical
// path: a clean X-User-Id header lands on ctx via agenticdispatch.WithIdentity
// so a downstream IdentityFromRequest call resolves to the header value.
func TestXUserIDIdentityMiddleware_SetsCtxFromHeader(t *testing.T) {
	var observed string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = agenticdispatch.IdentityFromRequest(r, "")
	})
	wrapped := xUserIDIdentityMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/teams-dispatch/loops/x/approval", nil)
	req.Header.Set(xUserIDHeader, "alice@example.com")
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if observed != "alice@example.com" {
		t.Fatalf("downstream identity = %q, want %q", observed, "alice@example.com")
	}
}

// TestXUserIDIdentityMiddleware_FallsThroughOnAbsent asserts the
// no-header path degrades to body / default — the middleware does not
// poison ctx with an empty value.
func TestXUserIDIdentityMiddleware_FallsThroughOnAbsent(t *testing.T) {
	var observed string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = agenticdispatch.IdentityFromRequest(r, "body-fallback")
	})
	wrapped := xUserIDIdentityMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/teams-dispatch/loops/x/approval", nil)
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if observed != "body-fallback" {
		t.Fatalf("downstream identity = %q, want body fallback to win", observed)
	}
}

// TestXUserIDIdentityMiddleware_RejectsControlChars asserts a header
// containing a control character is treated the same as absent — ctx
// stays clean and the body / default wins downstream.
func TestXUserIDIdentityMiddleware_RejectsControlChars(t *testing.T) {
	var observed string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = agenticdispatch.IdentityFromRequest(r, "body-fallback")
	})
	wrapped := xUserIDIdentityMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/teams-dispatch/loops/x/approval", nil)
	req.Header.Set(xUserIDHeader, "alice\nbob")
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if observed != "body-fallback" {
		t.Fatalf("downstream identity = %q, want body fallback (control char rejected)", observed)
	}
}

// TestMainWiringSmoke is the canary for the single line in main.go that
// passes productMiddleware() into the framework. Exercising the same
// shape — service.Manager.UseHTTPMiddleware(productMiddleware()...) —
// without booting the real binary fails to compile (and the test fails
// to build) if either the framework method signature changes or
// productMiddleware()'s element type drifts. A future contributor who
// deletes the wiring line in main.go will see this test still pass —
// that's the limitation of a unit-level canary. Pair with the upstream
// service/middleware_test.go suite for full coverage of UseHTTPMiddleware
// semantics; an end-to-end test that asserts the binary actually calls
// it is deferred to Playwright.
func TestMainWiringSmoke(_ *testing.T) {
	m := service.NewServiceManager(service.NewServiceRegistry())
	m.UseHTTPMiddleware(productMiddleware(nil, slog.Default())...)
	// No assertion: compile-time success is the contract. If
	// productMiddleware() drifts to an incompatible element type, this
	// test fails to build.
}

// TestProductMiddleware_OrderedOutermostFirst pins the chain shape so
// future additions don't reshuffle the identity middleware's slot
// without an explicit decision.
func TestProductMiddleware_OrderedOutermostFirst(t *testing.T) {
	chain := productMiddleware(nil, slog.Default())
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2 (identity + test_harness)", len(chain))
	}
	// Identity check via behaviour: the FIRST entry must lift the header
	// so downstream middleware (and handlers) see the resolved identity.
	var observed string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = agenticdispatch.IdentityFromRequest(r, "")
	})
	wrapped := chain[0](inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(xUserIDHeader, "carol")
	wrapped.ServeHTTP(httptest.NewRecorder(), req)
	if observed != "carol" {
		t.Fatalf("chain[0] is not the identity middleware: observed = %q", observed)
	}
	// Test harness middleware is index 1 — pass-through when manager is nil
	// (verified by the chain accepting a nil manager without panicking
	// during the identity probe above).
}
