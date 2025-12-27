package stages

import (
	"testing"
)

func TestEntityVerifier_GetMinRequired(t *testing.T) {
	tests := []struct {
		name     string
		variant  string
		expected int
	}{
		{"structural", "structural", StructuralMinEntities},
		{"statistical", "statistical", StatisticalMinEntities},
		{"semantic", "semantic", SemanticMinEntities},
		{"unknown falls back to config", "unknown", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &EntityVerifier{
				Variant:           tt.variant,
				MinExpectedConfig: 100,
			}
			got := v.getMinRequired()
			if got != tt.expected {
				t.Errorf("getMinRequired() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestEntityVerifier_GetCriticalEntities(t *testing.T) {
	tests := []struct {
		name     string
		variant  string
		contains string
	}{
		{"structural uses sensor entity", "structural", "temp-sensor"},
		{"statistical uses document entity", "statistical", "doc-ops"},
		{"semantic uses document entity", "semantic", "doc-ops"},
		{"unknown uses document entity", "unknown", "doc-ops"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &EntityVerifier{Variant: tt.variant}
			entities := v.getCriticalEntities()
			if len(entities) == 0 {
				t.Fatal("getCriticalEntities() returned empty list")
			}
			found := false
			for _, e := range entities {
				if containsSubstring(e, tt.contains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("getCriticalEntities() = %v, want entity containing %q", entities, tt.contains)
			}
		})
	}
}

func TestEntityVerifier_GetExpectedFromTestData(t *testing.T) {
	tests := []struct {
		name     string
		variant  string
		expected int
	}{
		{"structural has 74 entities", "structural", 74},
		{"statistical has 74 entities", "statistical", 74},
		{"semantic has 74 entities", "semantic", 74},
		{"unknown defaults to 74", "unknown", 74},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &EntityVerifier{Variant: tt.variant}
			got := v.getExpectedFromTestData()
			if got != tt.expected {
				t.Errorf("getExpectedFromTestData() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestDefaultTestEntities(t *testing.T) {
	entities := DefaultTestEntities()
	if len(entities) == 0 {
		t.Fatal("DefaultTestEntities() returned empty list")
	}

	// Verify each entity has required fields
	for i, e := range entities {
		if e.ID == "" {
			t.Errorf("entity %d has empty ID", i)
		}
		if e.ExpectedType == "" {
			t.Errorf("entity %d has empty ExpectedType", i)
		}
		if e.Source == "" {
			t.Errorf("entity %d has empty Source", i)
		}
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && containsAt(s, substr)))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
