package agenticgovernance

import (
	"context"
)

// InjectionFilter detects prompt injection and jailbreak attempts
type InjectionFilter struct {
	// Patterns contains known injection patterns
	Patterns []*InjectionPattern

	// ConfidenceThreshold determines when to block (0.0-1.0)
	ConfidenceThreshold float64
}

// InjectionMatch records a detected injection attempt
type InjectionMatch struct {
	PatternName string
	Description string
	Severity    Severity
	Confidence  float64
	MatchStart  int
	MatchEnd    int
}

// NewInjectionFilter creates a new injection filter from configuration
func NewInjectionFilter(config *InjectionFilterConfig) (*InjectionFilter, error) {
	filter := &InjectionFilter{
		Patterns:            make([]*InjectionPattern, 0),
		ConfidenceThreshold: config.ConfidenceThreshold,
	}

	if filter.ConfidenceThreshold == 0 {
		filter.ConfidenceThreshold = 0.80
	}

	// Add enabled default patterns
	if len(config.EnabledPatterns) > 0 {
		for _, name := range config.EnabledPatterns {
			if pattern, ok := GetInjectionPattern(name); ok {
				filter.Patterns = append(filter.Patterns, pattern)
			}
		}
	} else {
		// If no patterns specified, use all default patterns
		for _, pattern := range DefaultInjectionPatterns {
			filter.Patterns = append(filter.Patterns, pattern)
		}
	}

	// Add custom patterns
	for _, def := range config.Patterns {
		pattern, err := CompileInjectionPattern(def)
		if err != nil {
			return nil, err
		}
		filter.Patterns = append(filter.Patterns, pattern)
	}

	return filter, nil
}

// Name returns the filter name
func (f *InjectionFilter) Name() string {
	return "injection_detection"
}

// Process detects injection attempts in the message
func (f *InjectionFilter) Process(_ context.Context, msg *Message) (*FilterResult, error) {
	text := msg.Content.Text

	// Check against all patterns
	for _, pattern := range f.Patterns {
		if pattern.Pattern.MatchString(text) {
			matches := pattern.Pattern.FindAllStringSubmatchIndex(text, -1)

			// Check if confidence exceeds threshold
			if pattern.Confidence >= f.ConfidenceThreshold {
				violation := NewViolation(f.Name(), pattern.Severity, msg).
					WithAction(ViolationActionBlocked).
					WithConfidence(pattern.Confidence).
					WithDetail("pattern_name", pattern.Name).
					WithDetail("pattern_description", pattern.Description).
					WithDetail("match_count", len(matches)).
					WithOriginalContent(truncateForAudit(text, 200))

				return NewFilterResult(false).
					WithViolation(violation).
					WithConfidence(pattern.Confidence).
					WithMetadata("pattern_name", pattern.Name).
					WithMetadata("severity", pattern.Severity), nil
			}

			// Below threshold - log but don't block
			// This could be extended to flag for review
		}
	}

	// No injection detected
	return NewFilterResult(true).WithConfidence(1.0), nil
}

// DetectAll finds all injection patterns in text (for analysis/testing)
func (f *InjectionFilter) DetectAll(text string) []InjectionMatch {
	matches := make([]InjectionMatch, 0)

	for _, pattern := range f.Patterns {
		locs := pattern.Pattern.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			matches = append(matches, InjectionMatch{
				PatternName: pattern.Name,
				Description: pattern.Description,
				Severity:    pattern.Severity,
				Confidence:  pattern.Confidence,
				MatchStart:  loc[0],
				MatchEnd:    loc[1],
			})
		}
	}

	return matches
}

// HighestSeverityMatch returns the highest severity match
func (f *InjectionFilter) HighestSeverityMatch(matches []InjectionMatch) *InjectionMatch {
	if len(matches) == 0 {
		return nil
	}

	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	highest := &matches[0]
	for i := 1; i < len(matches); i++ {
		if severityOrder[matches[i].Severity] > severityOrder[highest.Severity] {
			highest = &matches[i]
		}
	}

	return highest
}

// truncateForAudit truncates text for audit logging
func truncateForAudit(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "...[TRUNCATED]"
}
