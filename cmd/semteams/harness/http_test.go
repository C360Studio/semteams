package harness

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// nilManagerNext is the sentinel inner handler used to verify pass-
// through behaviour. Any request that reaches it will set the X-Pass
// header so the test can assert.
func nilManagerNext() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Pass", "1")
		w.WriteHeader(http.StatusTeapot)
	}
}

func TestHTTPMiddleware_NilManager_PassThrough(t *testing.T) {
	mw := HTTPMiddleware(nil, slog.Default())
	wrapped := mw(nilManagerNext())

	req := httptest.NewRequest(http.MethodGet, "/harnesses", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Header().Get("X-Pass") != "1" {
		t.Errorf("nil manager should pass through; status=%d", rr.Code)
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("expected pass-through status 418, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_NonGet_PassThrough(t *testing.T) {
	mw := HTTPMiddleware(&Manager{}, slog.Default())
	wrapped := mw(nilManagerNext())

	req := httptest.NewRequest(http.MethodPost, "/harnesses", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Header().Get("X-Pass") != "1" {
		t.Errorf("non-GET should pass through; status=%d", rr.Code)
	}
}

func TestHTTPMiddleware_OtherPath_PassThrough(t *testing.T) {
	mw := HTTPMiddleware(&Manager{}, slog.Default())
	wrapped := mw(nilManagerNext())

	for _, path := range []string{"/", "/teams-dispatch/loops", "/health", "/harness"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)
			if rr.Header().Get("X-Pass") != "1" {
				t.Errorf("path %q should pass through; status=%d", path, rr.Code)
			}
		})
	}
}

func TestHTTPMiddleware_NestedPath_PassThrough(t *testing.T) {
	// /harnesses/foo/bar has two slashes after /harnesses/ — middleware
	// shouldn't try to claim it as a Get-by-name request. Pass through
	// so the next handler (or 404) can deal with it.
	mw := HTTPMiddleware(&Manager{}, slog.Default())
	wrapped := mw(nilManagerNext())

	req := httptest.NewRequest(http.MethodGet, "/harnesses/foo/bar", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Header().Get("X-Pass") != "1" {
		t.Errorf("nested path should pass through; status=%d", rr.Code)
	}
}

// fakeListMgr is a Manager-shaped fake that lets us drive list outcomes
// without spinning up a NATS testcontainer. We can't satisfy *Manager
// directly (the type is concrete) so instead we exercise the wire-format
// helpers via the real Manager surface in the integration test;
// middleware unit tests cover the routing paths only.

func TestWriteJSONError_Shape(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONError(rr, http.StatusNotFound, "no such harness")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	body, _ := io.ReadAll(rr.Body)
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if got["error"] != "no such harness" {
		t.Errorf("error field: got %q, want %q", got["error"], "no such harness")
	}
}

func TestWriteJSON_ListResponseShape(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, listResponse{Harnesses: []*Harness{
		{
			Name:                "stub",
			ComposeProfile:      "harness-stub",
			Image:               "scratch",
			SmokeContractSchema: "stub.smoke_contract.v1",
			DomainDescription:   "stub for testing",
		},
	}})
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), `"harnesses"`) {
		t.Errorf("body missing 'harnesses' key: %s", body)
	}
	if !strings.Contains(string(body), `"name":"stub"`) {
		t.Errorf("body missing harness name: %s", body)
	}
}
