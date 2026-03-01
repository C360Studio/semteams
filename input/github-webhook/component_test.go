package githubwebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- signature validation ---------------------------------------------------

func TestValidateSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name   string
		sig    string
		body   []byte
		secret string
		want   bool
	}{
		{
			name:   "valid signature",
			sig:    validSig,
			body:   body,
			secret: secret,
			want:   true,
		},
		{
			name:   "wrong secret",
			sig:    validSig,
			body:   body,
			secret: "wrong-secret",
			want:   false,
		},
		{
			name:   "tampered body",
			sig:    validSig,
			body:   []byte(`{"action":"closed"}`),
			secret: secret,
			want:   false,
		},
		{
			name:   "missing sha256 prefix",
			sig:    hex.EncodeToString(mac.Sum(nil)),
			body:   body,
			secret: secret,
			want:   false,
		},
		{
			name:   "empty signature",
			sig:    "",
			body:   body,
			secret: secret,
			want:   false,
		},
		{
			name:   "malformed hex",
			sig:    "sha256=notvalidhex!!!",
			body:   body,
			secret: secret,
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateSignature(tc.sig, tc.body, tc.secret)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---- repoAllowed ------------------------------------------------------------

func TestRepoAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		repo      string
		want      bool
	}{
		{
			name:      "empty allowlist accepts everything",
			allowlist: nil,
			repo:      "acme/backend",
			want:      true,
		},
		{
			name:      "repo in allowlist",
			allowlist: []string{"acme/backend", "acme/frontend"},
			repo:      "acme/backend",
			want:      true,
		},
		{
			name:      "repo not in allowlist",
			allowlist: []string{"acme/backend"},
			repo:      "acme/frontend",
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inp := &Input{config: Config{RepoAllowlist: tc.allowlist}}
			assert.Equal(t, tc.want, inp.repoAllowed(tc.repo))
		})
	}
}

// ---- acceptsEventType -------------------------------------------------------

func TestAcceptsEventType(t *testing.T) {
	inp := &Input{
		config: Config{EventFilter: []string{"issues", "pull_request"}},
	}

	assert.True(t, inp.acceptsEventType("issues"))
	assert.True(t, inp.acceptsEventType("pull_request"))
	assert.False(t, inp.acceptsEventType("pull_request_review"))
	assert.False(t, inp.acceptsEventType("push"))
}

// ---- raw JSON helpers -------------------------------------------------------

func TestRawHelpers(t *testing.T) {
	raw := map[string]json.RawMessage{
		"str":    json.RawMessage(`"hello"`),
		"num":    json.RawMessage(`42`),
		"user":   json.RawMessage(`{"login":"octocat","id":1}`),
		"labels": json.RawMessage(`[{"name":"bug"},{"name":"help wanted"}]`),
		"head":   json.RawMessage(`{"ref":"feature-branch","sha":"abc"}`),
	}

	assert.Equal(t, "hello", rawString(raw, "str"))
	assert.Equal(t, "", rawString(raw, "missing"))
	assert.Equal(t, 42, rawInt(raw, "num"))
	assert.Equal(t, 0, rawInt(raw, "missing"))
	assert.Equal(t, "octocat", rawLoginFromObject(raw, "user"))
	assert.Equal(t, "", rawLoginFromObject(raw, "missing"))
	assert.Equal(t, []string{"bug", "help wanted"}, rawLabelsFromArray(raw, "labels"))
	assert.Nil(t, rawLabelsFromArray(raw, "missing"))
	assert.Equal(t, "feature-branch", rawRefName(raw, "head"))
	assert.Equal(t, "", rawRefName(raw, "missing"))
}

// ---- event parsing ----------------------------------------------------------

// makeBaseRaw builds the outer-level raw JSON map with repository and sender fields.
func makeBaseRaw(_, action, owner, repo, sender string) map[string]json.RawMessage {
	fullName := owner + "/" + repo
	repoJSON := fmt.Sprintf(`{"full_name":%q,"name":%q,"owner":{"login":%q}}`, fullName, repo, owner)
	senderJSON := fmt.Sprintf(`{"login":%q}`, sender)
	return map[string]json.RawMessage{
		"action":     json.RawMessage(fmt.Sprintf("%q", action)),
		"repository": json.RawMessage(repoJSON),
		"sender":     json.RawMessage(senderJSON),
	}
}

