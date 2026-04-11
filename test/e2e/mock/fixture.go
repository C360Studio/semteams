package mock

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Fixture is a deterministic LLM response script loaded from a YAML file.
// Each entry in Responses represents one LLM call; the mock server returns
// them in order and repeats the last entry once the sequence is exhausted.
//
// See docs/journeys/README.md for usage context and
// test/fixtures/journeys/ for examples.
type Fixture struct {
	// Name identifies the fixture (human-readable, typically matches filename).
	Name string `yaml:"name"`

	// Description is a one-line human summary of what this fixture simulates.
	Description string `yaml:"description,omitempty"`

	// Responses is the ordered sequence of LLM responses. Must contain at
	// least one entry.
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
	if len(f.Responses) == 0 {
		return fmt.Errorf("fixture has no responses (must have at least one)")
	}
	for i, r := range f.Responses {
		if r.ToolCall == nil && r.Completion == nil {
			return fmt.Errorf("response %d: must set exactly one of tool_call or completion", i)
		}
		if r.ToolCall != nil && r.Completion != nil {
			return fmt.Errorf("response %d: cannot set both tool_call and completion", i)
		}
		if r.ToolCall != nil && r.ToolCall.Name == "" {
			return fmt.Errorf("response %d: tool_call.name is required", i)
		}
		if r.Completion != nil && r.Completion.Content == "" {
			return fmt.Errorf("response %d: completion.content is required", i)
		}
	}
	return nil
}
