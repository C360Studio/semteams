package oasfgenerator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/c360studio/semstreams/message"
	agentic "github.com/c360studio/semstreams/vocabulary/agentic"
)

// Mapper converts SemStreams triples to OASF records.
// It maps agent.capability.*, agent.intent.*, and agent.action.* predicates
// to the OASF skills and domains format.
type Mapper struct {
	// defaultVersion is used when no version is specified.
	defaultVersion string

	// defaultAuthors is used when no authors are specified.
	defaultAuthors []string

	// includeExtensions enables SemStreams-specific extensions.
	includeExtensions bool
}

// NewMapper creates a new OASF mapper.
func NewMapper(defaultVersion string, defaultAuthors []string, includeExtensions bool) *Mapper {
	return &Mapper{
		defaultVersion:    defaultVersion,
		defaultAuthors:    defaultAuthors,
		includeExtensions: includeExtensions,
	}
}

// mappingContext holds the state while mapping triples.
type mappingContext struct {
	record                  *OASFRecord
	skillsByExpression      map[string]*OASFSkill
	domainsByName           map[string]*OASFDomain
	permissionsByExpression map[string][]string
}

// MapTriplesToOASF converts a set of triples for an agent entity into an OASF record.
// The triples should all be about the same agent entity (same subject).
func (m *Mapper) MapTriplesToOASF(agentID string, triples []message.Triple) (*OASFRecord, error) {
	if len(triples) == 0 {
		return nil, fmt.Errorf("no triples provided")
	}

	// Initialize mapping context
	ctx := &mappingContext{
		record:                  NewOASFRecord(extractAgentName(agentID), m.defaultVersion, ""),
		skillsByExpression:      make(map[string]*OASFSkill),
		domainsByName:           make(map[string]*OASFDomain),
		permissionsByExpression: make(map[string][]string),
	}

	// First pass: create skills from expressions and names
	m.extractSkills(ctx, triples)

	// Second pass: apply additional properties
	m.applyTripleProperties(ctx, triples)

	// Finalize the record
	m.finalizeRecord(ctx, agentID)

	return ctx.record, nil
}

// extractSkills creates skills from capability expressions and names (first pass).
func (m *Mapper) extractSkills(ctx *mappingContext, triples []message.Triple) {
	for _, triple := range triples {
		switch triple.Predicate {
		case agentic.CapabilityExpression:
			expr := toString(triple.Object)
			tripleCtx := triple.Context
			if tripleCtx == "" {
				tripleCtx = expr
			}
			skill := m.getOrCreateSkill(ctx.skillsByExpression, tripleCtx)
			skill.ID = expr

		case agentic.CapabilityName:
			name := toString(triple.Object)
			tripleCtx := triple.Context
			if tripleCtx == "" {
				tripleCtx = name
			}
			skill := m.getOrCreateSkill(ctx.skillsByExpression, tripleCtx)
			skill.Name = name
		}
	}
}

// applyTripleProperties applies additional properties from triples (second pass).
func (m *Mapper) applyTripleProperties(ctx *mappingContext, triples []message.Triple) {
	for _, triple := range triples {
		tripleCtx := triple.Context
		if tripleCtx == "" {
			// Use first skill if no context
			for key := range ctx.skillsByExpression {
				tripleCtx = key
				break
			}
		}

		switch triple.Predicate {
		case agentic.CapabilityDescription:
			desc := toString(triple.Object)
			skill := m.findSkillForContext(ctx.skillsByExpression, tripleCtx)
			if skill != nil {
				skill.Description = desc
			}

		case agentic.CapabilityConfidence:
			conf := toFloat64(triple.Object)
			skill := m.findSkillForContext(ctx.skillsByExpression, tripleCtx)
			if skill != nil {
				skill.Confidence = conf
			}

		case agentic.CapabilityPermission:
			perm := toString(triple.Object)
			ctx.permissionsByExpression[tripleCtx] = append(ctx.permissionsByExpression[tripleCtx], perm)

		case agentic.IntentGoal:
			goal := toString(triple.Object)
			if ctx.record.Description == "" {
				ctx.record.Description = goal
			}

		case agentic.IntentType:
			intentType := toString(triple.Object)
			m.getOrCreateDomain(ctx.domainsByName, intentType)

		case agentic.ActionType:
			if m.includeExtensions {
				actionType := toString(triple.Object)
				ctx.record.SetExtension("action_types", appendUnique(
					toStringSlice(ctx.record.Extensions["action_types"]),
					actionType,
				))
			}
		}
	}
}