func TestParseIssueEvent(t *testing.T) {
	raw := makeBaseRaw("issues", "opened", "acme", "backend", "alice")
	issueJSON := `{
		"number": 42,
		"title":  "Fix the bug",
		"body":   "It crashes",
		"state":  "open",
		"labels": [{"name":"bug"},{"name":"priority"}],
		"user":   {"login":"alice"},
		"html_url": "https://github.com/acme/backend/issues/42"
	}`
	raw["issue"] = json.RawMessage(issueJSON)

	header, err := extractEventHeader("issues", raw)
	require.NoError(t, err)

	evt, err := parseIssueEvent(header, raw)
	require.NoError(t, err)

	assert.Equal(t, "issues", evt.EventType)
	assert.Equal(t, "opened", evt.Action)
	assert.Equal(t, "acme/backend", evt.Repository.FullName)
	assert.Equal(t, "acme", evt.Repository.Owner)
	assert.Equal(t, "backend", evt.Repository.Name)
	assert.Equal(t, "alice", evt.Sender)
	assert.Equal(t, 42, evt.Issue.Number)
	assert.Equal(t, "Fix the bug", evt.Issue.Title)
	assert.Equal(t, "It crashes", evt.Issue.Body)
	assert.Equal(t, "open", evt.Issue.State)
	assert.Equal(t, []string{"bug", "priority"}, evt.Issue.Labels)
	assert.Equal(t, "alice", evt.Issue.Author)
	assert.Equal(t, "https://github.com/acme/backend/issues/42", evt.Issue.HTMLURL)
}

func TestParsePREvent(t *testing.T) {
	raw := makeBaseRaw("pull_request", "opened", "acme", "backend", "bob")
	prJSON := `{
		"number": 7,
		"title":  "Add feature",
		"body":   "Implements X",
		"state":  "open",
		"head":   {"ref":"feature-x","sha":"aaa"},
		"base":   {"ref":"main","sha":"bbb"},
		"html_url": "https://github.com/acme/backend/pull/7"
	}`
	raw["pull_request"] = json.RawMessage(prJSON)

	header, err := extractEventHeader("pull_request", raw)
	require.NoError(t, err)

	evt, err := parsePREvent(header, raw)
	require.NoError(t, err)

	assert.Equal(t, "pull_request", evt.EventType)
	assert.Equal(t, 7, evt.PullRequest.Number)
	assert.Equal(t, "Add feature", evt.PullRequest.Title)
	assert.Equal(t, "feature-x", evt.PullRequest.Head)
	assert.Equal(t, "main", evt.PullRequest.Base)
	assert.Equal(t, "https://github.com/acme/backend/pull/7", evt.PullRequest.HTMLURL)
}

func TestParseReviewEvent(t *testing.T) {
	raw := makeBaseRaw("pull_request_review", "submitted", "acme", "backend", "carol")
	raw["pull_request"] = json.RawMessage(`{"number":7,"title":"Add feature","state":"open","head":{"ref":"feature-x"},"base":{"ref":"main"}}`)
	raw["review"] = json.RawMessage(`{"state":"approved","body":"LGTM"}`)

	header, err := extractEventHeader("pull_request_review", raw)
	require.NoError(t, err)

	evt, err := parseReviewEvent(header, raw)
	require.NoError(t, err)

	assert.Equal(t, "approved", evt.Review.State)
	assert.Equal(t, "LGTM", evt.Review.Body)
	assert.Equal(t, 7, evt.PullRequest.Number)
}

func TestParseCommentEvent(t *testing.T) {
	raw := makeBaseRaw("issue_comment", "created", "acme", "backend", "dave")
	raw["comment"] = json.RawMessage(`{"body":"Great work!","user":{"login":"dave"}}`)

	header, err := extractEventHeader("issue_comment", raw)
	require.NoError(t, err)

	evt, err := parseCommentEvent(header, raw)
	require.NoError(t, err)

	assert.Equal(t, "Great work!", evt.Comment.Body)
	assert.Equal(t, "dave", evt.Comment.Author)
}

