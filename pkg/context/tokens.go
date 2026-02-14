package context

import (
	"strings"
	"unicode"
)

// TokenEstimator provides token counting utilities.
// These are estimates based on common tokenization patterns.
// For exact counts, use model-specific tokenizers.

// DefaultCharsPerToken is the average characters per token for most LLMs.
// Claude uses roughly 4 characters per token for English text.
const DefaultCharsPerToken = 4

// EstimateTokens estimates token count for a string.
// Uses a heuristic of ~4 characters per token, which is accurate
// for English text with Claude models.
func EstimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return len(s) / DefaultCharsPerToken
}

// EstimateTokensForModel estimates tokens for a specific model.
// Currently all models use the same estimate, but this allows
// for model-specific adjustments in the future.
func EstimateTokensForModel(s string, model string) int {
	// Model-specific adjustments could be added here
	// For now, use default estimation
	switch {
	case strings.HasPrefix(model, "claude"):
		return EstimateTokens(s)
	case strings.HasPrefix(model, "gpt"):
		// GPT models have slightly different tokenization
		return len(s) / 4
	default:
		return EstimateTokens(s)
	}
}

// CountWords counts words in a string (useful for rough estimates)
func CountWords(s string) int {
	words := 0
	inWord := false

	for _, r := range s {
		if unicode.IsSpace(r) {
			if inWord {
				words++
				inWord = false
			}
		} else {
			inWord = true
		}
	}

	if inWord {
		words++
	}

	return words
}

// TokensFromWords estimates tokens from word count.
// Roughly 1.3 tokens per word for English.
func TokensFromWords(wordCount int) int {
	return int(float64(wordCount) * 1.3)
}

// FitsInBudget checks if content fits within a token budget.
func FitsInBudget(content string, budget int) bool {
	return EstimateTokens(content) <= budget
}

// TruncateToBudget truncates content to fit within a token budget.
// Attempts to truncate at word boundaries.
func TruncateToBudget(content string, budget int) string {
	if budget <= 0 {
		return ""
	}

	currentTokens := EstimateTokens(content)
	if currentTokens <= budget {
		return content
	}

	// Estimate character limit
	charLimit := budget * DefaultCharsPerToken

	if charLimit >= len(content) {
		return content
	}

	// Find last space before limit to avoid cutting words
	truncated := content[:charLimit]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 && lastSpace > charLimit-50 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// BudgetAllocation helps allocate token budget across multiple content sections.
type BudgetAllocation struct {
	TotalBudget int
	Allocated   int
	Sections    map[string]int
}

// NewBudgetAllocation creates a new budget allocation tracker.
func NewBudgetAllocation(totalBudget int) *BudgetAllocation {
	return &BudgetAllocation{
		TotalBudget: totalBudget,
		Allocated:   0,
		Sections:    make(map[string]int),
	}
}

// Remaining returns the remaining budget.
func (b *BudgetAllocation) Remaining() int {
	return b.TotalBudget - b.Allocated
}

// Allocate allocates budget for a section. Returns the actual allocation
// (may be less than requested if budget is exhausted).
func (b *BudgetAllocation) Allocate(section string, requested int) int {
	remaining := b.Remaining()
	if requested > remaining {
		requested = remaining
	}

	b.Sections[section] = requested
	b.Allocated += requested
	return requested
}

// AllocateProportionally allocates remaining budget proportionally across sections.
func (b *BudgetAllocation) AllocateProportionally(sections []string, weights []float64) map[string]int {
	if len(sections) != len(weights) {
		return nil
	}

	remaining := b.Remaining()
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}

	result := make(map[string]int)
	for i, section := range sections {
		allocation := int(float64(remaining) * (weights[i] / totalWeight))
		b.Sections[section] = allocation
		b.Allocated += allocation
		result[section] = allocation
	}

	return result
}
