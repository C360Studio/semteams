package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseFixture(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     string // substring match; empty = expect no error
		wantCount   int
		wantName    string
		firstIsCall bool // true if first response should be a tool_call
	}{
		{
			name: "valid single tool_call",
			yaml: `
name: single-tool-call
responses:
  - tool_call:
      name: create_rule
      arguments_json: '{"name":"high-temp"}'
`,
			wantCount:   1,
			wantName:    "single-tool-call",
			firstIsCall: true,
		},
		{
			name: "valid single completion",
			yaml: `
name: single-completion
description: just a completion
responses:
  - completion:
      content: All done.
`,
			wantCount:   1,
			wantName:    "single-completion",
			firstIsCall: false,
		},
		{
			name: "valid multi-turn tool_call then completion",
			yaml: `
name: tool-approval-gate
description: Agent proposes create_rule, user approves, loop completes.
responses:
  - tool_call:
      name: create_rule
      arguments_json: '{"name":"high-temp","condition":"temp > 100"}'
  - completion:
      content: Rule created successfully.
`,
			wantCount:   2,
			wantName:    "tool-approval-gate",
			firstIsCall: true,
		},
		{
			name:    "empty responses",
			yaml:    `name: empty`,
			wantErr: "no responses",
		},
		{
			name: "response with neither tool_call nor completion",
			yaml: `
name: bad
responses:
  - {}
`,
			wantErr: "must set exactly one",
		},
		{
			name: "response with both tool_call and completion",
			yaml: `
name: bad
responses:
  - tool_call:
      name: foo
      arguments_json: '{}'
    completion:
      content: hi
`,
			wantErr: "cannot set both",
		},
		{
			name: "tool_call missing name",
			yaml: `
name: bad
responses:
  - tool_call:
      arguments_json: '{}'
`,
			wantErr: "tool_call.name is required",
		},
		{
			name: "completion missing content",
			yaml: `
name: bad
responses:
  - completion: {}
`,
			wantErr: "completion.content is required",
		},
		{
			name:    "invalid YAML syntax",
			yaml:    `name: [unterminated`,
			wantErr: "parse fixture YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFixture([]byte(tt.yaml))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if len(got.Responses) != tt.wantCount {
				t.Errorf("len(Responses) = %d, want %d", len(got.Responses), tt.wantCount)
			}
			if tt.wantCount > 0 {
				first := got.Responses[0]
				if tt.firstIsCall && first.ToolCall == nil {
					t.Errorf("first response: expected tool_call, got completion")
				}
				if !tt.firstIsCall && first.Completion == nil {
					t.Errorf("first response: expected completion, got tool_call")
				}
			}
		})
	}
}