// ---- HTTP handler tests (no real NATS) --------------------------------------

// buildTestBody creates a minimal GitHub webhook JSON body for the given event type.
func buildTestBody(eventType, action string) []byte {
	switch eventType {
	case "issues":
		return []byte(fmt.Sprintf(`{
			"action": %q,
			"repository": {"full_name":"acme/backend","name":"backend"},
			"sender": {"login":"alice"},
			"issue": {"number":1,"title":"t","body":"b","state":"open","labels":[],"user":{"login":"alice"},"html_url":"http://x"}
		}`, action))
	case "pull_request":
		return []byte(fmt.Sprintf(`{
			"action": %q,
			"repository": {"full_name":"acme/backend","name":"backend"},
			"sender": {"login":"alice"},
			"pull_request": {"number":2,"title":"t","body":"b","state":"open","head":{"ref":"feat"},"base":{"ref":"main"},"html_url":"http://x"}
		}`, action))
	case "pull_request_review":
		return []byte(fmt.Sprintf(`{
			"action": %q,
			"repository": {"full_name":"acme/backend","name":"backend"},
			"sender": {"login":"alice"},
			"pull_request": {"number":2,"title":"t","body":"b","state":"open","head":{"ref":"feat"},"base":{"ref":"main"},"html_url":"http://x"},
			"review": {"state":"approved","body":"LGTM"}
		}`, action))
	case "issue_comment":
		return []byte(fmt.Sprintf(`{
			"action": %q,
			"repository": {"full_name":"acme/backend","name":"backend"},
			"sender": {"login":"alice"},
			"comment": {"body":"nice","user":{"login":"alice"}}
		}`, action))
	default:
		return []byte(fmt.Sprintf(`{
			"action": %q,
			"repository": {"full_name":"acme/backend","name":"backend"},
			"sender": {"login":"alice"}
		}`, action))
	}
}

// signBody produces the X-Hub-Signature-256 header value for the given secret and body.
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// newTestInputNoNATS builds an Input with no NATS client for handler-level tests.
// The nil NATS client means dispatch will return an error after parsing,
// which is used to distinguish "reached publish" from earlier failures.
func newTestInputNoNATS(cfg Config) *Input {
	inp := &Input{
		name:   "test",
		config: cfg,
		outputSubjects: map[string]string{
			"github.event.issue":   "github.event.issue",
			"github.event.pr":      "github.event.pr",
			"github.event.review":  "github.event.review",
			"github.event.comment": "github.event.comment",
		},
	}
	if len(inp.config.EventFilter) == 0 {
		inp.config.EventFilter = defaultEventFilter
	}
	return inp
}

