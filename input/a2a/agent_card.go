package a2a

import (
	"encoding/json"
	"fmt"
)

// AgentCard represents an A2A Agent Card.
// This is the publicly-accessible description of an agent's capabilities.
type AgentCard struct {
	// Name is the agent's display name.
	Name string `json:"name"`

	// Description describes what the agent does.
	Description string `json:"description"`

	// URL is the agent's A2A endpoint.
	URL string `json:"url"`

	// Version is the agent card schema version.
	Version string `json:"version"`

	// Provider contains information about the agent provider.
	Provider *Provider `json:"provider,omitempty"`

	// Capabilities lists what the agent can do.
	Capabilities []Capability `json:"capabilities"`

	// Authentication describes supported auth methods.
	Authentication *Authentication `json:"authentication,omitempty"`

	// DefaultInputModes lists supported input types.
	DefaultInputModes []string `json:"defaultInputModes,omitempty"`

	// DefaultOutputModes lists supported output types.
	DefaultOutputModes []string `json:"defaultOutputModes,omitempty"`

	// Skills lists specific skills the agent has.
	Skills []Skill `json:"skills,omitempty"`
}

// Provider contains information about the agent provider.
type Provider struct {
	// Organization is the provider's organization name.
	Organization string `json:"organization"`

	// URL is the provider's website.
	URL string `json:"url,omitempty"`
}

// Capability describes a capability the agent has.
type Capability struct {
	// Name is the capability name.
	Name string `json:"name"`

	// Description describes the capability.
	Description string `json:"description,omitempty"`
}

// Authentication describes supported authentication methods.
type Authentication struct {
	// Schemes lists supported auth schemes.
	Schemes []string `json:"schemes"` // e.g., ["did", "bearer"]

	// Credentials contains credential information.
	Credentials *Credentials `json:"credentials,omitempty"`
}

// Credentials contains credential configuration.
type Credentials struct {
	// DID is the agent's DID.
	DID string `json:"did,omitempty"`

	// PublicKeyJWK is the agent's public key in JWK format.
	PublicKeyJWK json.RawMessage `json:"publicKeyJwk,omitempty"`
}

// Skill describes a specific skill from the OASF record.
type Skill struct {
	// ID is the skill identifier.
	ID string `json:"id"`

	// Name is the skill display name.
	Name string `json:"name"`

	// Description describes what the skill does.
	Description string `json:"description,omitempty"`

	// InputSchema describes the expected input format.
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`

	// OutputSchema describes the output format.
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

// OASFRecord represents an OASF record (from processor/oasf-generator).
// This is used to generate agent cards.
type OASFRecord struct {
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	SchemaVersion string      `json:"schema_version"`
	Authors       []string    `json:"authors,omitempty"`
	CreatedAt     string      `json:"created_at"`
	Description   string      `json:"description"`
	Skills        []OASFSkill `json:"skills,omitempty"`
	Domains       []string    `json:"domains,omitempty"`
}

// OASFSkill represents a skill in an OASF record.
type OASFSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// AgentCardGenerator generates A2A agent cards from OASF records.
type AgentCardGenerator struct {
	// BaseURL is the base URL for the agent's A2A endpoint.
	BaseURL string

	// ProviderOrg is the provider organization name.
	ProviderOrg string

	// ProviderURL is the provider's website URL.
	ProviderURL string

	// AgentDID is the agent's DID for authentication.
	AgentDID string
}

// NewAgentCardGenerator creates a new agent card generator.
func NewAgentCardGenerator(baseURL, providerOrg string) *AgentCardGenerator {
	return &AgentCardGenerator{
		BaseURL:     baseURL,
		ProviderOrg: providerOrg,
	}
}

// GenerateFromOASF generates an agent card from an OASF record.
func (g *AgentCardGenerator) GenerateFromOASF(record *OASFRecord) (*AgentCard, error) {
	if record == nil {
		return nil, fmt.Errorf("OASF record cannot be nil")
	}

	card := &AgentCard{
		Name:               record.Name,
		Description:        record.Description,
		URL:                g.BaseURL,
		Version:            "1.0",
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}

	// Add provider if configured
	if g.ProviderOrg != "" {
		card.Provider = &Provider{
			Organization: g.ProviderOrg,
			URL:          g.ProviderURL,
		}
	}

	// Convert OASF skills to A2A capabilities and skills
	card.Capabilities = g.convertCapabilities(record.Skills)
	card.Skills = g.convertSkills(record.Skills)

	// Add authentication if DID is configured
	if g.AgentDID != "" {
		card.Authentication = &Authentication{
			Schemes: []string{"did"},
			Credentials: &Credentials{
				DID: g.AgentDID,
			},
		}
	}

	return card, nil
}

// convertCapabilities converts OASF skills to A2A capabilities.
func (g *AgentCardGenerator) convertCapabilities(skills []OASFSkill) []Capability {
	capabilities := make([]Capability, 0, len(skills))
	seen := make(map[string]bool)

	for _, skill := range skills {
		// Use skill name as capability, deduplicated
		name := skill.Name
		if name == "" {
			name = skill.ID
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		capabilities = append(capabilities, Capability{
			Name:        name,
			Description: skill.Description,
		})
	}

	return capabilities
}

// convertSkills converts OASF skills to A2A skills.
func (g *AgentCardGenerator) convertSkills(oasfSkills []OASFSkill) []Skill {
	skills := make([]Skill, 0, len(oasfSkills))

	for _, oasf := range oasfSkills {
		skill := Skill{
			ID:          oasf.ID,
			Name:        oasf.Name,
			Description: oasf.Description,
		}

		// Could add input/output schemas based on OASF permissions
		// For now, leave them empty

		skills = append(skills, skill)
	}

	return skills
}

// SerializeAgentCard serializes an agent card to JSON.
func SerializeAgentCard(card *AgentCard) ([]byte, error) {
	return json.MarshalIndent(card, "", "  ")
}

// ParseAgentCard parses a JSON agent card.
func ParseAgentCard(data []byte) (*AgentCard, error) {
	var card AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, fmt.Errorf("parse agent card: %w", err)
	}
	return &card, nil
}