// finalizeRecord applies final transformations to the record.
func (m *Mapper) finalizeRecord(ctx *mappingContext, agentID string) {
	// Apply permissions to skills
	for expr, perms := range ctx.permissionsByExpression {
		if skill, ok := ctx.skillsByExpression[expr]; ok {
			skill.Permissions = perms
		}
	}

	// Convert skill map to slice
	for _, skill := range ctx.skillsByExpression {
		if skill.ID == "" {
			skill.ID = generateSkillID(skill.Name)
		}
		ctx.record.AddSkill(*skill)
	}

	// Convert domain map to slice
	for _, domain := range ctx.domainsByName {
		ctx.record.AddDomain(*domain)
	}

	// Add default authors if none specified
	if len(ctx.record.Authors) == 0 {
		ctx.record.Authors = m.defaultAuthors
	}

	// Add SemStreams extensions if enabled
	if m.includeExtensions {
		ctx.record.SetExtension("semstreams_entity_id", agentID)
		ctx.record.SetExtension("source", "semstreams")
	}
}

// getOrCreateSkill gets or creates a skill by expression/name.
func (m *Mapper) getOrCreateSkill(skills map[string]*OASFSkill, key string) *OASFSkill {
	if skill, ok := skills[key]; ok {
		return skill
	}
	skill := &OASFSkill{
		ID:         key,
		Name:       key,
		Confidence: 1.0, // Default confidence
	}
	skills[key] = skill
	return skill
}

// findSkillForContext finds a skill matching the triple context.
// If no context or no match, returns the first skill or nil.
func (m *Mapper) findSkillForContext(skills map[string]*OASFSkill, context string) *OASFSkill {
	if context != "" {
		if skill, ok := skills[context]; ok {
			return skill
		}
	}
	// Return first skill if any
	for _, skill := range skills {
		return skill
	}
	return nil
}

// getOrCreateDomain gets or creates a domain by name.
func (m *Mapper) getOrCreateDomain(domains map[string]*OASFDomain, name string) *OASFDomain {
	if domain, ok := domains[name]; ok {
		return domain
	}
	domain := &OASFDomain{
		Name: name,
	}
	domains[name] = domain
	return domain
}

// extractAgentName extracts a human-readable name from an entity ID.
// Entity ID format: org.platform.domain.system.type.instance
func extractAgentName(entityID string) string {
	parts := strings.Split(entityID, ".")
	if len(parts) >= 6 {
		// Use type.instance as name (e.g., "agent.architect")
		return parts[4] + "-" + parts[5]
	}
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "-" + parts[len(parts)-1]
	}
	return entityID
}

// generateSkillID generates a unique skill ID from a name.
func generateSkillID(name string) string {
	// Convert to lowercase, replace spaces with hyphens
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// toString converts any value to a string.
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toFloat64 converts any value to a float64.
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// toStringSlice converts any value to a string slice.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = toString(item)
		}
		return result
	default:
		return nil
	}
}

// appendUnique appends a value to a slice if not already present.
func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

// PredicateMapping defines how SemStreams predicates map to OASF fields.
// This table is for documentation and validation purposes.
var PredicateMapping = map[string]string{
	// Capability predicates -> Skills
	agentic.CapabilityName:        "skills[].name",
	agentic.CapabilityDescription: "skills[].description",
	agentic.CapabilityExpression:  "skills[].id",
	agentic.CapabilityConfidence:  "skills[].confidence",
	agentic.CapabilityPermission:  "skills[].permissions[]",

	// Intent predicates -> Description and Domains
	agentic.IntentGoal: "description",
	agentic.IntentType: "domains[].name",

	// Action predicates -> Extensions
	agentic.ActionType: "extensions.action_types[]",
}

// SupportedPredicates returns the list of predicates this mapper handles.
func SupportedPredicates() []string {
	predicates := make([]string, 0, len(PredicateMapping))
	for pred := range PredicateMapping {
		predicates = append(predicates, pred)
	}
	return predicates
}