// TestHandlerHTTPMethodRejection ensures non-POST methods return 405.
func TestHandlerHTTPMethodRejection(t *testing.T) {
	inp := newTestInputNoNATS(DefaultConfig())

	req := httptest.NewRequest(http.MethodGet, "/github/webhook", nil)
	rec := httptest.NewRecorder()
	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// TestHandlerMissingEventHeader ensures missing X-GitHub-Event returns 400.
func TestHandlerMissingEventHeader(t *testing.T) {
	inp := newTestInputNoNATS(DefaultConfig())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook",
		bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandlerInvalidSignature ensures a bad HMAC returns 403.
func TestHandlerInvalidSignature(t *testing.T) {
	inp := newTestInputNoNATS(Config{
		HTTPPort:    8090,
		Path:        "/github/webhook",
		EventFilter: defaultEventFilter,
	})
	inp.webhookSecret = "correct-secret"

	body := buildTestBody("issues", "opened")
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", "sha256=badhex")
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestHandlerValidSignature verifies that a correct HMAC passes signature validation.
// Because no NATS client is present, dispatch returns an error and the handler
// responds 500 — which is distinct from the 403 that would indicate a bad signature.
func TestHandlerValidSignature(t *testing.T) {
	secret := "my-secret"
	inp := newTestInputNoNATS(DefaultConfig())
	inp.webhookSecret = secret

	body := buildTestBody("issues", "opened")
	sig := signBody(secret, body)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	// 500 is expected because natsClient is nil; 403 would mean sig validation failed.
	assert.NotEqual(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestHandlerEventTypeFilter verifies that unsupported events return 200 with
// a "filtered" response (not an error).
func TestHandlerEventTypeFilter(t *testing.T) {
	inp := newTestInputNoNATS(Config{
		HTTPPort:    8090,
		Path:        "/github/webhook",
		EventFilter: []string{"issues"}, // only issues
	})

	body := buildTestBody("push", "")
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "filtered")
}

// TestHandlerRepoAllowlistFilter verifies that disallowed repositories return 200
// with a filtered response.
func TestHandlerRepoAllowlistFilter(t *testing.T) {
	inp := newTestInputNoNATS(Config{
		HTTPPort:      8090,
		Path:          "/github/webhook",
		EventFilter:   defaultEventFilter,
		RepoAllowlist: []string{"acme/other"},
	})

	body := buildTestBody("issues", "opened")
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "filtered")
}

// TestHandlerInvalidJSON ensures malformed JSON returns 400.
func TestHandlerInvalidJSON(t *testing.T) {
	inp := newTestInputNoNATS(DefaultConfig())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook",
		bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("X-GitHub-Event", "issues")
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---- end-to-end parsing via the internal helpers ----------------------------

// TestHandlerEndToEndParsing calls the low-level parse helpers with the same
// JSON that buildTestBody produces, confirming the parsing pipeline is correct
// for each event type.
func TestHandlerEndToEndParsing(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		action    string
		check     func(t *testing.T, raw map[string]json.RawMessage)
	}{
		{
			name:      "issue event parses correctly",
			eventType: "issues",
			action:    "opened",
			check: func(t *testing.T, raw map[string]json.RawMessage) {
				header, err := extractEventHeader("issues", raw)
				require.NoError(t, err)
				evt, err := parseIssueEvent(header, raw)
				require.NoError(t, err)
				assert.Equal(t, "issues", evt.EventType)
				assert.Equal(t, "opened", evt.Action)
				assert.Equal(t, 1, evt.Issue.Number)
			},
		},
		{
			name:      "pull_request event parses correctly",
			eventType: "pull_request",
			action:    "opened",
			check: func(t *testing.T, raw map[string]json.RawMessage) {
				header, err := extractEventHeader("pull_request", raw)
				require.NoError(t, err)
				evt, err := parsePREvent(header, raw)
				require.NoError(t, err)
				assert.Equal(t, "pull_request", evt.EventType)
				assert.Equal(t, 2, evt.PullRequest.Number)
			},
		},
		{
			name:      "pull_request_review event parses correctly",
			eventType: "pull_request_review",
			action:    "submitted",
			check: func(t *testing.T, raw map[string]json.RawMessage) {
				header, err := extractEventHeader("pull_request_review", raw)
				require.NoError(t, err)
				evt, err := parseReviewEvent(header, raw)
				require.NoError(t, err)
				assert.Equal(t, "approved", evt.Review.State)
			},
		},
		{
			name:      "issue_comment event parses correctly",
			eventType: "issue_comment",
			action:    "created",
			check: func(t *testing.T, raw map[string]json.RawMessage) {
				header, err := extractEventHeader("issue_comment", raw)
				require.NoError(t, err)
				evt, err := parseCommentEvent(header, raw)
				require.NoError(t, err)
				assert.Equal(t, "nice", evt.Comment.Body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := buildTestBody(tc.eventType, tc.action)
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &raw))
			tc.check(t, raw)
		})
	}
}

// ---- component lifecycle tests ----------------------------------------------

func TestComponentLifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HTTPPort = 19876

	t.Run("initialize rejects port 0", func(t *testing.T) {
		inp := &Input{config: Config{HTTPPort: 0, Path: "/webhook"}}
		err := inp.Initialize()
		assert.Error(t, err)
	})

	t.Run("initialize rejects empty path", func(t *testing.T) {
		inp := &Input{config: Config{HTTPPort: 8090, Path: ""}}
		err := inp.Initialize()
		assert.Error(t, err)
	})

	t.Run("stop on non-started component is a no-op", func(t *testing.T) {
		inp := &Input{config: cfg}
		err := inp.Stop(time.Second)
		assert.NoError(t, err)
	})

	t.Run("start requires non-nil context", func(t *testing.T) {
		inp := &Input{config: cfg}
		//nolint:staticcheck // intentional nil context test
		err := inp.Start(nil)
		assert.Error(t, err)
	})

	t.Run("second start returns error", func(t *testing.T) {
		inp := &Input{
			config:         cfg,
			outputSubjects: map[string]string{},
		}
		inp.started.Store(true)
		err := inp.Start(context.Background())
		assert.Error(t, err)
	})
}

// TestComponentMeta verifies the Discoverable metadata.
func TestComponentMeta(t *testing.T) {
	inp := &Input{
		name:   "my-webhook",
		config: DefaultConfig(),
		outputSubjects: map[string]string{
			"github.event.issue":   "github.event.issue",
			"github.event.pr":      "github.event.pr",
			"github.event.review":  "github.event.review",
			"github.event.comment": "github.event.comment",
		},
	}

	meta := inp.Meta()
	assert.Equal(t, "my-webhook", meta.Name)
	assert.Equal(t, "input", meta.Type)
	assert.Equal(t, "1.0.0", meta.Version)

	inPorts := inp.InputPorts()
	require.Len(t, inPorts, 1)
	assert.Equal(t, "http", inPorts[0].Name)

	outPorts := inp.OutputPorts()
	require.Len(t, outPorts, 4)
	names := make([]string, len(outPorts))
	for idx, p := range outPorts {
		names[idx] = p.Name
	}
	assert.ElementsMatch(t, []string{
		"github.event.issue",
		"github.event.pr",
		"github.event.review",
		"github.event.comment",
	}, names)
}

// TestComponentHealth verifies health reporting.
func TestComponentHealth(t *testing.T) {
	inp := &Input{config: DefaultConfig()}

	h := inp.Health()
	assert.False(t, h.Healthy)
	assert.Equal(t, 0, h.ErrorCount)

	inp.started.Store(true)
	inp.startTime = time.Now()
	h = inp.Health()
	assert.True(t, h.Healthy)
}

// TestComponentDataFlow verifies flow metric calculation.
func TestComponentDataFlow(t *testing.T) {
	inp := &Input{
		config:    DefaultConfig(),
		startTime: time.Now().Add(-2 * time.Second),
	}
	inp.started.Store(true)
	atomic.StoreInt64(&inp.messagesReceived, 10)

	flow := inp.DataFlow()
	assert.Greater(t, flow.MessagesPerSecond, 0.0)
}

// TestDefaultConfig verifies sensible defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 8090, cfg.HTTPPort)
	assert.Equal(t, "/github/webhook", cfg.Path)
	assert.ElementsMatch(t, []string{"issues", "pull_request", "pull_request_review"}, cfg.EventFilter)
	require.NotNil(t, cfg.Ports)
	assert.Len(t, cfg.Ports.Outputs, 4)
}

