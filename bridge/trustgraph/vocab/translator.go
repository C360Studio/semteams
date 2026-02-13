package vocab

import (
	"net/url"
	"strings"
	"time"

	"github.com/c360studio/semstreams/message"
)

// TGTriple is TrustGraph's compact triple representation.
// This mirrors the JSON format used in TrustGraph REST APIs.
type TGTriple struct {
	S TGValue `json:"s"`
	P TGValue `json:"p"`
	O TGValue `json:"o"`
}

// TGValue is a compact value with entity flag.
type TGValue struct {
	V string `json:"v"` // Value (URI or literal)
	E bool   `json:"e"` // Is entity (true = URI, false = literal)
}

// URIMapping configures how URIs from a specific domain translate to SemStreams entity IDs.
type URIMapping struct {
	// Org is the SemStreams org segment
	Org string `json:"org"`

	// Platform is the default platform (if URI has < 6 path segments)
	Platform string `json:"platform"`

	// Domain is the default domain
	Domain string `json:"domain"`

	// System is the default system
	System string `json:"system"`

	// Type is the default type
	Type string `json:"type"`
}

// TranslatorConfig configures the vocabulary translator.
type TranslatorConfig struct {
	// OrgMappings maps SemStreams org segment to base URI.
	// Example: "acme" → "https://data.acme-corp.com/"
	OrgMappings map[string]string `json:"org_mappings"`

	// URIMappings maps URI domain to SemStreams org segment and defaults.
	// Example: "trustgraph.ai" → URIMapping{Org: "trustgraph", ...}
	URIMappings map[string]URIMapping `json:"uri_mappings"`

	// PredicateMappings for the predicate map.
	PredicateMappings map[string]string `json:"predicate_mappings"`

	// DefaultOrg for URIs with unmapped domains.
	DefaultOrg string `json:"default_org"`

	// DefaultURIBase for entities with unmapped orgs.
	DefaultURIBase string `json:"default_uri_base"`

	// DefaultPlatform for URIs with insufficient path segments.
	DefaultPlatform string `json:"default_platform"`

	// DefaultDomain for URIs with insufficient path segments.
	DefaultDomain string `json:"default_domain"`

	// DefaultSystem for URIs with insufficient path segments.
	DefaultSystem string `json:"default_system"`

	// DefaultType for URIs with insufficient path segments.
	DefaultType string `json:"default_type"`
}

// Translator converts between SemStreams entity IDs and RDF URIs.
type Translator struct {
	// OrgMappings maps org segment to base URI
	OrgMappings map[string]string

	// URIMappings maps URI domain to org segment + defaults
	URIMappings map[string]URIMapping

	// PredicateMap handles predicate translation
	PredicateMap *PredicateMap

	// DefaultOrg for URIs with unmapped domains
	DefaultOrg string

	// DefaultURIBase for entities with unmapped orgs
	DefaultURIBase string

	// Default values for incomplete URIs
	DefaultPlatform string
	DefaultDomain   string
	DefaultSystem   string
	DefaultType     string
}

// NewTranslator creates a translator with the given configuration.
func NewTranslator(cfg TranslatorConfig) *Translator {
	// Apply defaults
	defaultOrg := cfg.DefaultOrg
	if defaultOrg == "" {
		defaultOrg = "external"
	}

	defaultURIBase := cfg.DefaultURIBase
	if defaultURIBase == "" {
		defaultURIBase = "http://semstreams.io/e/"
	}

	defaultPlatform := cfg.DefaultPlatform
	if defaultPlatform == "" {
		defaultPlatform = "default"
	}

	defaultDomain := cfg.DefaultDomain
	if defaultDomain == "" {
		defaultDomain = "knowledge"
	}

	defaultSystem := cfg.DefaultSystem
	if defaultSystem == "" {
		defaultSystem = "entity"
	}

	defaultType := cfg.DefaultType
	if defaultType == "" {
		defaultType = "concept"
	}

	return &Translator{
		OrgMappings:     cfg.OrgMappings,
		URIMappings:     cfg.URIMappings,
		PredicateMap:    NewPredicateMap(cfg.PredicateMappings, defaultURIBase+"predicate/"),
		DefaultOrg:      defaultOrg,
		DefaultURIBase:  defaultURIBase,
		DefaultPlatform: defaultPlatform,
		DefaultDomain:   defaultDomain,
		DefaultSystem:   defaultSystem,
		DefaultType:     defaultType,
	}
}

