package mock

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Fixture is a deterministic LLM response script loaded from a YAML file.
//
// Sequential mode (default): set Responses — each entry in the ordered list
// represents one LLM call; the mock server returns them in order and repeats
// the last entry once the sequence is exhausted.
//
// Role-keyed mode (opt-in): set ResponsesByRole — each RoleBucket holds its
// own ordered response queue matched by a substring in the concatenated
// system-prompt messages of the incoming request. Responses for distinct
// roles advance independently. Use this for fan-out / interleaved chains
// where the global request order is nondeterministic.
//
// Exactly one of Responses / ResponsesByRole must be set.
//
// See docs/journeys/README.md for usage context and
// test/fixtures/journeys/ for examples.
type Fixture struct {
	// Name identifies the fixture (human-readable, typically matches filename).
	Name string `yaml:"name"`

	// Description is a one-line human summary of what this fixture simulates.
	Description string `yaml:"description,omitempty"`

	// Responses is the ordered sequence of LLM responses for sequential mode.
	// Must contain at least one entry when ResponsesByRole is not set.
	Responses []FixtureResponse `yaml:"responses,omitempty"`

	// ResponsesByRole is the role-keyed response map for fan-out chains.
	// Each bucket's responses advance independently; the first bucket whose
	// Match substring appears in the concatenated system messages is used.
	// Must contain at least one bucket when Responses is not set.
	ResponsesByRole []RoleBucket `yaml:"responses_by_role,omitempty"`
}

// RoleBucket is an ordered response queue keyed by a system-prompt fingerprint.
// The mock server concatenates all system-role message contents in the request
// and returns the next response from the first bucket whose Match string is
// found as a substring. Per-bucket cursors advance independently, enabling
// role-interleaved fan-out chains.
type RoleBucket struct {
	// Match is a substring sought in the concatenated system prompts of the
	// incoming request. Must be non-empty and unique across all buckets in
	// the fixture. Ordering matters: the FIRST bucket whose Match is contained
	// in the prompt wins, so if one Match is a substring of another (e.g.
	// "research" vs "research plan") list the more-specific bucket first.
	// Validate rejects exact duplicates but cannot detect substring overlap —
	// an overlap silently shadows the later bucket (a match IS found, so no
	// warning fires), so pick distinctive fingerprints.
	Match string `yaml:"match"`

	// Responses is the ordered sequence of responses for this role bucket.
	// Must contain at least one entry. Follows the same exactly-one-of
	// tool_call/completion constraint as the top-level Responses field.
	// Once exhausted, the last response is repeated (same semantics as
	// sequential mode).
	Responses []FixtureResponse `yaml:"responses"`
}

// FixtureResponse is a single LLM response. Exactly one of ToolCall or
// Completion must be set.
type FixtureResponse struct {
	// ToolCall makes the LLM emit a tool call with the given name and
	// arguments. Set this for turns where the agent should propose a tool.
	ToolCall *FixtureToolCall `yaml:"tool_call,omitempty"`

	// Completion makes the LLM emit a text completion with the given
	// content. Set this for turns where the agent should respond with prose
	// (final answer, intermediate reasoning, etc.).
	Completion *FixtureCompletion `yaml:"completion,omitempty"`
}

// FixtureToolCall is a deterministic tool call. Name and ArgumentsJSON are
// the only fields the mock server passes through to the OpenAI response.
type FixtureToolCall struct {
	// Name is the tool name (e.g. "create_rule", "query_entity").
	Name string `yaml:"name"`

	// ArgumentsJSON is the tool arguments as a JSON string. The OpenAI
	// protocol expects tool arguments as an opaque JSON-encoded string, not
	// a structured object, so fixtures store them that way too.
	ArgumentsJSON string `yaml:"arguments_json"`
}

// FixtureCompletion is a deterministic completion.
type FixtureCompletion struct {
	// Content is the completion text (plain prose or a JSON blob the agent
	// is expected to parse — depends on what the component expects).
	Content string `yaml:"content"`
}

// LoadFixture reads and validates a fixture from a YAML file on disk.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixture path is operator-controlled via CLI flag
	if err != nil {
		return nil, fmt.Errorf("read fixture %q: %w", path, err)
	}
	return ParseFixture(data)
}

// ParseFixture parses and validates a fixture from in-memory YAML bytes.
// Exposed separately from LoadFixture to enable in-memory testing.
func ParseFixture(data []byte) (*Fixture, error) {
	var f Fixture
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse fixture YAML: %w", err)
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Validate checks that the fixture is well-formed and usable by the mock
// server. Returns the first error encountered.
func (f *Fixture) Validate() error {
	hasSeq := len(f.Responses) > 0
	hasRole := len(f.ResponsesByRole) > 0

	if hasSeq && hasRole {
		return fmt.Errorf("fixture must set exactly one of responses or responses_by_role, not both")
	}
	if !hasSeq && !hasRole {
		return fmt.Errorf("fixture has no responses (must have at least one)")
	}

	if hasSeq {
		return validateResponses(f.Responses, "")
	}

	// Role-keyed mode validation.
	seen := make(map[string]int, len(f.ResponsesByRole))
	for bi, bucket := range f.ResponsesByRole {
		if bucket.Match == "" {
			return fmt.Errorf("responses_by_role[%d]: match is required", bi)
		}
		if prev, dup := seen[bucket.Match]; dup {
			return fmt.Errorf("responses_by_role[%d]: match %q duplicates bucket %d", bi, bucket.Match, prev)
		}
		seen[bucket.Match] = bi
		if len(bucket.Responses) == 0 {
			return fmt.Errorf("responses_by_role[%d] (match=%q): must have at least one response", bi, bucket.Match)
		}
		if err := validateResponses(bucket.Responses, fmt.Sprintf("responses_by_role[%d] (match=%q) ", bi, bucket.Match)); err != nil {
			return err
		}
	}
	return nil
}

// validateResponses validates the per-response exactly-one-of constraint for
// a slice of FixtureResponse values. prefix is prepended to error messages
// (use empty string for top-level Responses).
func validateResponses(responses []FixtureResponse, prefix string) error {
	for i, r := range responses {
		if r.ToolCall == nil && r.Completion == nil {
			return fmt.Errorf("%sresponse %d: must set exactly one of tool_call or completion", prefix, i)
		}
		if r.ToolCall != nil && r.Completion != nil {
			return fmt.Errorf("%sresponse %d: cannot set both tool_call and completion", prefix, i)
		}
		if r.ToolCall != nil && r.ToolCall.Name == "" {
			return fmt.Errorf("%sresponse %d: tool_call.name is required", prefix, i)
		}
		if r.Completion != nil && r.Completion.Content == "" {
			return fmt.Errorf("%sresponse %d: completion.content is required", prefix, i)
		}
	}
	return nil
}
