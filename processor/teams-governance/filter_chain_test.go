package teamsgovernance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockFilter is a test filter
type MockFilter struct {
	name       string
	allowed    bool
	modified   *Message
	violation  *Violation
	confidence float64
}

func (f *MockFilter) Name() string { return f.name }

func (f *MockFilter) Process(_ context.Context, _ *Message) (*FilterResult, error) {
	result := &FilterResult{
		Allowed:    f.allowed,
		Modified:   f.modified,
		Violation:  f.violation,
		Confidence: f.confidence,
	}
	return result, nil
}

func TestFilterChain_FailFastPolicy(t *testing.T) {
	chain := NewFilterChain(PolicyFailFast, nil)
	chain.AddFilter(&MockFilter{name: "filter1", allowed: true, confidence: 1.0})
	chain.AddFilter(&MockFilter{
		name:       "filter2",
		allowed:    false,
		confidence: 0.9,
		violation:  &Violation{FilterName: "filter2", Severity: SeverityHigh},
	})
	chain.AddFilter(&MockFilter{name: "filter3", allowed: true, confidence: 1.0})

	msg := &Message{ID: "test", Content: Content{Text: "test message"}}
	result, err := chain.Process(context.Background(), msg)

	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Len(t, result.FiltersApplied, 2, "Should stop at filter2")
	assert.Len(t, result.Violations, 1)
	assert.Equal(t, "filter2", result.Violations[0].FilterName)
}

func TestFilterChain_ContinuePolicy(t *testing.T) {
	chain := NewFilterChain(PolicyContinue, nil)
	chain.AddFilter(&MockFilter{
		name:       "filter1",
		allowed:    false,
		confidence: 0.9,
		violation:  &Violation{FilterName: "filter1", Severity: SeverityMedium},
	})
	chain.AddFilter(&MockFilter{
		name:       "filter2",
		allowed:    false,
		confidence: 0.8,
		violation:  &Violation{FilterName: "filter2", Severity: SeverityHigh},
	})
	chain.AddFilter(&MockFilter{name: "filter3", allowed: true, confidence: 1.0})

	msg := &Message{ID: "test", Content: Content{Text: "test message"}}
	result, err := chain.Process(context.Background(), msg)

	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Len(t, result.FiltersApplied, 3, "All filters should run")
	assert.Len(t, result.Violations, 2, "Both violations collected")
}

func TestFilterChain_LogOnlyPolicy(t *testing.T) {
	chain := NewFilterChain(PolicyLogOnly, nil)
	chain.AddFilter(&MockFilter{
		name:       "filter1",
		allowed:    false,
		confidence: 0.9,
		violation:  &Violation{FilterName: "filter1", Severity: SeverityHigh},
	})
	chain.AddFilter(&MockFilter{name: "filter2", allowed: true, confidence: 1.0})

	msg := &Message{ID: "test", Content: Content{Text: "test message"}}
	result, err := chain.Process(context.Background(), msg)

	require.NoError(t, err)
	assert.True(t, result.Allowed, "LogOnly policy should allow all content")
	assert.Len(t, result.FiltersApplied, 2)
	assert.Len(t, result.Violations, 1, "Violations still collected")
}

func TestFilterChain_ModifiedMessage(t *testing.T) {
	modifiedMsg := &Message{ID: "test", Content: Content{Text: "modified text"}}

	chain := NewFilterChain(PolicyFailFast, nil)
	chain.AddFilter(&MockFilter{name: "filter1", allowed: true, modified: modifiedMsg, confidence: 1.0})
	chain.AddFilter(&MockFilter{name: "filter2", allowed: true, confidence: 1.0})

	msg := &Message{ID: "test", Content: Content{Text: "original text"}}
	result, err := chain.Process(context.Background(), msg)

	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, modifiedMsg, result.ModifiedMessage)
	assert.Contains(t, result.Modifications, "filter1")
}

func TestFilterChain_AllFiltersPass(t *testing.T) {
	chain := NewFilterChain(PolicyFailFast, nil)
	chain.AddFilter(&MockFilter{name: "filter1", allowed: true, confidence: 1.0})
	chain.AddFilter(&MockFilter{name: "filter2", allowed: true, confidence: 1.0})
	chain.AddFilter(&MockFilter{name: "filter3", allowed: true, confidence: 1.0})

	msg := &Message{ID: "test", Content: Content{Text: "clean message"}}
	result, err := chain.Process(context.Background(), msg)

	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Len(t, result.FiltersApplied, 3)
	assert.Len(t, result.Violations, 0)
}