// EntityIDToURI converts a SemStreams 6-part entity ID to an RDF URI.
//
// Format: org.platform.domain.system.type.instance
// Result: http://{org-base}/platform/domain/system/type/instance
//
// If the org has a configured mapping, that base URI is used.
// Otherwise, the org becomes the domain: http://{org}.org/...
//
// Example:
//
//	"acme.ops.environmental.sensor.temperature.sensor-042"
//	→ "http://acme.org/ops/environmental/sensor/temperature/sensor-042"
//
// With OrgMappings["acme"] = "https://data.acme-corp.com/":
//
//	→ "https://data.acme-corp.com/ops/environmental/sensor/temperature/sensor-042"
func (t *Translator) EntityIDToURI(entityID string) string {
	parts := strings.Split(entityID, ".")
	if len(parts) < 2 {
		// Not a valid entity ID, return as-is wrapped in default base
		return t.DefaultURIBase + url.PathEscape(entityID)
	}

	org := parts[0]
	var baseURI string

	// Check for org mapping
	if mapped, ok := t.OrgMappings[org]; ok {
		baseURI = mapped
	} else {
		// Default: org becomes the domain
		baseURI = "http://" + org + ".org/"
	}

	// Ensure base ends with /
	if !strings.HasSuffix(baseURI, "/") {
		baseURI += "/"
	}

	// Build path from remaining segments
	pathParts := parts[1:]
	escapedParts := make([]string, len(pathParts))
	for i, p := range pathParts {
		escapedParts[i] = url.PathEscape(p)
	}

	return baseURI + strings.Join(escapedParts, "/")
}

// URIToEntityID converts an RDF URI to a SemStreams 6-part entity ID.
//
// The translation extracts the domain from the URI and looks up configured mappings.
// If the URI has fewer than 6 path segments, defaults are filled in.
//
// Example:
//
//	"http://trustgraph.ai/e/supply-chain-risk"
//	→ "trustgraph.default.knowledge.entity.concept.supply-chain-risk"
//
// With URIMappings["trustgraph.ai"] = {Org: "client", Platform: "intel", ...}:
//
//	→ "client.intel.knowledge.trustgraph.entity.supply-chain-risk"
func (t *Translator) URIToEntityID(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		// If we can't parse, sanitize and return with defaults
		return t.buildEntityID(t.DefaultOrg, t.DefaultPlatform, t.DefaultDomain, t.DefaultSystem, t.DefaultType, sanitizeInstance(uri))
	}

	// Extract host (domain)
	host := parsed.Host
	if host == "" {
		host = t.DefaultOrg
	}

	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}

	// Look up URI mapping
	var mapping URIMapping
	var hasMapping bool
	if t.URIMappings != nil {
		mapping, hasMapping = t.URIMappings[host]
	}

	// Determine org
	org := t.DefaultOrg
	if hasMapping && mapping.Org != "" {
		org = mapping.Org
	} else {
		// Use first part of host as org
		hostParts := strings.Split(host, ".")
		org = sanitizeSegment(hostParts[0])
	}

	// Extract path segments
	path := strings.TrimPrefix(parsed.Path, "/")

	// Remove common prefixes like /e/, /entity/, etc.
	for _, prefix := range []string{"e/", "entity/", "entities/", "resource/"} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}

	pathParts := strings.Split(path, "/")

	// Filter empty parts
	var segments []string
	for _, p := range pathParts {
		decoded, _ := url.PathUnescape(p)
		if decoded != "" {
			segments = append(segments, sanitizeSegment(decoded))
		}
	}

	// Build 6-part entity ID
	platform := getValue(segments, 0, getDefault(hasMapping, mapping.Platform, t.DefaultPlatform))
	domain := getValue(segments, 1, getDefault(hasMapping, mapping.Domain, t.DefaultDomain))
	system := getValue(segments, 2, getDefault(hasMapping, mapping.System, t.DefaultSystem))
	entityType := getValue(segments, 3, getDefault(hasMapping, mapping.Type, t.DefaultType))
	instance := getValue(segments, 4, "")

	// If we only have one segment, it's probably just the instance
	if len(segments) == 1 {
		instance = segments[0]
		platform = getDefault(hasMapping, mapping.Platform, t.DefaultPlatform)
		domain = getDefault(hasMapping, mapping.Domain, t.DefaultDomain)
		system = getDefault(hasMapping, mapping.System, t.DefaultSystem)
		entityType = getDefault(hasMapping, mapping.Type, t.DefaultType)
	}

	// If instance is empty, use the last non-empty segment
	if instance == "" && len(segments) > 0 {
		instance = segments[len(segments)-1]
	}

	// If still empty, use a placeholder
	if instance == "" {
		instance = "unknown"
	}

	return t.buildEntityID(org, platform, domain, system, entityType, instance)
}