func TestLoadFixture_FileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journey.yaml")
	content := []byte(`
name: round-trip
description: Verifies the file loader.
responses:
  - tool_call:
      name: create_rule
      arguments_json: '{"name":"x"}'
  - completion:
      content: Done.
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	f, err := LoadFixture(path)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if f.Name != "round-trip" {
		t.Errorf("Name = %q, want %q", f.Name, "round-trip")
	}
	if len(f.Responses) != 2 {
		t.Fatalf("len(Responses) = %d, want 2", len(f.Responses))
	}
	if f.Responses[0].ToolCall == nil || f.Responses[0].ToolCall.Name != "create_rule" {
		t.Errorf("Responses[0].ToolCall.Name = %+v, want create_rule", f.Responses[0].ToolCall)
	}
	if f.Responses[1].Completion == nil || f.Responses[1].Completion.Content != "Done." {
		t.Errorf("Responses[1].Completion.Content = %+v, want Done.", f.Responses[1].Completion)
	}
}

func TestLoadFixture_MissingFile(t *testing.T) {
	_, err := LoadFixture("/nonexistent/path/fixture.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "read fixture") {
		t.Errorf("error %q does not mention 'read fixture'", err.Error())
	}
}

// --- Role-keyed fixture tests ------------------------------------------------

// TestParseFixture_RoleMode_Valid exercises the happy-path parse + validate
// for a two-bucket role-keyed fixture.
func TestParseFixture_RoleMode_Valid(t *testing.T) {
	yaml := `
name: fan-out
description: gather + synthesize interleaved
responses_by_role:
  - match: "evidence in scratchpad"
    responses:
      - completion:
          content: gather done
  - match: "synthesis pass"
    responses:
      - tool_call:
          name: submit_work
          arguments_json: '{"result":"synth"}'
      - completion:
          content: synthesis done
`
	f, err := ParseFixture([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Name != "fan-out" {
		t.Errorf("Name = %q, want fan-out", f.Name)
	}
	if len(f.Responses) != 0 {
		t.Errorf("Responses should be empty in role mode, got %d", len(f.Responses))
	}
	if len(f.ResponsesByRole) != 2 {
		t.Fatalf("len(ResponsesByRole) = %d, want 2", len(f.ResponsesByRole))
	}
	if f.ResponsesByRole[0].Match != "evidence in scratchpad" {
		t.Errorf("bucket[0].Match = %q, want 'evidence in scratchpad'", f.ResponsesByRole[0].Match)
	}
	if len(f.ResponsesByRole[0].Responses) != 1 {
		t.Errorf("bucket[0] len(Responses) = %d, want 1", len(f.ResponsesByRole[0].Responses))
	}
	if len(f.ResponsesByRole[1].Responses) != 2 {
		t.Errorf("bucket[1] len(Responses) = %d, want 2", len(f.ResponsesByRole[1].Responses))
	}
}

// TestParseFixture_RoleMode_Errors covers all validate error paths for
// role-keyed fixtures.
func TestParseFixture_RoleMode_Errors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "both responses and responses_by_role set",
			yaml: `
name: bad
responses:
  - completion:
      content: hi
responses_by_role:
  - match: "foo"
    responses:
      - completion:
          content: bar
`,
			wantErr: "exactly one of responses or responses_by_role",
		},
		{
			name:    "neither responses nor responses_by_role",
			yaml:    `name: empty`,
			wantErr: "no responses",
		},
		{
			name: "empty match string",
			yaml: `
name: bad
responses_by_role:
  - match: ""
    responses:
      - completion:
          content: hi
`,
			wantErr: "match is required",
		},
		{
			name: "duplicate match strings",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses:
      - completion:
          content: a
  - match: "coordinator"
    responses:
      - completion:
          content: b
`,
			wantErr: "duplicates bucket",
		},
		{
			name: "bucket with no responses",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses: []
`,
			wantErr: "must have at least one response",
		},
		{
			name: "bucket response missing both tool_call and completion",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses:
      - {}
`,
			wantErr: "must set exactly one of tool_call or completion",
		},
		{
			name: "bucket response with both tool_call and completion",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses:
      - tool_call:
          name: foo
          arguments_json: '{}'
        completion:
          content: hi
`,
			wantErr: "cannot set both",
		},
		{
			name: "bucket response tool_call missing name",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses:
      - tool_call:
          arguments_json: '{}'
`,
			wantErr: "tool_call.name is required",
		},
		{
			name: "bucket response completion missing content",
			yaml: `
name: bad
responses_by_role:
  - match: "coordinator"
    responses:
      - completion: {}
`,
			wantErr: "completion.content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFixture([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestOpenAIServer_RoleMode_BucketSelection verifies that requests are routed
// to the correct bucket based on system-prompt content, and that per-bucket
// cursors advance independently.
func TestOpenAIServer_RoleMode_BucketSelection(t *testing.T) {
	fixture, err := ParseFixture([]byte(`
name: role-selection
responses_by_role:
  - match: "Delegation rules"
    responses:
      - completion:
          content: coordinator response 1
      - completion:
          content: coordinator response 2
  - match: "evidence in scratchpad"
    responses:
      - tool_call:
          name: submit_work
          arguments_json: '{"result":"gathered"}'
      - completion:
          content: gather done
`))
	if err != nil {
		t.Fatalf("ParseFixture: %v", err)
	}

	srv := NewOpenAIServer().WithFixture(fixture)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	coordMsg := func() ChatCompletionRequest {
		return ChatCompletionRequest{
			Model: "mock",
			Messages: []ChatMessage{
				{Role: "system", Content: "Delegation rules — you are the coordinator"},
				{Role: "user", Content: "hello"},
			},
		}
	}
	gatherMsg := func() ChatCompletionRequest {
		return ChatCompletionRequest{
			Model: "mock",
			Messages: []ChatMessage{
				{Role: "system", Content: "You are a gather agent. evidence in scratchpad."},
				{Role: "user", Content: "gather now"},
			},
		}
	}

	// Interleave coordinator and gather calls to confirm independent cursors.

	// Call 1: coordinator → coordinator response 1
	r1 := chatCompletion(t, srv.URL(), coordMsg())
	if r1.Choices[0].Message.Content != "coordinator response 1" {
		t.Errorf("call 1: content = %q, want 'coordinator response 1'", r1.Choices[0].Message.Content)
	}

	// Call 2: gather → tool_call (gather bucket idx=0)
	r2 := chatCompletion(t, srv.URL(), gatherMsg())
	if len(r2.Choices[0].Message.ToolCalls) == 0 || r2.Choices[0].Message.ToolCalls[0].Function.Name != "submit_work" {
		t.Errorf("call 2: expected submit_work tool_call, got %+v", r2.Choices[0])
	}

	// Call 3: coordinator → coordinator response 2 (coordinator cursor advanced independently)
	r3 := chatCompletion(t, srv.URL(), coordMsg())
	if r3.Choices[0].Message.Content != "coordinator response 2" {
		t.Errorf("call 3: content = %q, want 'coordinator response 2'", r3.Choices[0].Message.Content)
	}

	// Call 4: gather → gather done (gather bucket idx=1)
	r4 := chatCompletion(t, srv.URL(), gatherMsg())
	if r4.Choices[0].Message.Content != "gather done" {
		t.Errorf("call 4: content = %q, want 'gather done'", r4.Choices[0].Message.Content)
	}
}

// TestOpenAIServer_RoleMode_RepeatLastOnExhaustion verifies that once a
// bucket's responses are exhausted the last entry is repeated.
func TestOpenAIServer_RoleMode_RepeatLastOnExhaustion(t *testing.T) {
	fixture, err := ParseFixture([]byte(`
name: repeat-last
responses_by_role:
  - match: "synthesis pass"
    responses:
      - completion:
          content: synth response
`))
	if err != nil {
		t.Fatalf("ParseFixture: %v", err)
	}

	srv := NewOpenAIServer().WithFixture(fixture)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	req := ChatCompletionRequest{
		Model: "mock",
		Messages: []ChatMessage{
			{Role: "system", Content: "synthesis pass — N gather terminal summaries"},
			{Role: "user", Content: "synthesize"},
		},
	}

	// First call consumes the only entry.
	r1 := chatCompletion(t, srv.URL(), req)
	if r1.Choices[0].Message.Content != "synth response" {
		t.Errorf("call 1: content = %q, want 'synth response'", r1.Choices[0].Message.Content)
	}

	// Second and third calls repeat the last entry.
	for i := 2; i <= 3; i++ {
		r := chatCompletion(t, srv.URL(), req)
		if r.Choices[0].Message.Content != "synth response" {
			t.Errorf("call %d (repeat): content = %q, want 'synth response'", i, r.Choices[0].Message.Content)
		}
	}
}

// TestOpenAIServer_RoleMode_NoMatchFallback verifies that when no bucket
// matches the system prompt the server returns the last response of the
// first bucket (and doesn't hang or crash).
func TestOpenAIServer_RoleMode_NoMatchFallback(t *testing.T) {
	fixture, err := ParseFixture([]byte(`
name: no-match-fallback
responses_by_role:
  - match: "Delegation rules"
    responses:
      - completion:
          content: fallback response
  - match: "evidence in scratchpad"
    responses:
      - completion:
          content: gather response
`))
	if err != nil {
		t.Fatalf("ParseFixture: %v", err)
	}

	srv := NewOpenAIServer().WithFixture(fixture)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	// A request with no matching system-prompt keyword should fall back to
	// the first bucket's last response.
	req := ChatCompletionRequest{
		Model: "mock",
		Messages: []ChatMessage{
			{Role: "system", Content: "completely unrecognised persona text"},
			{Role: "user", Content: "do something"},
		},
	}

	r := chatCompletion(t, srv.URL(), req)
	// Falls back to last response of bucket[0] = "fallback response".
	if r.Choices[0].Message.Content != "fallback response" {
		t.Errorf("no-match fallback: content = %q, want 'fallback response'", r.Choices[0].Message.Content)
	}
}

// TestOpenAIServer_FixtureDrivesResponses verifies that WithFixture plumbs
// the fixture into the chat completion handler and that responses are
// returned in order with the last one repeated after exhaustion.
func TestOpenAIServer_FixtureDrivesResponses(t *testing.T) {
	fixture, err := ParseFixture([]byte(`
name: approval-gate
responses:
  - tool_call:
      name: create_rule
      arguments_json: '{"name":"high-temp-alert"}'
  - completion:
      content: Rule created.
`))
	if err != nil {
		t.Fatalf("ParseFixture: %v", err)
	}

	srv := NewOpenAIServer().WithFixture(fixture)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	// Turn 1 — expect the tool_call response, regardless of whether the
	// request has any tools listed.
	resp1 := chatCompletion(t, srv.URL(), ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "add a rule for high temp"}},
	})
	if len(resp1.Choices) != 1 {
		t.Fatalf("turn 1: len(Choices) = %d, want 1", len(resp1.Choices))
	}
	tcs := resp1.Choices[0].Message.ToolCalls
	if len(tcs) != 1 || tcs[0].Function.Name != "create_rule" {
		t.Errorf("turn 1: tool_calls = %+v, want create_rule", tcs)
	}
	if resp1.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("turn 1: FinishReason = %q, want tool_calls", resp1.Choices[0].FinishReason)
	}

	// Turn 2 — completion.
	resp2 := chatCompletion(t, srv.URL(), ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "user", Content: "add a rule"},
			{Role: "assistant", ToolCalls: tcs},
			{Role: "tool", ToolCallID: tcs[0].ID, Content: "rule created"},
		},
	})
	if resp2.Choices[0].Message.Content != "Rule created." {
		t.Errorf("turn 2: Content = %q, want %q", resp2.Choices[0].Message.Content, "Rule created.")
	}
	if resp2.Choices[0].FinishReason != "stop" {
		t.Errorf("turn 2: FinishReason = %q, want stop", resp2.Choices[0].FinishReason)
	}

	// Turn 3 — sequence exhausted, repeats the last entry (completion).
	resp3 := chatCompletion(t, srv.URL(), ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "anything"}},
	})
	if resp3.Choices[0].Message.Content != "Rule created." {
		t.Errorf("turn 3 (repeat): Content = %q, want %q", resp3.Choices[0].Message.Content, "Rule created.")
	}
}

