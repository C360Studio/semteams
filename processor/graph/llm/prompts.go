// Package llm provides LLM client and prompt templates for graph processing.
package llm

import (
	"bytes"
	"encoding/json"
	"os"
	"text/template"
)

// Prompt templates - package variables, directly accessible.
// These can be overridden via LoadPromptsFromFile.
var (
	// CommunityPrompt is the prompt template for community summarization.
	CommunityPrompt = PromptTemplate{
		System: `You are an analyst summarizing communities of related entities.

Entity IDs follow a 6-part federated notation:
  {org}.{platform}.{domain}.{system}.{type}.{instance}

Parts:
- org: Organization identifier (multi-tenancy)
- platform: Platform/product within organization
- domain: Business domain (e.g., environmental, content, logistics)
- system: System or subsystem (e.g., sensor, document, device)
- type: Entity type within system (e.g., temperature, manual, humidity)
- instance: Unique instance identifier

Generate concise summaries (1-2 sentences) that leverage this structure.
For environmental domains: emphasize monitoring scope and measurements.
For content domains: emphasize topics, themes, and knowledge areas.
For mixed domains: describe relationships between different entity types.`,

		UserFormat: `Summarize this community of {{.EntityCount}} entities:

{{if .OrgPlatform}}Organization/Platform: {{.OrgPlatform}}
{{end}}Dominant domain: {{.DominantDomain}}

Entities by domain:
{{range .Domains}}- {{.Domain}} ({{.Count}} entities):
{{range .SystemTypes}}  - {{.Name}}: {{.Count}}
{{end}}{{end}}
Key themes: {{.Keywords}}

Sample entities (parsed):
{{range .SampleEntities}}- {{.Full}}
  org={{.Org}} platform={{.Platform}} domain={{.Domain}} system={{.System}} type={{.Type}} instance={{.Instance}}
{{if .Title}}  title: {{.Title}}{{end}}
{{if .Abstract}}  description: {{.Abstract}}{{end}}
{{end}}
Generate a concise summary describing what this community represents.`,
	}

	// SearchPrompt is the prompt template for GraphRAG search answer generation.
	SearchPrompt = PromptTemplate{
		System: `You are a helpful assistant that answers questions based on entity graph context.
Use the provided community summaries and entity information to answer the user's question.
Be concise and factual. If the information is insufficient, say so.`,

		UserFormat: `Question: {{.Query}}

Relevant communities:
{{range .Communities}}- {{.Summary}} ({{.EntityCount}} entities, keywords: {{.Keywords}})
{{end}}
Top matching entities:
{{range .Entities}}- {{.ID}} ({{.Type}}){{if .Name}}: {{.Name}}{{end}}
{{if .Description}}  {{.Description}}{{end}}
{{end}}
Based on the above context, answer the question concisely.`,
	}

	// EntityPrompt is the prompt template for single entity descriptions.
	EntityPrompt = PromptTemplate{
		System: `You are a helpful assistant that describes entities in a knowledge graph.
Generate clear, informative descriptions based on the entity's properties and relationships.`,

		UserFormat: `Describe this entity:

ID: {{.ID}}
Type: {{.Type}}

Properties:
{{range .Properties}}- {{.Predicate}}: {{.Value}}
{{end}}
Relationships:
{{range .Relationships}}- {{.Predicate}} -> {{.Target}}
{{end}}
Generate a brief description of this entity.`,
	}
)

// PromptTemplate defines a reusable prompt template.
type PromptTemplate struct {
	// System is the system message that sets assistant behavior.
	System string `json:"system"`

	// UserFormat is a Go text/template for the user message.
	UserFormat string `json:"user_format"`
}

// Render executes the template with the given data.
func (p PromptTemplate) Render(data any) (*RenderedPrompt, error) {
	t, err := template.New("user").Parse(p.UserFormat)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return &RenderedPrompt{
		System: p.System,
		User:   buf.String(),
	}, nil
}

// RenderedPrompt contains the rendered system and user messages.
type RenderedPrompt struct {
	System string
	User   string
}

// LoadPromptsFromFile overrides prompts from a JSON file.
// File format: {"community_summary": {...}, "search_answer": {...}, "entity_description": {...}}
func LoadPromptsFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var overrides map[string]PromptTemplate
	if err := json.Unmarshal(data, &overrides); err != nil {
		return err
	}
	if p, ok := overrides["community_summary"]; ok {
		CommunityPrompt = p
	}
	if p, ok := overrides["search_answer"]; ok {
		SearchPrompt = p
	}
	if p, ok := overrides["entity_description"]; ok {
		EntityPrompt = p
	}
	return nil
}

// EntityContent contains description fields fetched via NATS from ObjectStore.
// Used to enrich LLM prompts with actual entity titles and descriptions.
type EntityContent struct {
	Title    string // From ContentRole "title"
	Abstract string // From ContentRole "abstract" or description
}

// EntityParts represents parsed 6-part entity ID components.
// Entity IDs follow the pattern: {org}.{platform}.{domain}.{system}.{type}.{instance}
type EntityParts struct {
	Full     string // Complete entity ID
	Org      string // Part 0: Organization
	Platform string // Part 1: Platform
	Domain   string // Part 2: Business domain
	System   string // Part 3: System/subsystem
	Type     string // Part 4: Entity type
	Instance string // Part 5: Instance ID
	Title    string // Entity title from content store (optional)
	Abstract string // Entity abstract from content store (optional)
}

// DomainGroup groups entities by their domain (part[2] of entity ID).
type DomainGroup struct {
	Domain      string       // e.g., "environmental", "content"
	Count       int          // Total entities in domain
	SystemTypes []SystemType // system.type breakdown
}

// SystemType represents a system.type combination count.
type SystemType struct {
	Name  string // e.g., "sensor.temperature"
	Count int
}

// CommunitySummaryData is the data structure for community_summary prompts.
type CommunitySummaryData struct {
	EntityCount    int
	Domains        []DomainGroup // Grouped by domain from entity ID part[2]
	DominantDomain string        // Most common domain, or "mixed"
	OrgPlatform    string        // Common org.platform if uniform
	Keywords       string
	SampleEntities []EntityParts // Parsed entity samples
}

// SearchAnswerData is the data structure for search_answer prompts.
type SearchAnswerData struct {
	Query       string
	Communities []CommunitySummaryInfo
	Entities    []EntitySample
}

// CommunitySummaryInfo contains community info for search prompts.
type CommunitySummaryInfo struct {
	Summary     string
	EntityCount int
	Keywords    string
}

// EntitySample represents a sample entity for search prompts.
type EntitySample struct {
	ID          string
	Type        string
	Name        string
	Description string // From content store abstract (optional)
}

// EntityDescriptionData is the data structure for entity_description prompts.
type EntityDescriptionData struct {
	ID            string
	Type          string
	Properties    []PropertyInfo
	Relationships []RelationshipInfo
}

// PropertyInfo represents a property for prompts.
type PropertyInfo struct {
	Predicate string
	Value     string
}

// RelationshipInfo represents a relationship for prompts.
type RelationshipInfo struct {
	Predicate string
	Target    string
}
