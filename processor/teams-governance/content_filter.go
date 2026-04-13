package teamsgovernance

import (
	"context"
	"math"
	"regexp"
	"strings"
)

// ContentFilter enforces content policies
type ContentFilter struct {
	// Policies to enforce
	Policies []*ContentPolicy

	// BlockThreshold for immediate blocking (0.0-1.0)
	BlockThreshold float64

	// WarnThreshold for logging warnings (0.0-1.0)
	WarnThreshold float64
}

// ContentPolicy defines a content filtering rule
type ContentPolicy struct {
	// Name is the policy identifier
	Name string

	// Keywords to match (case-insensitive)
	Keywords []string

	// Patterns for regex-based matching
	Patterns []*regexp.Regexp

	// Action when policy is violated
	Action PolicyAction

	// Severity of violations
	Severity Severity

	// Categories this policy covers
	Categories []string

	// Weight for scoring (default 1.0)
	Weight float64
}

// PolicyViolation records a policy match
type PolicyViolation struct {
	PolicyName string
	Score      float64
	Action     PolicyAction
	Severity   Severity
	Matches    []string
}

// DefaultContentPolicies provides baseline moderation
var DefaultContentPolicies = map[string]*ContentPolicy{
	"harmful": {
		Name:       "harmful",
		Keywords:   []string{"violence", "self-harm", "suicide", "murder", "kill", "attack", "weapon"},
		Action:     PolicyActionBlock,
		Severity:   SeverityHigh,
		Categories: []string{"violence", "self-harm"},
		Weight:     1.0,
	},
	"illegal": {
		Name:       "illegal",
		Keywords:   []string{"drugs", "trafficking", "fraud", "money laundering", "terrorism", "exploit"},
		Action:     PolicyActionBlock,
		Severity:   SeverityCritical,
		Categories: []string{"illegal", "criminal"},
		Weight:     1.5,
	},
	"hate": {
		Name:       "hate",
		Keywords:   []string{"hate speech", "discrimination", "racist", "sexist", "slur"},
		Action:     PolicyActionBlock,
		Severity:   SeverityHigh,
		Categories: []string{"hate", "discrimination"},
		Weight:     1.0,
	},
	"spam": {
		Name: "spam",
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(buy now|click here|limited time|act now|free offer).*(http|www)`),
			regexp.MustCompile(`(?i)(winner|won|prize|lottery).*(claim|collect|receive)`),
		},
		Action:     PolicyActionFlag,
		Severity:   SeverityLow,
		Categories: []string{"spam", "marketing"},
		Weight:     0.5,
	},
}

// NewContentFilter creates a new content filter from configuration
func NewContentFilter(config *ContentFilterConfig) (*ContentFilter, error) {
	filter := &ContentFilter{
		Policies:       make([]*ContentPolicy, 0),
		BlockThreshold: config.BlockThreshold,
		WarnThreshold:  config.WarnThreshold,
	}

	if filter.BlockThreshold == 0 {
		filter.BlockThreshold = 0.90
	}

	if filter.WarnThreshold == 0 {
		filter.WarnThreshold = 0.70
	}

	// Add enabled default policies
	if len(config.EnabledDefault) > 0 {
		for _, name := range config.EnabledDefault {
			if policy, ok := DefaultContentPolicies[name]; ok {
				filter.Policies = append(filter.Policies, policy)
			}
		}
	}

	// Add custom policies
	for _, def := range config.Policies {
		policy, err := buildPolicy(def)
		if err != nil {
			return nil, err
		}
		filter.Policies = append(filter.Policies, policy)
	}

	return filter, nil
}

// buildPolicy creates a ContentPolicy from a definition
func buildPolicy(def ContentPolicyDef) (*ContentPolicy, error) {
	policy := &ContentPolicy{
		Name:       def.Name,
		Keywords:   def.Keywords,
		Action:     def.Action,
		Severity:   def.Severity,
		Categories: def.Categories,
		Weight:     1.0,
	}

	// Compile patterns
	for _, patternStr := range def.Patterns {
		regex, err := regexp.Compile(patternStr)
		if err != nil {
			return nil, err
		}
		policy.Patterns = append(policy.Patterns, regex)
	}

	return policy, nil
}

// Name returns the filter name
func (f *ContentFilter) Name() string {
	return "content_moderation"
}

// Process checks content against policies
func (f *ContentFilter) Process(_ context.Context, msg *Message) (*FilterResult, error) {
	text := strings.ToLower(msg.Content.Text)
	violations := make([]PolicyViolation, 0)

	for _, policy := range f.Policies {
		score, matches := f.scorePolicy(text, policy)

		if score >= f.BlockThreshold {
			violations = append(violations, PolicyViolation{
				PolicyName: policy.Name,
				Score:      score,
				Action:     policy.Action,
				Severity:   policy.Severity,
				Matches:    matches,
			})
		} else if score >= f.WarnThreshold {
			// Log warning but don't necessarily block
			violations = append(violations, PolicyViolation{
				PolicyName: policy.Name,
				Score:      score,
				Action:     PolicyActionFlag,
				Severity:   SeverityLow,
				Matches:    matches,
			})
		}
	}

	// No violations above threshold
	if len(violations) == 0 {
		return NewFilterResult(true).WithConfidence(1.0), nil
	}

	// Determine overall action
	maxSeverity := f.maxViolationSeverity(violations)
	shouldBlock := f.shouldBlock(violations)
	maxScore := f.maxViolationScore(violations)

	violation := NewViolation(f.Name(), maxSeverity, msg).
		WithConfidence(maxScore).
		WithDetail("policy_violations", summarizePolicyViolations(violations)).
		WithDetail("violation_count", len(violations)).
		WithOriginalContent(truncateForAudit(msg.Content.Text, 200))

	if shouldBlock {
		violation.WithAction(ViolationActionBlocked)
		return NewFilterResult(false).
			WithViolation(violation).
			WithConfidence(maxScore).
			WithMetadata("blocked_policies", getBlockedPolicyNames(violations)), nil
	}

	// Flag but allow
	violation.WithAction(ViolationActionFlagged)
	return NewFilterResult(true).
		WithViolation(violation).
		WithConfidence(maxScore).
		WithMetadata("flagged_policies", getFlaggedPolicyNames(violations)), nil
}

// scorePolicy calculates policy violation score
func (f *ContentFilter) scorePolicy(text string, policy *ContentPolicy) (float64, []string) {
	matches := make([]string, 0)
	matchCount := 0.0
	totalChecks := float64(len(policy.Keywords) + len(policy.Patterns))

	if totalChecks == 0 {
		return 0, matches
	}

	// Keyword matching
	for _, keyword := range policy.Keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			matchCount++
			matches = append(matches, keyword)
		}
	}

	// Pattern matching (weighted higher than keywords)
	for _, pattern := range policy.Patterns {
		if pattern.MatchString(text) {
			matchCount += 2 // Patterns weighted higher
			found := pattern.FindString(text)
			if found != "" {
				matches = append(matches, found)
			}
		}
	}

	// Calculate normalized score with weight
	score := (matchCount / totalChecks) * policy.Weight

	return math.Min(score, 1.0), matches
}

// maxViolationSeverity returns the highest severity among violations
func (f *ContentFilter) maxViolationSeverity(violations []PolicyViolation) Severity {
	if len(violations) == 0 {
		return SeverityLow
	}

	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	highest := violations[0].Severity
	for _, v := range violations[1:] {
		if severityOrder[v.Severity] > severityOrder[highest] {
			highest = v.Severity
		}
	}

	return highest
}

// maxViolationScore returns the highest score among violations
func (f *ContentFilter) maxViolationScore(violations []PolicyViolation) float64 {
	if len(violations) == 0 {
		return 0
	}

	maxScore := violations[0].Score
	for _, v := range violations[1:] {
		if v.Score > maxScore {
			maxScore = v.Score
		}
	}

	return maxScore
}

// shouldBlock determines if the message should be blocked
func (f *ContentFilter) shouldBlock(violations []PolicyViolation) bool {
	for _, v := range violations {
		if v.Action == PolicyActionBlock && v.Score >= f.BlockThreshold {
			return true
		}
	}
	return false
}

// summarizePolicyViolations creates a summary for metadata
func summarizePolicyViolations(violations []PolicyViolation) []map[string]any {
	summary := make([]map[string]any, len(violations))
	for i, v := range violations {
		summary[i] = map[string]any{
			"policy":   v.PolicyName,
			"score":    v.Score,
			"action":   v.Action,
			"severity": v.Severity,
			"matches":  len(v.Matches),
		}
	}
	return summary
}

// getBlockedPolicyNames returns names of policies that triggered blocks
func getBlockedPolicyNames(violations []PolicyViolation) []string {
	names := make([]string, 0)
	for _, v := range violations {
		if v.Action == PolicyActionBlock {
			names = append(names, v.PolicyName)
		}
	}
	return names
}

// getFlaggedPolicyNames returns names of policies that triggered flags
func getFlaggedPolicyNames(violations []PolicyViolation) []string {
	names := make([]string, 0)
	for _, v := range violations {
		if v.Action == PolicyActionFlag {
			names = append(names, v.PolicyName)
		}
	}
	return names
}
