package trustgraph

import (
	"net/url"
	"strings"
)

// PredicateMap manages bidirectional predicate translation between
// SemStreams dotted predicates and RDF predicate URIs.
type PredicateMap struct {
	// toRDF maps SemStreams predicate -> RDF URI
	toRDF map[string]string

	// fromRDF maps RDF URI -> SemStreams predicate
	fromRDF map[string]string

	// defaultBase is the base URI for unmapped predicates
	defaultBase string
}

// NewPredicateMap creates a predicate map from configuration.
// The mappings parameter maps SemStreams predicates to RDF URIs.
// The defaultBase is used for structural fallback when no exact match exists.
func NewPredicateMap(mappings map[string]string, defaultBase string) *PredicateMap {
	if defaultBase == "" {
		defaultBase = "http://semstreams.io/predicate/"
	}

	// Ensure defaultBase ends with /
	if !strings.HasSuffix(defaultBase, "/") {
		defaultBase += "/"
	}

	pm := &PredicateMap{
		toRDF:       make(map[string]string),
		fromRDF:     make(map[string]string),
		defaultBase: defaultBase,
	}

	// Start with well-known predicates
	for ssKey, rdfURI := range WellKnownPredicates {
		pm.toRDF[ssKey] = rdfURI
		pm.fromRDF[rdfURI] = ssKey
	}

	// Override with user-provided mappings
	for ssKey, rdfURI := range mappings {
		pm.toRDF[ssKey] = rdfURI
		pm.fromRDF[rdfURI] = ssKey
	}

	return pm
}

// ToRDF converts a SemStreams predicate to an RDF predicate URI.
//
// Translation uses a two-tier approach:
//  1. Exact match: If the predicate exists in the mapping table, return the mapped URI.
//  2. Structural fallback: Convert dots to slashes and prepend the default base.
//
// Examples:
//
//	"sensor.measurement.celsius" -> "http://www.w3.org/ns/sosa/hasSimpleResult" (exact)
//	"custom.domain.property" -> "http://semstreams.io/predicate/custom/domain/property" (fallback)
func (pm *PredicateMap) ToRDF(predicate string) string {
	// Tier 1: Exact match
	if uri, ok := pm.toRDF[predicate]; ok {
		return uri
	}

	// Tier 2: Structural fallback - convert dots to path segments
	path := strings.ReplaceAll(predicate, ".", "/")
	return pm.defaultBase + path
}

// FromRDF converts an RDF predicate URI to a SemStreams predicate.
//
// Translation uses a two-tier approach:
//  1. Exact match: If the URI exists in the reverse mapping table, return the mapped predicate.
//  2. Structural fallback: Parse the URI path and convert slashes to dots.
//
// Examples:
//
//	"http://www.w3.org/ns/sosa/hasSimpleResult" -> "sensor.measurement.value" (exact)
//	"http://example.org/predicate/custom/domain/property" -> "custom.domain.property" (fallback)
func (pm *PredicateMap) FromRDF(uri string) string {
	// Tier 1: Exact match
	if predicate, ok := pm.fromRDF[uri]; ok {
		return predicate
	}

	// Tier 2: Structural fallback - parse URI and convert path to dots
	parsed, err := url.Parse(uri)
	if err != nil {
		// If we can't parse, return a sanitized version
		return sanitizePredicate(uri)
	}

	// Extract path, removing leading slash
	path := strings.TrimPrefix(parsed.Path, "/")

	// Check for common patterns like /predicate/, /property/, /p/
	for _, prefix := range []string{"predicate/", "property/", "p/", "e/"} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}

	// Convert slashes to dots and add fragment if present
	predicate := strings.ReplaceAll(path, "/", ".")

	// If there's a fragment (e.g., rdf#type), append it
	if parsed.Fragment != "" {
		predicate = predicate + "." + parsed.Fragment
	}

	return sanitizePredicate(predicate)
}

// AddMapping adds a predicate mapping at runtime.
func (pm *PredicateMap) AddMapping(semstreamsPredicate, rdfURI string) {
	pm.toRDF[semstreamsPredicate] = rdfURI
	pm.fromRDF[rdfURI] = semstreamsPredicate
}

// HasMapping returns true if an exact mapping exists for the predicate.
func (pm *PredicateMap) HasMapping(semstreamsPredicate string) bool {
	_, ok := pm.toRDF[semstreamsPredicate]
	return ok
}

// HasReverseMapping returns true if an exact reverse mapping exists for the URI.
func (pm *PredicateMap) HasReverseMapping(rdfURI string) bool {
	_, ok := pm.fromRDF[rdfURI]
	return ok
}

// sanitizePredicate ensures the predicate uses valid characters.
// Replaces invalid characters with underscores and removes leading/trailing dots.
func sanitizePredicate(s string) string {
	// Remove any scheme prefix if present
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}

	// Replace invalid characters
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			result.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			result.WriteRune(r)
		case r >= '0' && r <= '9':
			result.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			result.WriteRune(r)
		case r == '/' || r == '#':
			result.WriteRune('.')
		default:
			result.WriteRune('_')
		}
	}

	// Clean up consecutive dots and trim
	predicate := result.String()
	for strings.Contains(predicate, "..") {
		predicate = strings.ReplaceAll(predicate, "..", ".")
	}
	predicate = strings.Trim(predicate, ".")

	return predicate
}
