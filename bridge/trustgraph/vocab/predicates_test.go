package vocab

import (
	"testing"
)

func TestPredicateMap_ToRDF_ExactMatch(t *testing.T) {
	pm := NewPredicateMap(map[string]string{
		"sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
		"custom.predicate.example":   "http://example.org/custom/predicate",
	}, "http://default.base/")

	tests := []struct {
		name      string
		predicate string
		want      string
	}{
		{
			name:      "custom mapping",
			predicate: "sensor.measurement.celsius",
			want:      "http://www.w3.org/ns/sosa/hasSimpleResult",
		},
		{
			name:      "user-provided mapping",
			predicate: "custom.predicate.example",
			want:      "http://example.org/custom/predicate",
		},
		{
			name:      "well-known predicate rdf:type",
			predicate: "entity.classification.type",
			want:      RDF + "type",
		},
		{
			name:      "well-known predicate rdfs:label",
			predicate: "entity.metadata.label",
			want:      RDFS + "label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pm.ToRDF(tt.predicate)
			if got != tt.want {
				t.Errorf("ToRDF(%q) = %q, want %q", tt.predicate, got, tt.want)
			}
		})
	}
}

func TestPredicateMap_ToRDF_Fallback(t *testing.T) {
	pm := NewPredicateMap(nil, "http://fallback.example.org/pred/")

	tests := []struct {
		name      string
		predicate string
		want      string
	}{
		{
			name:      "simple predicate",
			predicate: "domain.category.property",
			want:      "http://fallback.example.org/pred/domain/category/property",
		},
		{
			name:      "two-part predicate",
			predicate: "simple.predicate",
			want:      "http://fallback.example.org/pred/simple/predicate",
		},
		{
			name:      "single-part predicate",
			predicate: "single",
			want:      "http://fallback.example.org/pred/single",
		},
		{
			name:      "deep predicate",
			predicate: "a.b.c.d.e.f",
			want:      "http://fallback.example.org/pred/a/b/c/d/e/f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pm.ToRDF(tt.predicate)
			if got != tt.want {
				t.Errorf("ToRDF(%q) = %q, want %q", tt.predicate, got, tt.want)
			}
		})
	}
}

func TestPredicateMap_FromRDF_ExactMatch(t *testing.T) {
	pm := NewPredicateMap(map[string]string{
		"sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
		"custom.predicate.example":   "http://example.org/custom/predicate",
	}, "http://default.base/")

	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "custom mapping reverse",
			uri:  "http://www.w3.org/ns/sosa/hasSimpleResult",
			want: "sensor.measurement.celsius",
		},
		{
			name: "user-provided mapping reverse",
			uri:  "http://example.org/custom/predicate",
			want: "custom.predicate.example",
		},
		{
			name: "well-known predicate reverse rdf:type",
			uri:  RDF + "type",
			want: "entity.classification.type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pm.FromRDF(tt.uri)
			if got != tt.want {
				t.Errorf("FromRDF(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestPredicateMap_FromRDF_Fallback(t *testing.T) {
	pm := NewPredicateMap(nil, "http://default.base/")

	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "simple path",
			uri:  "http://example.org/domain/category/property",
			want: "domain.category.property",
		},
		{
			name: "predicate prefix stripped",
			uri:  "http://example.org/predicate/custom/pred",
			want: "custom.pred",
		},
		{
			name: "property prefix stripped",
			uri:  "http://example.org/property/my/prop",
			want: "my.prop",
		},
		{
			name: "fragment handled",
			uri:  "http://example.org/vocab#somePredicate",
			want: "vocab.somePredicate",
		},
		{
			name: "e prefix stripped",
			uri:  "http://example.org/e/entity/pred",
			want: "entity.pred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pm.FromRDF(tt.uri)
			if got != tt.want {
				t.Errorf("FromRDF(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestPredicateMap_DefaultBase(t *testing.T) {
	// Test that default base is applied when not provided
	pm := NewPredicateMap(nil, "")

	got := pm.ToRDF("unmapped.predicate")
	want := "http://semstreams.io/predicate/unmapped/predicate"
	if got != want {
		t.Errorf("ToRDF with default base = %q, want %q", got, want)
	}
}

func TestPredicateMap_AddMapping(t *testing.T) {
	pm := NewPredicateMap(nil, "http://base/")

	// Before adding
	got := pm.ToRDF("new.predicate")
	if got != "http://base/new/predicate" {
		t.Errorf("ToRDF before AddMapping = %q, want fallback", got)
	}

	// Add mapping
	pm.AddMapping("new.predicate", "http://custom/uri")

	// After adding
	got = pm.ToRDF("new.predicate")
	if got != "http://custom/uri" {
		t.Errorf("ToRDF after AddMapping = %q, want %q", got, "http://custom/uri")
	}

	// Reverse mapping should also work
	got = pm.FromRDF("http://custom/uri")
	if got != "new.predicate" {
		t.Errorf("FromRDF after AddMapping = %q, want %q", got, "new.predicate")
	}
}

func TestPredicateMap_HasMapping(t *testing.T) {
	pm := NewPredicateMap(map[string]string{
		"mapped.predicate": "http://example/uri",
	}, "http://base/")

	if !pm.HasMapping("mapped.predicate") {
		t.Error("HasMapping should return true for mapped predicate")
	}

	if pm.HasMapping("unmapped.predicate") {
		t.Error("HasMapping should return false for unmapped predicate")
	}

	// Well-known predicates should be present
	if !pm.HasMapping("entity.classification.type") {
		t.Error("HasMapping should return true for well-known predicate")
	}
}

func TestPredicateMap_HasReverseMapping(t *testing.T) {
	pm := NewPredicateMap(map[string]string{
		"mapped.predicate": "http://example/uri",
	}, "http://base/")

	if !pm.HasReverseMapping("http://example/uri") {
		t.Error("HasReverseMapping should return true for mapped URI")
	}

	if pm.HasReverseMapping("http://unmapped/uri") {
		t.Error("HasReverseMapping should return false for unmapped URI")
	}

	// Well-known predicates should be present
	if !pm.HasReverseMapping(RDF + "type") {
		t.Error("HasReverseMapping should return true for well-known URI")
	}
}

func TestSanitizePredicate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple.predicate", "simple.predicate"},
		{"http://example.org/path/to/pred", "example.org.path.to.pred"},
		{"with spaces", "with_spaces"},
		{"with/slashes", "with.slashes"},
		{"with#fragment", "with.fragment"},
		{"..leading.dots..", "leading.dots"},
		{"consecutive..dots", "consecutive.dots"},
		{"MixedCase", "MixedCase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizePredicate(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePredicate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