func TestFilterChain_ContextCancellation(t *testing.T) {
	chain := NewFilterChain(PolicyFailFast, nil)
	chain.AddFilter(&MockFilter{name: "filter1", allowed: true, confidence: 1.0})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg := &Message{ID: "test", Content: Content{Text: "test"}}
	_, err := chain.Process(ctx, msg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestChainResult_HasViolations(t *testing.T) {
	result := &ChainResult{
		Violations: []*Violation{},
	}
	assert.False(t, result.HasViolations())

	result.Violations = append(result.Violations, &Violation{})
	assert.True(t, result.HasViolations())
}

func TestChainResult_HighestSeverity(t *testing.T) {
	result := &ChainResult{
		Violations: []*Violation{
			{Severity: SeverityLow},
			{Severity: SeverityHigh},
			{Severity: SeverityMedium},
		},
	}
	assert.Equal(t, SeverityHigh, result.HighestSeverity())

	result = &ChainResult{
		Violations: []*Violation{
			{Severity: SeverityCritical},
			{Severity: SeverityHigh},
		},
	}
	assert.Equal(t, SeverityCritical, result.HighestSeverity())

	result = &ChainResult{Violations: []*Violation{}}
	assert.Equal(t, Severity(""), result.HighestSeverity())
}

func TestChainResult_AddGovernanceMetadata(t *testing.T) {
	msg := &Message{
		ID:      "test",
		Content: Content{Text: "test", Metadata: make(map[string]any)},
	}

	result := &ChainResult{
		ModifiedMessage: msg,
		FiltersApplied:  []string{"filter1", "filter2"},
		Modifications:   []string{"filter1"},
		Violations: []*Violation{
			{FilterName: "filter2", Severity: SeverityHigh, Action: ViolationActionFlagged},
		},
	}

	result.AddGovernanceMetadata()

	meta, ok := msg.Content.Metadata["governance"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, meta, "filters_applied")
	assert.Contains(t, meta, "modifications")
	assert.Contains(t, meta, "violations")
}

func TestFilterChainBuilder(t *testing.T) {
	chain := NewFilterChainBuilder(nil).
		WithPolicy(PolicyContinue).
		AddFilter(&MockFilter{name: "filter1", allowed: true, confidence: 1.0}).
		AddFilter(&MockFilter{name: "filter2", allowed: true, confidence: 1.0}).
		Build()

	assert.Equal(t, PolicyContinue, chain.Policy)
	assert.Len(t, chain.Filters, 2)
}

func TestBuildFromConfig(t *testing.T) {
	config := FilterChainConfig{
		Policy: PolicyFailFast,
		Filters: []FilterConfig{
			{
				Name:    "pii_redaction",
				Enabled: true,
				PIIConfig: &PIIFilterConfig{
					Types:               []PIIType{PIITypeEmail},
					Strategy:            RedactionLabel,
					ConfidenceThreshold: 0.9,
				},
			},
			{
				Name:    "injection_detection",
				Enabled: true,
				InjectionConfig: &InjectionFilterConfig{
					ConfidenceThreshold: 0.8,
					EnabledPatterns:     []string{"instruction_override"},
				},
			},
			{
				Name:    "content_moderation",
				Enabled: false, // Disabled filter
			},
		},
	}

	chain, err := BuildFromConfig(config, nil)
	require.NoError(t, err)
	assert.Equal(t, PolicyFailFast, chain.Policy)
	assert.Len(t, chain.Filters, 2, "Disabled filter should not be added")
}

func TestBuildFromConfig_InvalidFilter(t *testing.T) {
	config := FilterChainConfig{
		Policy: PolicyFailFast,
		Filters: []FilterConfig{
			{
				Name:    "unknown_filter",
				Enabled: true,
			},
		},
	}

	_, err := BuildFromConfig(config, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown filter")
}

func TestBuildFromConfig_DefaultConfigs(t *testing.T) {
	config := FilterChainConfig{
		Policy: PolicyFailFast,
		Filters: []FilterConfig{
			{Name: "pii_redaction", Enabled: true},
			{Name: "injection_detection", Enabled: true},
			{Name: "content_moderation", Enabled: true},
			{Name: "rate_limiting", Enabled: true},
		},
	}

	chain, err := BuildFromConfig(config, nil)
	require.NoError(t, err)
	assert.Len(t, chain.Filters, 4)
}
