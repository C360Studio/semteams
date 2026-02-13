package trustgraph

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
)

func TestTranslator_EntityIDToURI(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		OrgMappings: map[string]string{
			"acme": "https://data.acme-corp.com/",
		},
	})

	tests := []struct {
		name     string
		entityID string
		want     string
	}{
		{
			name:     "standard 6-part with org mapping",
			entityID: "acme.ops.environmental.sensor.temperature.sensor-042",
			want:     "https://data.acme-corp.com/ops/environmental/sensor/temperature/sensor-042",
		},
		{
			name:     "6-part without org mapping",
			entityID: "external.platform.domain.system.type.instance",
			want:     "http://external.org/platform/domain/system/type/instance",
		},
		{
			name:     "short entity ID",
			entityID: "org.instance",
			want:     "http://org.org/instance",
		},
		{
			name:     "single segment",
			entityID: "single",
			want:     "http://semstreams.io/e/single",
		},
		{
			name:     "with special characters",
			entityID: "acme.ops.robotics.gcs.drone.drone-001",
			want:     "https://data.acme-corp.com/ops/robotics/gcs/drone/drone-001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.EntityIDToURI(tt.entityID)
			if got != tt.want {
				t.Errorf("EntityIDToURI(%q) = %q, want %q", tt.entityID, got, tt.want)
			}
		})
	}
}

func TestTranslator_URIToEntityID(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		URIMappings: map[string]URIMapping{
			"trustgraph.ai": {
				Org:      "trustgraph",
				Platform: "default",
				Domain:   "knowledge",
				System:   "entity",
				Type:     "concept",
			},
			"data.acme-corp.com": {
				Org:      "acme",
				Platform: "ops",
			},
		},
		DefaultOrg:      "external",
		DefaultPlatform: "default",
		DefaultDomain:   "knowledge",
		DefaultSystem:   "entity",
		DefaultType:     "concept",
	})

	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "TrustGraph entity URI",
			uri:  "http://trustgraph.ai/e/supply-chain-risk",
			want: "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
		},
		{
			name: "TrustGraph URI with multiple path segments",
			uri:  "http://trustgraph.ai/e/intel/threat/apt-29",
			want: "trustgraph.intel.threat.apt-29.concept.apt-29",
		},
		{
			name: "mapped domain URI",
			uri:  "https://data.acme-corp.com/ops/environmental/sensor/temperature/sensor-042",
			want: "acme.ops.environmental.sensor.temperature.sensor-042",
		},
		{
			name: "unmapped domain",
			uri:  "http://unknown.example.org/e/some-entity",
			want: "unknown.default.knowledge.entity.concept.some-entity",
		},
		{
			name: "URI with port",
			uri:  "http://localhost:8088/e/test-entity",
			want: "localhost.default.knowledge.entity.concept.test-entity",
		},
		{
			name: "URI without /e/ prefix",
			uri:  "http://example.org/entities/my-entity",
			want: "example.default.knowledge.entity.concept.my-entity",
		},
		{
			name: "full 5-segment path",
			uri:  "http://example.org/platform/domain/system/type/instance",
			want: "example.platform.domain.system.type.instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.URIToEntityID(tt.uri)
			if got != tt.want {
				t.Errorf("URIToEntityID(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestTranslator_RoundTrip(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		OrgMappings: map[string]string{
			"acme": "https://data.acme-corp.com/",
		},
		URIMappings: map[string]URIMapping{
			"data.acme-corp.com": {
				Org: "acme",
			},
		},
	})

	// Entity ID -> URI -> Entity ID
	originalID := "acme.ops.environmental.sensor.temperature.sensor-042"
	uri := translator.EntityIDToURI(originalID)
	roundTripped := translator.URIToEntityID(uri)

	if roundTripped != originalID {
		t.Errorf("Round trip failed: %q -> %q -> %q", originalID, uri, roundTripped)
	}
}

