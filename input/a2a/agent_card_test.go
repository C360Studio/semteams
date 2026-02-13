package a2a

import (
	"encoding/json"
	"testing"
)

func TestNewAgentCardGenerator(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")

	if gen == nil {
		t.Fatal("expected generator, got nil")
	}

	if gen.BaseURL != "http://localhost:8080" {
		t.Errorf("expected BaseURL 'http://localhost:8080', got %q", gen.BaseURL)
	}

	if gen.ProviderOrg != "TestOrg" {
		t.Errorf("expected ProviderOrg 'TestOrg', got %q", gen.ProviderOrg)
	}
}

func TestAgentCardGeneratorGenerateFromOASF(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")
	gen.ProviderURL = "https://testorg.com"
	gen.AgentDID = "did:key:test123"

	record := &OASFRecord{
		Name:          "Test Agent",
		Version:       "1.0.0",
		SchemaVersion: "1.0.0",
		Description:   "A test agent for unit testing",
		Skills: []OASFSkill{
			{
				ID:          "skill-1",
				Name:        "code-analysis",
				Description: "Analyzes source code",
			},
			{
				ID:          "skill-2",
				Name:        "code-generation",
				Description: "Generates code",
			},
		},
		Domains: []string{"software-development"},
	}

	card, err := gen.GenerateFromOASF(record)
	if err != nil {
		t.Fatalf("GenerateFromOASF failed: %v", err)
	}

	// Verify basic fields
	if card.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %q", card.Name)
	}

	if card.Description != "A test agent for unit testing" {
		t.Errorf("unexpected description: %q", card.Description)
	}

	if card.URL != "http://localhost:8080" {
		t.Errorf("expected URL 'http://localhost:8080', got %q", card.URL)
	}

	// Verify provider
	if card.Provider == nil {
		t.Fatal("expected provider, got nil")
	}

	if card.Provider.Organization != "TestOrg" {
		t.Errorf("expected org 'TestOrg', got %q", card.Provider.Organization)
	}

	if card.Provider.URL != "https://testorg.com" {
		t.Errorf("expected provider URL, got %q", card.Provider.URL)
	}

	// Verify capabilities
	if len(card.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(card.Capabilities))
	}

	// Verify skills
	if len(card.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(card.Skills))
	}

	// Verify authentication
	if card.Authentication == nil {
		t.Fatal("expected authentication, got nil")
	}

	if len(card.Authentication.Schemes) != 1 || card.Authentication.Schemes[0] != "did" {
		t.Errorf("expected 'did' auth scheme")
	}

	if card.Authentication.Credentials.DID != "did:key:test123" {
		t.Errorf("expected DID in credentials")
	}
}

func TestAgentCardGeneratorGenerateFromOASFNilRecord(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")

	_, err := gen.GenerateFromOASF(nil)
	if err == nil {
		t.Error("expected error for nil record")
	}
}

func TestAgentCardGeneratorNoProvider(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "")

	record := &OASFRecord{
		Name:        "Test Agent",
		Description: "A test agent",
	}

	card, err := gen.GenerateFromOASF(record)
	if err != nil {
		t.Fatalf("GenerateFromOASF failed: %v", err)
	}

	if card.Provider != nil {
		t.Error("expected nil provider when ProviderOrg is empty")
	}
}

func TestAgentCardGeneratorNoAuthentication(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")
	// AgentDID is empty

	record := &OASFRecord{
		Name:        "Test Agent",
		Description: "A test agent",
	}

	card, err := gen.GenerateFromOASF(record)
	if err != nil {
		t.Fatalf("GenerateFromOASF failed: %v", err)
	}

	if card.Authentication != nil {
		t.Error("expected nil authentication when AgentDID is empty")
	}
}

func TestSerializeAgentCard(t *testing.T) {
	card := &AgentCard{
		Name:        "Test Agent",
		Description: "A test agent",
		URL:         "http://localhost:8080",
		Version:     "1.0",
		Capabilities: []Capability{
			{Name: "test", Description: "Test capability"},
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}

	data, err := SerializeAgentCard(card)
	if err != nil {
		t.Fatalf("SerializeAgentCard failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded AgentCard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to decode serialized card: %v", err)
	}

	if decoded.Name != card.Name {
		t.Errorf("name mismatch")
	}
}

func TestParseAgentCard(t *testing.T) {
	cardJSON := `{
		"name": "Test Agent",
		"description": "A test agent",
		"url": "http://localhost:8080",
		"version": "1.0",
		"capabilities": [
			{"name": "test", "description": "Test capability"}
		],
		"defaultInputModes": ["text"],
		"defaultOutputModes": ["text"]
	}`

	card, err := ParseAgentCard([]byte(cardJSON))
	if err != nil {
		t.Fatalf("ParseAgentCard failed: %v", err)
	}

	if card.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %q", card.Name)
	}

	if len(card.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(card.Capabilities))
	}
}

func TestParseAgentCardInvalid(t *testing.T) {
	_, err := ParseAgentCard([]byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestConvertCapabilitiesDeduplication(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")

	skills := []OASFSkill{
		{ID: "skill-1", Name: "analysis"},
		{ID: "skill-2", Name: "analysis"}, // Duplicate name
		{ID: "skill-3", Name: "generation"},
	}

	caps := gen.convertCapabilities(skills)

	// Should deduplicate by name
	if len(caps) != 2 {
		t.Errorf("expected 2 unique capabilities, got %d", len(caps))
	}
}

func TestConvertSkillsUseIDWhenNameEmpty(t *testing.T) {
	gen := NewAgentCardGenerator("http://localhost:8080", "TestOrg")

	skills := []OASFSkill{
		{ID: "skill-1", Name: ""}, // Empty name
	}

	caps := gen.convertCapabilities(skills)

	if len(caps) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(caps))
	}

	// Should use ID when name is empty
	if caps[0].Name != "skill-1" {
		t.Errorf("expected capability name to be ID 'skill-1', got %q", caps[0].Name)
	}
}

func TestOASFRecordSerialization(t *testing.T) {
	record := OASFRecord{
		Name:          "Test Agent",
		Version:       "1.0.0",
		SchemaVersion: "1.0.0",
		Authors:       []string{"author1", "author2"},
		CreatedAt:     "2026-02-13T00:00:00Z",
		Description:   "A test agent",
		Skills: []OASFSkill{
			{
				ID:          "skill-1",
				Name:        "test-skill",
				Description: "A test skill",
				Confidence:  0.9,
				Permissions: []string{"read", "write"},
			},
		},
		Domains: []string{"testing"},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded OASFRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != record.Name {
		t.Errorf("Name mismatch")
	}
	if len(decoded.Skills) != 1 {
		t.Errorf("Skills count mismatch")
	}
	if decoded.Skills[0].Confidence != 0.9 {
		t.Errorf("Skill confidence mismatch")
	}
}