// chatCompletion is a tiny helper that posts a request to the mock's chat
// completion endpoint and returns the parsed response.
func chatCompletion(t *testing.T, baseURL string, req ChatCompletionRequest) ChatCompletionResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	httpResp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", httpResp.StatusCode)
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// TestOpenAIServer_RoleMode_ConcurrentRequests fires many interleaved requests
// across two buckets to exercise the per-bucket cursor map (roleIdx) under
// `go test -race`. Role-keyed mode exists precisely to serve loops that run
// concurrently / out of global order, so the cursor map must be race-free —
// and -race only catches the race if the test actually races the map. Uses raw
// http.Post (not the chatCompletion helper) because t.Fatalf must not be called
// from a goroutine; failures are funnelled to the main goroutine via a channel.
func TestOpenAIServer_RoleMode_ConcurrentRequests(t *testing.T) {
	fixture, err := ParseFixture([]byte(`
name: role-concurrent
responses_by_role:
  - match: "Delegation rules"
    responses:
      - completion:
          content: "coordinator-A"
      - completion:
          content: "coordinator-B"
  - match: "evidence in scratchpad"
    responses:
      - completion:
          content: "gather-A"
      - completion:
          content: "gather-B"
`))
	if err != nil {
		t.Fatalf("ParseFixture: %v", err)
	}
	srv := NewOpenAIServer().WithFixture(fixture)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	post := func(sysContent string) (string, error) {
		req := ChatCompletionRequest{
			Model: "mock",
			Messages: []ChatMessage{
				{Role: "system", Content: sysContent},
				{Role: "user", Content: "go"},
			},
		}
		body, err := json.Marshal(req)
		if err != nil {
			return "", err
		}
		httpResp, err := http.Post(srv.URL()+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		defer httpResp.Body.Close()
		var resp ChatCompletionResponse
		if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		return resp.Choices[0].Message.Content, nil
	}

	type result struct {
		family string
		got    string
		err    error
	}
	const perBucket = 40
	results := make(chan result, perBucket*2)
	var wg sync.WaitGroup
	for i := 0; i < perBucket; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			got, err := post("Delegation rules — you are the coordinator")
			results <- result{"coordinator", got, err}
		}()
		go func() {
			defer wg.Done()
			got, err := post("gather agent: evidence in scratchpad")
			results <- result{"gather", got, err}
		}()
	}
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("%s request failed: %v", r.family, r.err)
			continue
		}
		// Repeat-last keeps every response in-family regardless of how the
		// cursor advanced under concurrency, so an order-independent prefix
		// check is the right correctness assertion.
		if !strings.HasPrefix(r.got, r.family) {
			t.Errorf("%s request got %q, want prefix %q", r.family, r.got, r.family)
		}
	}
}
