package sandboxmanager

import (
	"encoding/json"
	"testing"
)

func TestParseRequirements_HappyPath(t *testing.T) {
	raw := map[string]any{
		"languages":  []any{"go", "node"},
		"tools":      []any{"task"},
		"services":   []any{"nats"},
		"network":    "restricted",
		"secrets":    []any{"OPENAI_API_KEY"},
		"mounts":     []any{"workspace-write"},
		"privileges": []any{"docker-socket"},
		"verification": []any{
			map[string]any{"name": "go", "command": "go version"},
			map[string]any{"name": "task", "command": "task --version", "expect_exit": float64(0)},
		},
	}
	got, err := ParseRequirements(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Languages) != 2 || got.Languages[1] != "node" {
		t.Fatalf("languages wrong: %v", got.Languages)
	}
	if got.Network != NetworkRestricted {
		t.Fatalf("network wrong: %q", got.Network)
	}
	if len(got.Privileges) != 1 || got.Privileges[0] != PrivilegeDockerSocket {
		t.Fatalf("privileges wrong: %v", got.Privileges)
	}
	if len(got.Verification) != 2 {
		t.Fatalf("verification len wrong: %d", len(got.Verification))
	}
}

func TestParseRequirements_TrimsAllStringFields(t *testing.T) {
	raw := map[string]any{
		"languages": []any{" go "},
		"secrets":   []any{" OPENAI_API_KEY "},
	}
	got, err := ParseRequirements(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Languages[0] != "go" {
		t.Fatalf("language not trimmed: %q", got.Languages[0])
	}
	if got.Secrets[0] != "OPENAI_API_KEY" {
		t.Fatalf("secret not trimmed: %q", got.Secrets[0])
	}
}

func TestParseRequirements_UnrecognizedFieldsIgnored(t *testing.T) {
	raw := map[string]any{
		"languages":   []any{"go"},
		"some_future": "field",
	}
	got, err := ParseRequirements(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Languages) != 1 {
		t.Fatalf("forward-compat broken: %v", got)
	}
}

func TestParseRequirements_BadShape(t *testing.T) {
	cases := []map[string]any{
		{"languages": []any{42}},
		{"mounts": []any{99}},
		{"privileges": []any{true}},
		{"verification": []any{"not an object"}},
		{"verification": []any{map[string]any{"name": "x", "command": "y", "expect_exit": "not int"}}},
	}
	for i, raw := range cases {
		if _, err := ParseRequirements(raw); err == nil {
			t.Errorf("case %d: invalid input accepted: %+v", i, raw)
		}
	}
}

func TestParseRequirements_ExpectExitFlexibleType(t *testing.T) {
	for _, v := range []any{float64(2), int(2), json.Number("2")} {
		raw := map[string]any{
			"verification": []any{map[string]any{"name": "x", "command": "echo", "expect_exit": v}},
		}
		got, err := ParseRequirements(raw)
		if err != nil {
			t.Fatalf("expect_exit %T: %v", v, err)
		}
		if got.Verification[0].ExpectExit != 2 {
			t.Fatalf("expect_exit %T did not parse: %d", v, got.Verification[0].ExpectExit)
		}
	}
}

func TestParseRequirements_RoundTripsThroughHash(t *testing.T) {
	// Same JSON content, different field order on the wire (LLMs
	// produce both orderings) — Hash() must be identical.
	rawA := map[string]any{
		"languages": []any{"go", "node"},
		"tools":     []any{"task"},
		"network":   "restricted",
		"verification": []any{
			map[string]any{"name": "go", "command": "go version"},
			map[string]any{"name": "task", "command": "task --list"},
		},
	}
	rawB := map[string]any{
		"verification": []any{
			map[string]any{"name": "task", "command": "task --list"},
			map[string]any{"name": "GO", "command": "go version"}, // case differs
		},
		"tools":     []any{"task"},
		"languages": []any{"node", "go"}, // order differs
		"network":   "",                  // zero defaults to restricted
	}
	a, err := ParseRequirements(rawA)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := ParseRequirements(rawB)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("equivalent parsed shapes hashed differently:\n  a=%s\n  b=%s", a.Hash(), b.Hash())
	}
}