func TestTranslator_TripleToRDF(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		OrgMappings: map[string]string{
			"acme": "https://data.acme-corp.com/",
		},
		PredicateMappings: map[string]string{
			"sensor.measurement.celsius": SOSA + "hasSimpleResult",
		},
	})

	tests := []struct {
		name   string
		triple message.Triple
		want   TGTriple
	}{
		{
			name: "literal value",
			triple: message.Triple{
				Subject:   "acme.ops.environmental.sensor.temperature.sensor-042",
				Predicate: "sensor.measurement.celsius",
				Object:    "45.2",
			},
			want: TGTriple{
				S: TGValue{V: "https://data.acme-corp.com/ops/environmental/sensor/temperature/sensor-042", E: true},
				P: TGValue{V: SOSA + "hasSimpleResult", E: true},
				O: TGValue{V: "45.2", E: false},
			},
		},
		{
			name: "entity reference",
			triple: message.Triple{
				Subject:   "acme.ops.robotics.gcs.drone.001",
				Predicate: "relation.part_of",
				Object:    "acme.ops.robotics.gcs.fleet.alpha",
			},
			want: TGTriple{
				S: TGValue{V: "https://data.acme-corp.com/ops/robotics/gcs/drone/001", E: true},
				P: TGValue{V: BFO + "BFO_0000050", E: true},
				O: TGValue{V: "https://data.acme-corp.com/ops/robotics/gcs/fleet/alpha", E: true},
			},
		},
		{
			name: "boolean value",
			triple: message.Triple{
				Subject:   "acme.ops.robotics.gcs.drone.001",
				Predicate: "status.armed",
				Object:    true,
			},
			want: TGTriple{
				S: TGValue{V: "https://data.acme-corp.com/ops/robotics/gcs/drone/001", E: true},
				P: TGValue{V: "http://semstreams.io/e/predicate/status/armed", E: true},
				O: TGValue{V: "true", E: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.TripleToRDF(tt.triple)
			if got.S != tt.want.S {
				t.Errorf("Subject: got %+v, want %+v", got.S, tt.want.S)
			}
			if got.P != tt.want.P {
				t.Errorf("Predicate: got %+v, want %+v", got.P, tt.want.P)
			}
			if got.O != tt.want.O {
				t.Errorf("Object: got %+v, want %+v", got.O, tt.want.O)
			}
		})
	}
}

func TestTranslator_RDFToTriple(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		URIMappings: map[string]URIMapping{
			"trustgraph.ai": {
				Org:      "trustgraph",
				Platform: "default",
				Domain:   "knowledge",
				System:   "entity",
				Type:     "concept",
			},
		},
	})

	tests := []struct {
		name     string
		tgTriple TGTriple
		source   string
		want     message.Triple
	}{
		{
			name: "literal value",
			tgTriple: TGTriple{
				S: TGValue{V: "http://trustgraph.ai/e/supply-chain-risk", E: true},
				P: TGValue{V: RDFS + "label", E: true},
				O: TGValue{V: "Supply Chain Risk Assessment", E: false},
			},
			source: "trustgraph",
			want: message.Triple{
				Subject:    "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
				Predicate:  "entity.metadata.label",
				Object:     "Supply Chain Risk Assessment",
				Source:     "trustgraph",
				Confidence: 1.0,
			},
		},
		{
			name: "entity reference",
			tgTriple: TGTriple{
				S: TGValue{V: "http://trustgraph.ai/e/concept-a", E: true},
				P: TGValue{V: SCHEMA + "relatedTo", E: true},
				O: TGValue{V: "http://trustgraph.ai/e/concept-b", E: true},
			},
			source: "trustgraph",
			want: message.Triple{
				Subject:    "trustgraph.default.knowledge.entity.concept.concept-a",
				Predicate:  "relation.relates_to",
				Object:     "trustgraph.default.knowledge.entity.concept.concept-b",
				Source:     "trustgraph",
				Confidence: 1.0,
			},
		},
		{
			name: "numeric literal parsed",
			tgTriple: TGTriple{
				S: TGValue{V: "http://trustgraph.ai/e/sensor-001", E: true},
				P: TGValue{V: SOSA + "resultTime", E: true}, // use unique mapping
				O: TGValue{V: "42.5", E: false},
			},
			source: "trustgraph",
			want: message.Triple{
				Subject:    "trustgraph.default.knowledge.entity.concept.sensor-001",
				Predicate:  "sensor.observation.time", // unique mapping from resultTime
				Object:     42.5,
				Source:     "trustgraph",
				Confidence: 1.0,
			},
		},
		{
			name: "boolean literal parsed",
			tgTriple: TGTriple{
				S: TGValue{V: "http://trustgraph.ai/e/entity-001", E: true},
				P: TGValue{V: "http://example.org/property/active", E: true},
				O: TGValue{V: "true", E: false},
			},
			source: "trustgraph",
			want: message.Triple{
				Subject:    "trustgraph.default.knowledge.entity.concept.entity-001",
				Predicate:  "active", // fallback converts /property/active to just "active"
				Object:     true,
				Source:     "trustgraph",
				Confidence: 1.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.RDFToTriple(tt.tgTriple, tt.source)

			if got.Subject != tt.want.Subject {
				t.Errorf("Subject: got %q, want %q", got.Subject, tt.want.Subject)
			}
			if got.Predicate != tt.want.Predicate {
				t.Errorf("Predicate: got %q, want %q", got.Predicate, tt.want.Predicate)
			}
			if got.Object != tt.want.Object {
				t.Errorf("Object: got %v (%T), want %v (%T)", got.Object, got.Object, tt.want.Object, tt.want.Object)
			}
			if got.Source != tt.want.Source {
				t.Errorf("Source: got %q, want %q", got.Source, tt.want.Source)
			}
			if got.Confidence != tt.want.Confidence {
				t.Errorf("Confidence: got %f, want %f", got.Confidence, tt.want.Confidence)
			}
		})
	}
}