// TripleToRDF converts a SemStreams message.Triple to a TrustGraph triple.
func (t *Translator) TripleToRDF(triple message.Triple) TGTriple {
	// Convert subject
	subjectURI := t.EntityIDToURI(triple.Subject)

	// Convert predicate
	predicateURI := t.PredicateMap.ToRDF(triple.Predicate)

	// Convert object - check if it's an entity reference
	var obj TGValue
	if objStr, ok := triple.Object.(string); ok && message.IsValidEntityID(objStr) {
		// Object is an entity reference
		obj = TGValue{
			V: t.EntityIDToURI(objStr),
			E: true,
		}
	} else {
		// Object is a literal
		obj = TGValue{
			V: formatLiteral(triple.Object),
			E: false,
		}
	}

	return TGTriple{
		S: TGValue{V: subjectURI, E: true},
		P: TGValue{V: predicateURI, E: true},
		O: obj,
	}
}

// RDFToTriple converts a TrustGraph triple to a SemStreams message.Triple.
func (t *Translator) RDFToTriple(tgTriple TGTriple, source string) message.Triple {
	// Convert subject
	subject := t.URIToEntityID(tgTriple.S.V)

	// Convert predicate
	predicate := t.PredicateMap.FromRDF(tgTriple.P.V)

	// Convert object
	var object any
	if tgTriple.O.E {
		// Object is an entity reference
		object = t.URIToEntityID(tgTriple.O.V)
	} else {
		// Object is a literal - parse if possible
		object = parseLiteral(tgTriple.O.V)
	}

	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     source,
		Timestamp:  time.Now(),
		Confidence: 1.0,
	}
}

// TriplesToRDF converts multiple SemStreams triples to TrustGraph format.
func (t *Translator) TriplesToRDF(triples []message.Triple) []TGTriple {
	result := make([]TGTriple, len(triples))
	for i, triple := range triples {
		result[i] = t.TripleToRDF(triple)
	}
	return result
}

// RDFToTriples converts multiple TrustGraph triples to SemStreams format.
func (t *Translator) RDFToTriples(tgTriples []TGTriple, source string) []message.Triple {
	result := make([]message.Triple, len(tgTriples))
	for i, tg := range tgTriples {
		result[i] = t.RDFToTriple(tg, source)
	}
	return result
}

// buildEntityID constructs a 6-part entity ID.
func (t *Translator) buildEntityID(org, platform, domain, system, entityType, instance string) string {
	return org + "." + platform + "." + domain + "." + system + "." + entityType + "." + instance
}

// getValue returns the value at index if it exists, otherwise returns the default.
func getValue(segments []string, index int, defaultVal string) string {
	if index < len(segments) && segments[index] != "" {
		return segments[index]
	}
	return defaultVal
}

// getDefault returns the mapping value if non-empty, otherwise the fallback.
func getDefault(hasMapping bool, mappingVal, fallback string) string {
	if hasMapping && mappingVal != "" {
		return mappingVal
	}
	return fallback
}

