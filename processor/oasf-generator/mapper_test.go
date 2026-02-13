package oasfgenerator

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	agentic "github.com/c360studio/semstreams/vocabulary/agentic"
)

func TestMapper_MapTriplesToOASF_BasicCapability(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, true)

	agentID := "acme.ops.agentic.system.agent.architect"
	capabilityContext := "software-design" // Links related capability triples
	triples := []message.Triple{
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityName,
			Object:    "Software Design",
			Source:    "test",
			Timestamp: time.Now(),
			Context:   capabilityContext,
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityDescription,
			Object:    "Creates software architecture diagrams",
			Source:    "test",
			Timestamp: time.Now(),
			Context:   capabilityContext,
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityExpression,
			Object:    "software-design",
			Source:    "test",
			Timestamp: time.Now(),
			Context:   capabilityContext,
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityConfidence,
			Object:    0.95,
			Source:    "test",
			Timestamp: time.Now(),
			Context:   capabilityContext,
		},
	}

	record, err := mapper.MapTriplesToOASF(agentID, triples)
	if err != nil {
		t.Fatalf("MapTriplesToOASF() error = %v", err)
	}

	if len(record.Skills) == 0 {
		t.Fatal("expected at least one skill")
	}

	// Find the software-design skill
	var skill *OASFSkill
	for i := range record.Skills {
		if record.Skills[i].ID == "software-design" {
			skill = &record.Skills[i]
			break
		}
	}

	if skill == nil {
		t.Fatal("expected to find 'software-design' skill")
	}

	if skill.Name != "Software Design" {
		t.Errorf("expected skill name 'Software Design', got %q", skill.Name)
	}
}

func TestMapper_MapTriplesToOASF_WithPermissions(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, true)

	agentID := "acme.ops.agentic.system.agent.editor"
	triples := []message.Triple{
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityExpression,
			Object:    "file-editing",
			Context:   "file-editing",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityName,
			Object:    "File Editing",
			Context:   "file-editing",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityPermission,
			Object:    "file_read",
			Context:   "file-editing",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityPermission,
			Object:    "file_write",
			Context:   "file-editing",
			Source:    "test",
			Timestamp: time.Now(),
		},
	}

	record, err := mapper.MapTriplesToOASF(agentID, triples)
	if err != nil {
		t.Fatalf("MapTriplesToOASF() error = %v", err)
	}

	if len(record.Skills) == 0 {
		t.Fatal("expected at least one skill")
	}

	skill := record.Skills[0]
	if len(skill.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(skill.Permissions))
	}
}

func TestMapper_MapTriplesToOASF_WithIntent(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, true)

	agentID := "acme.ops.agentic.system.agent.analyst"
	triples := []message.Triple{
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityName,
			Object:    "Data Analysis",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.IntentGoal,
			Object:    "Analyze data and provide insights",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.IntentType,
			Object:    "data-analysis",
			Source:    "test",
			Timestamp: time.Now(),
		},
	}

	record, err := mapper.MapTriplesToOASF(agentID, triples)
	if err != nil {
		t.Fatalf("MapTriplesToOASF() error = %v", err)
	}

	if record.Description != "Analyze data and provide insights" {
		t.Errorf("expected description from intent goal, got %q", record.Description)
	}

	if len(record.Domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(record.Domains))
	}

	if record.Domains[0].Name != "data-analysis" {
		t.Errorf("expected domain 'data-analysis', got %q", record.Domains[0].Name)
	}
}

func TestMapper_MapTriplesToOASF_WithExtensions(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, true)

	agentID := "acme.ops.agentic.system.agent.builder"
	triples := []message.Triple{
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityName,
			Object:    "Build",
			Source:    "test",
			Timestamp: time.Now(),
		},
		{
			Subject:   agentID,
			Predicate: agentic.ActionType,
			Object:    "tool-call",
			Source:    "test",
			Timestamp: time.Now(),
		},
	}

	record, err := mapper.MapTriplesToOASF(agentID, triples)
	if err != nil {
		t.Fatalf("MapTriplesToOASF() error = %v", err)
	}

	if record.Extensions == nil {
		t.Fatal("expected extensions to be set")
	}

	if record.Extensions["semstreams_entity_id"] != agentID {
		t.Errorf("expected semstreams_entity_id extension, got %v", record.Extensions["semstreams_entity_id"])
	}

	if record.Extensions["source"] != "semstreams" {
		t.Errorf("expected source extension 'semstreams', got %v", record.Extensions["source"])
	}
}

func TestMapper_MapTriplesToOASF_NoExtensions(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, false)

	agentID := "acme.ops.agentic.system.agent.simple"
	triples := []message.Triple{
		{
			Subject:   agentID,
			Predicate: agentic.CapabilityName,
			Object:    "Simple Task",
			Source:    "test",
			Timestamp: time.Now(),
		},
	}

	record, err := mapper.MapTriplesToOASF(agentID, triples)
	if err != nil {
		t.Fatalf("MapTriplesToOASF() error = %v", err)
	}

	// Extensions should not include semstreams-specific fields
	if record.Extensions != nil && record.Extensions["semstreams_entity_id"] != nil {
		t.Error("expected no semstreams extensions when disabled")
	}
}

func TestMapper_MapTriplesToOASF_EmptyTriples(t *testing.T) {
	mapper := NewMapper("1.0.0", []string{"system"}, true)

	_, err := mapper.MapTriplesToOASF("test.entity", nil)
	if err == nil {
		t.Error("expected error for empty triples")
	}

	_, err = mapper.MapTriplesToOASF("test.entity", []message.Triple{})
	if err == nil {
		t.Error("expected error for empty triples slice")
	}
}

func TestExtractAgentName(t *testing.T) {
	tests := []struct {
		entityID string
		want     string
	}{
		{"acme.ops.agentic.system.agent.architect", "agent-architect"},
		{"org.platform.domain.system.type.instance", "type-instance"},
		{"simple.entity", "simple-entity"},
		{"single", "single"},
	}

	for _, tt := range tests {
		t.Run(tt.entityID, func(t *testing.T) {
			got := extractAgentName(tt.entityID)
			if got != tt.want {
				t.Errorf("extractAgentName(%q) = %q, want %q", tt.entityID, got, tt.want)
			}
		})
	}
}

func TestGenerateSkillID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Code Review", "code-review"},
		{"software_design", "software-design"},
		{"UPPERCASE", "uppercase"},
		{"Mixed Case Name", "mixed-case-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSkillID(tt.name)
			if got != tt.want {
				t.Errorf("generateSkillID(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSupportedPredicates(t *testing.T) {
	predicates := SupportedPredicates()

	if len(predicates) == 0 {
		t.Error("expected supported predicates to be returned")
	}

	// Verify some key predicates are included
	expected := []string{
		agentic.CapabilityName,
		agentic.CapabilityDescription,
		agentic.IntentGoal,
	}

	for _, exp := range expected {
		found := false
		for _, pred := range predicates {
			if pred == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected predicate %q to be in supported list", exp)
		}
	}
}