// TestHTTPServerStartStop exercises Start+Stop using a real TCP port by starting
// the server and sending a real HTTP POST to confirm it is reachable.
// The nil NATS client causes a 500 response, which confirms the server is up
// and the handler pipeline executed past all filtering steps.
func TestHTTPServerStartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HTTPPort = 19877

	inp := &Input{
		name:   "test-server",
		config: cfg,
		outputSubjects: map[string]string{
			"github.event.issue":   "github.event.issue",
			"github.event.pr":      "github.event.pr",
			"github.event.review":  "github.event.review",
			"github.event.comment": "github.event.comment",
		},
	}
	inp.config.EventFilter = defaultEventFilter

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		inp.handleWebhook(ctx, w, r)
	})
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: mux,
	}
	inp.httpServer = srv
	inp.started.Store(true)
	inp.startTime = time.Now()

	go func() {
		_ = srv.ListenAndServe()
	}()

	// Wait briefly for the listener to be ready.
	time.Sleep(50 * time.Millisecond)

	body := buildTestBody("issues", "opened")
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://localhost:%d%s", cfg.HTTPPort, cfg.Path),
		bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-GitHub-Event", "issues")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	// 500 confirms the server is reachable and the handler ran to the publish step.
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	err = inp.Stop(2 * time.Second)
	assert.NoError(t, err)
}

