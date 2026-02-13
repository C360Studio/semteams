// Package trustgraph provides vocabulary translation between SemStreams dotted notation
// and RDF URIs for TrustGraph integration.
//
// The package handles bidirectional translation:
//   - Entity IDs: 6-part dotted notation (org.platform.domain.system.type.instance) <-> RDF URIs
//   - Predicates: SemStreams predicates (domain.category.property) <-> RDF predicate URIs
//   - Triples: SemStreams message.Triple <-> TrustGraph compact JSON triple format
//
// # Entity ID Translation
//
// SemStreams uses hierarchical 6-part entity IDs:
//
//	acme.ops.environmental.sensor.temperature.sensor-042
//
// These translate to RDF URIs:
//
//	http://acme.org/ops/environmental/sensor/temperature/sensor-042
//
// The translation is configurable via org mappings and default settings.
//
// # Predicate Translation
//
// Predicates use a two-tier approach:
//  1. Exact match table: Configured mappings for well-known predicates
//  2. Structural fallback: Unmapped predicates convert via path transformation
//
// Example exact match:
//
//	sensor.measurement.celsius -> http://www.w3.org/ns/sosa/hasSimpleResult
//
// Example structural fallback:
//
//	custom.domain.property -> http://{base}/predicate/custom/domain/property
//
// # Usage
//
//	translator := trustgraph.NewTranslator(trustgraph.TranslatorConfig{
//	    OrgMappings: map[string]string{
//	        "acme": "https://data.acme-corp.com/",
//	    },
//	    PredicateMappings: map[string]string{
//	        "sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
//	    },
//	})
//
//	uri := translator.EntityIDToURI("acme.ops.robotics.gcs.drone.001")
//	entityID := translator.URIToEntityID("http://trustgraph.ai/e/concept-name")
package trustgraph
