//go:build integration

package harness

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HTTPIntegrationSuite struct {
	suite.Suite
	testClient *natsclient.TestClient
	manager    *Manager
	wrapped    http.Handler
	ctx        context.Context
	cancel     context.CancelFunc
}

func (s *HTTPIntegrationSuite) SetupSuite() {
	s.testClient = natsclient.NewTestClient(s.T(),
		natsclient.WithJetStream(),
		natsclient.WithKV())
}

func (s *HTTPIntegrationSuite) SetupTest() {
	var err error
	s.manager, err = NewManager(s.testClient.Client)
	s.Require().NoError(err)
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)

	mw := HTTPMiddleware(s.manager, slog.Default())
	// "next" returns 418 so we can distinguish pass-through (handler-was-
	// reached) from middleware-served paths.
	s.wrapped = mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func (s *HTTPIntegrationSuite) TearDownTest() {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if keys, err := s.manager.kv.Keys(cleanupCtx); err == nil {
		for _, k := range keys {
			_ = s.manager.kv.Delete(cleanupCtx, k)
		}
	}
	s.cancel()
}

func TestHTTPIntegrationSuite(t *testing.T) {
	suite.Run(t, new(HTTPIntegrationSuite))
}

func (s *HTTPIntegrationSuite) TestList_Empty() {
	req := httptest.NewRequest(http.MethodGet, "/harnesses", nil)
	rr := httptest.NewRecorder()
	s.wrapped.ServeHTTP(rr, req)

	require.Equal(s.T(), http.StatusOK, rr.Code)
	body, _ := io.ReadAll(rr.Body)
	var got listResponse
	require.NoError(s.T(), json.Unmarshal(body, &got))
	require.Empty(s.T(), got.Harnesses)
}

func (s *HTTPIntegrationSuite) TestList_WithEntries() {
	require.NoError(s.T(), s.manager.Put(s.ctx, &Harness{
		Name:                "alpha",
		ComposeProfile:      "p",
		Image:               "i",
		SmokeContractSchema: "s.v1",
		DomainDescription:   "first",
	}))
	require.NoError(s.T(), s.manager.Put(s.ctx, &Harness{
		Name:                "bravo",
		ComposeProfile:      "p",
		Image:               "i",
		SmokeContractSchema: "s.v1",
		DomainDescription:   "second",
	}))

	req := httptest.NewRequest(http.MethodGet, "/harnesses", nil)
	rr := httptest.NewRecorder()
	s.wrapped.ServeHTTP(rr, req)

	require.Equal(s.T(), http.StatusOK, rr.Code)
	var got listResponse
	body, _ := io.ReadAll(rr.Body)
	require.NoError(s.T(), json.Unmarshal(body, &got))
	require.Len(s.T(), got.Harnesses, 2)
	require.Equal(s.T(), "alpha", got.Harnesses[0].Name)
	require.Equal(s.T(), "bravo", got.Harnesses[1].Name)
}

func (s *HTTPIntegrationSuite) TestGet_ByName() {
	require.NoError(s.T(), s.manager.Put(s.ctx, &Harness{
		Name:                "stub",
		ComposeProfile:      "harness-stub",
		Image:               "scratch",
		SmokeContractSchema: "stub.smoke_contract.v1",
		DomainDescription:   "test stub",
	}))

	req := httptest.NewRequest(http.MethodGet, "/harnesses/stub", nil)
	rr := httptest.NewRecorder()
	s.wrapped.ServeHTTP(rr, req)

	require.Equal(s.T(), http.StatusOK, rr.Code)
	var got Harness
	body, _ := io.ReadAll(rr.Body)
	require.NoError(s.T(), json.Unmarshal(body, &got))
	require.Equal(s.T(), "stub", got.Name)
	require.Equal(s.T(), "harness-stub", got.ComposeProfile)
}

func (s *HTTPIntegrationSuite) TestGet_NotFound() {
	req := httptest.NewRequest(http.MethodGet, "/harnesses/ghost", nil)
	rr := httptest.NewRecorder()
	s.wrapped.ServeHTTP(rr, req)

	require.Equal(s.T(), http.StatusNotFound, rr.Code)
}

func (s *HTTPIntegrationSuite) TestNonMatchingPath_PassesThrough() {
	req := httptest.NewRequest(http.MethodGet, "/teams-dispatch/loops", nil)
	rr := httptest.NewRecorder()
	s.wrapped.ServeHTTP(rr, req)

	require.Equal(s.T(), http.StatusTeapot, rr.Code, "non-/harnesses path should hit pass-through 418")
}