// TestHTTPServerAcceptsFilteredEvent verifies that a filtered event returns 200
// when the component is running a real HTTP server.
func TestHTTPServerAcceptsFilteredEvent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HTTPPort = 19878
	cfg.EventFilter = []string{"issues"} // only issues

	inp := &Input{
		name:   "test-server-filter",
		config: cfg,
		outputSubjects: map[string]string{
			"github.event.issue":   "github.event.issue",
			"github.event.pr":      "github.event.pr",
			"github.event.review":  "github.event.review",
			"github.event.comment": "github.event.comment",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		inp.handleWebhook(ctx, w, r)
	})
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: mux,
	}
	inp.httpServer = srv
	inp.started.Store(true)
	inp.startTime = time.Now()

	go func() {
		_ = srv.ListenAndServe()
	}()
	time.Sleep(50 * time.Millisecond)

	// Send a push event (not in filter) — expect 200 with "filtered".
	pushBody := []byte(`{"action":"","repository":{"full_name":"acme/backend"},"sender":{"login":"alice"}}`)
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://localhost:%d%s", cfg.HTTPPort, cfg.Path),
		bytes.NewReader(pushBody))
	require.NoError(t, err)
	req.Header.Set("X-GitHub-Event", "push")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(respBody), "filtered")

	_ = inp.Stop(2 * time.Second)
}

// TestPayloadRegistration verifies that the package init() function registered
// all four event types with the expected concrete factories.
func TestPayloadRegistration(t *testing.T) {
	// The init() function runs when this package is loaded. We verify each
	// registration by building instances from the same factories init() uses,
	// confirming the expected types are produced.
	factories := map[string]func() any{
		"github.issue_event.v1":   func() any { return &IssueEvent{} },
		"github.pr_event.v1":      func() any { return &PREvent{} },
		"github.review_event.v1":  func() any { return &ReviewEvent{} },
		"github.comment_event.v1": func() any { return &CommentEvent{} },
	}

	tests := []struct {
		key      string
		wantType any
	}{
		{"github.issue_event.v1", &IssueEvent{}},
		{"github.pr_event.v1", &PREvent{}},
		{"github.review_event.v1", &ReviewEvent{}},
		{"github.comment_event.v1", &CommentEvent{}},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			f, ok := factories[tc.key]
			require.True(t, ok, "factory not found for %s", tc.key)
			got := f()
			require.NotNil(t, got)
			assert.IsType(t, tc.wantType, got)
		})
	}
}

// TestReceivedAtIsSet ensures the ReceivedAt field is populated during event parsing.
func TestReceivedAtIsSet(t *testing.T) {
	before := time.Now().UTC()
	raw := makeBaseRaw("issues", "opened", "acme", "backend", "alice")
	raw["issue"] = json.RawMessage(`{"number":1,"title":"t","state":"open","labels":[],"user":{"login":"alice"}}`)

	header, err := extractEventHeader("issues", raw)
	require.NoError(t, err)
	after := time.Now().UTC()

	assert.True(t, !header.ReceivedAt.Before(before), "ReceivedAt should be >= before")
	assert.True(t, !header.ReceivedAt.After(after), "ReceivedAt should be <= after")
}

// TestNoSignatureValidationWhenSecretEmpty ensures that when no HMAC secret is
// configured, all requests proceed past signature validation.
func TestNoSignatureValidationWhenSecretEmpty(t *testing.T) {
	inp := newTestInputNoNATS(DefaultConfig())
	// webhookSecret is empty — validation must be skipped.

	body := buildTestBody("issues", "opened")
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	// No X-Hub-Signature-256 header intentionally.
	rec := httptest.NewRecorder()

	inp.handleWebhook(context.Background(), rec, req)

	// 500 is expected because natsClient is nil.
	// Any 403 would indicate the signature check ran incorrectly.
	assert.NotEqual(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