// sanitizeSegment ensures the segment uses valid characters for entity IDs.
func sanitizeSegment(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			result.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			result.WriteRune(r + 32) // lowercase
		case r >= '0' && r <= '9':
			result.WriteRune(r)
		case r == '-' || r == '_':
			result.WriteRune(r)
		default:
			result.WriteRune('-')
		}
	}

	// Clean up consecutive dashes
	segment := result.String()
	for strings.Contains(segment, "--") {
		segment = strings.ReplaceAll(segment, "--", "-")
	}
	return strings.Trim(segment, "-")
}

// sanitizeInstance is like sanitizeSegment but preserves more characters for instance IDs.
func sanitizeInstance(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			result.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			result.WriteRune(r + 32) // lowercase
		case r >= '0' && r <= '9':
			result.WriteRune(r)
		case r == '-' || r == '_':
			result.WriteRune(r)
		default:
			result.WriteRune('-')
		}
	}

	// Clean up consecutive dashes
	instance := result.String()
	for strings.Contains(instance, "--") {
		instance = strings.ReplaceAll(instance, "--", "-")
	}
	return strings.Trim(instance, "-")
}

// formatLiteral converts a Go value to a string representation.
func formatLiteral(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int, int32, int64, uint, uint32, uint64:
		return strings.TrimLeft(strings.TrimRight(string(rune(val.(int))), "\x00"), "\x00")
	case float32:
		return formatFloat(float64(val))
	case float64:
		return formatFloat(val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return ""
	}
}

// formatFloat formats a float without unnecessary trailing zeros.
func formatFloat(f float64) string {
	return floatToString(f)
}

// intToString converts an int to string without importing strconv.
func intToString(n int64) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}

// floatToString converts a float to string.
func floatToString(f float64) string {
	// Handle special cases
	if f == 0 {
		return "0"
	}

	negative := f < 0
	if negative {
		f = -f
	}

	// Split into integer and fractional parts
	intPart := int64(f)
	fracPart := f - float64(intPart)

	result := intToString(intPart)

	if fracPart > 0 {
		result += "."
		// Add up to 6 decimal places
		for i := 0; i < 6 && fracPart > 0.000001; i++ {
			fracPart *= 10
			digit := int(fracPart)
			result += string(rune('0' + digit))
			fracPart -= float64(digit)
		}
		// Trim trailing zeros
		result = strings.TrimRight(result, "0")
		result = strings.TrimRight(result, ".")
	}

	if negative {
		result = "-" + result
	}

	return result
}

// parseLiteral attempts to parse a string literal into a typed value.
func parseLiteral(s string) any {
	// Try boolean
	lower := strings.ToLower(s)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}

	// Try integer
	if isInteger(s) {
		return parseInteger(s)
	}

	// Try float
	if isFloat(s) {
		return parseFloat(s)
	}

	// Return as string
	return s
}

// isInteger checks if a string represents an integer.
func isInteger(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isFloat checks if a string represents a float.
func isFloat(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasDecimal := false
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			if hasDecimal {
				return false
			}
			hasDecimal = true
		} else if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return hasDecimal
}

// parseInteger parses a string as an integer.
func parseInteger(s string) int64 {
	negative := false
	start := 0
	if s[0] == '-' {
		negative = true
		start = 1
	} else if s[0] == '+' {
		start = 1
	}

	var result int64
	for i := start; i < len(s); i++ {
		result = result*10 + int64(s[i]-'0')
	}

	if negative {
		return -result
	}
	return result
}

// parseFloat parses a string as a float.
func parseFloat(s string) float64 {
	negative := false
	start := 0
	if s[0] == '-' {
		negative = true
		start = 1
	} else if s[0] == '+' {
		start = 1
	}

	var intPart int64
	var fracPart float64
	var fracDiv float64 = 1
	inFrac := false

	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			inFrac = true
			continue
		}
		if inFrac {
			fracDiv *= 10
			fracPart += float64(s[i]-'0') / fracDiv
		} else {
			intPart = intPart*10 + int64(s[i]-'0')
		}
	}

	result := float64(intPart) + fracPart
	if negative {
		return -result
	}
	return result
}