func TestTranslator_TriplesToRDF(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		OrgMappings: map[string]string{
			"acme": "https://data.acme-corp.com/",
		},
	})

	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "entity.metadata.label", Object: "Drone 001"},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "entity.classification.type", Object: "drone"},
	}

	got := translator.TriplesToRDF(triples)

	if len(got) != 2 {
		t.Fatalf("Expected 2 triples, got %d", len(got))
	}

	// Check first triple
	if got[0].S.V != "https://data.acme-corp.com/ops/robotics/gcs/drone/001" {
		t.Errorf("First triple subject: got %q", got[0].S.V)
	}
}

func TestTranslator_RDFToTriples(t *testing.T) {
	translator := NewTranslator(TranslatorConfig{
		URIMappings: map[string]URIMapping{
			"trustgraph.ai": {
				Org:      "trustgraph",
				Platform: "default",
				Domain:   "knowledge",
				System:   "entity",
				Type:     "concept",
			},
		},
	})

	tgTriples := []TGTriple{
		{
			S: TGValue{V: "http://trustgraph.ai/e/entity-001", E: true},
			P: TGValue{V: RDFS + "label", E: true},
			O: TGValue{V: "Entity One", E: false},
		},
		{
			S: TGValue{V: "http://trustgraph.ai/e/entity-001", E: true},
			P: TGValue{V: RDF + "type", E: true},
			O: TGValue{V: "http://trustgraph.ai/e/Concept", E: true},
		},
	}

	got := translator.RDFToTriples(tgTriples, "trustgraph")

	if len(got) != 2 {
		t.Fatalf("Expected 2 triples, got %d", len(got))
	}

	// Check that all triples have the correct source
	for i, triple := range got {
		if triple.Source != "trustgraph" {
			t.Errorf("Triple %d source: got %q, want %q", i, triple.Source, "trustgraph")
		}
	}
}

func TestSanitizeSegment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"MixedCase", "mixedcase"},
		{"with-hyphen", "with-hyphen"},
		{"with_underscore", "with_underscore"},
		{"with spaces", "with-spaces"},
		{"with.dots", "with-dots"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"-leading-trailing-", "leading-trailing"},
		{"123numbers", "123numbers"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeSegment(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"42", int64(42)},
		{"-123", int64(-123)},
		{"3.14", 3.14},
		{"-2.5", -2.5},
		{"hello world", "hello world"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLiteral(tt.input)
			if got != tt.want {
				t.Errorf("parseLiteral(%q) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestTranslator_Defaults(t *testing.T) {
	// Test with minimal configuration
	translator := NewTranslator(TranslatorConfig{})

	// Check that defaults are applied
	if translator.DefaultOrg != "external" {
		t.Errorf("DefaultOrg: got %q, want %q", translator.DefaultOrg, "external")
	}
	if translator.DefaultPlatform != "default" {
		t.Errorf("DefaultPlatform: got %q, want %q", translator.DefaultPlatform, "default")
	}
	if translator.DefaultDomain != "knowledge" {
		t.Errorf("DefaultDomain: got %q, want %q", translator.DefaultDomain, "knowledge")
	}
	if translator.DefaultSystem != "entity" {
		t.Errorf("DefaultSystem: got %q, want %q", translator.DefaultSystem, "entity")
	}
	if translator.DefaultType != "concept" {
		t.Errorf("DefaultType: got %q, want %q", translator.DefaultType, "concept")
	}

	// Test URI translation with defaults
	uri := "http://unknown.example.org/e/test-entity"
	got := translator.URIToEntityID(uri)
	want := "unknown.default.knowledge.entity.concept.test-entity"
	if got != want {
		t.Errorf("URIToEntityID with defaults: got %q, want %q", got, want)
	}
}

func TestFormatLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", "hello"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"time", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), "2024-01-15T10:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLiteral(tt.input)
			if got != tt.want {
				t.Errorf("formatLiteral(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
