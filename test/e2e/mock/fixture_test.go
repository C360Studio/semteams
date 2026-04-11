package mock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
