package agenticloop

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/c360studio/semstreams/agentic"
)

// RegionType defines the type of context region
type RegionType string

// Context region types define different areas of the conversation context
const (
	RegionSystemPrompt     RegionType = "system_prompt"     // System prompt and instructions
	RegionCompactedHistory RegionType = "compacted_history" // Summarized old conversation
	RegionRecentHistory    RegionType = "recent_history"    // Recent uncompacted messages
	RegionToolResults      RegionType = "tool_results"      // Tool execution results
	RegionHydratedContext  RegionType = "hydrated_context"  // Retrieved context from memory
)

// Region priorities (higher = more important, evict lower first)
var regionPriorities = map[RegionType]int{
	RegionToolResults:      1, // Evict first
	RegionRecentHistory:    2,
	RegionHydratedContext:  3,
	RegionCompactedHistory: 4,
	RegionSystemPrompt:     5, // Never evict
}

type contextMessage struct {
	Message   agentic.ChatMessage
	Tokens    int
	Iteration int // For GC - which iteration this was added
}

// ContextManager manages conversation context with memory optimization
type ContextManager struct {
	loopID           string
	model            string
	modelLimit       int
	config           ContextConfig
	regions          map[RegionType][]contextMessage
	mu               sync.RWMutex
	currentIteration int
	logger           *slog.Logger
}

// ContextManagerOption is a functional option for configuring ContextManager
type ContextManagerOption func(*ContextManager)

// WithLogger sets the logger for the ContextManager
func WithLogger(logger *slog.Logger) ContextManagerOption {
	return func(cm *ContextManager) {
		cm.logger = logger
	}
}

// NewContextManager creates a new context manager
func NewContextManager(loopID, model string, config ContextConfig, opts ...ContextManagerOption) *ContextManager {
	cm := &ContextManager{
		loopID:           loopID,
		model:            model,
		config:           config,
		regions:          make(map[RegionType][]contextMessage),
		currentIteration: 1,
		logger:           slog.Default(),
	}

	// Apply functional options
	for _, opt := range opts {
		opt(cm)
	}

	// Resolve model limit with logging
	cm.modelLimit = cm.resolveModelLimit(model)

	// Initialize empty regions
	for rt := range regionPriorities {
		cm.regions[rt] = []contextMessage{}
	}

	return cm
}

// resolveModelLimit looks up the model limit with fallback logging
func (cm *ContextManager) resolveModelLimit(model string) int {
	if limit, ok := cm.config.ModelLimits[model]; ok {
		return limit
	}

	defaultLimit := cm.config.ModelLimits[DefaultModelKey]
	cm.logger.Warn("model not in config, using default context limit",
		"loop_id", cm.loopID,
		"model", model,
		"default_limit", defaultLimit,
		"hint", "add to model_limits config for explicit limit",
	)
	return defaultLimit
}

// Utilization returns the current context utilization (0.0 to 1.0)
func (cm *ContextManager) Utilization() float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := 0
	for _, messages := range cm.regions {
		for _, m := range messages {
			total += m.Tokens
		}
	}

	return float64(total) / float64(cm.modelLimit)
}

// ShouldCompact returns true if compaction should be triggered
func (cm *ContextManager) ShouldCompact() bool {
	return cm.Utilization() >= cm.config.CompactThreshold
}

// GetContext returns all messages in region priority order
func (cm *ContextManager) GetContext() []agentic.ChatMessage {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var messages []agentic.ChatMessage

	// Order by region priority: System -> Compacted -> Hydrated -> Recent -> Tools
	order := []RegionType{
		RegionSystemPrompt,
		RegionCompactedHistory,
		RegionHydratedContext,
		RegionRecentHistory,
		RegionToolResults,
	}

	for _, rt := range order {
		for _, cm := range cm.regions[rt] {
			messages = append(messages, cm.Message)
		}
	}

	return messages
}

// AddMessage adds a message to a specific region
func (cm *ContextManager) AddMessage(region RegionType, msg agentic.ChatMessage) error {
	if _, valid := regionPriorities[region]; !valid {
		return fmt.Errorf("invalid region type: %s", region)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tokens := estimateTokens(msg.Content)
	cm.regions[region] = append(cm.regions[region], contextMessage{
		Message:   msg,
		Tokens:    tokens,
		Iteration: cm.currentIteration,
	})

	return nil
}

// AdvanceIteration moves to the next iteration. Call this after processing
// each loop iteration to ensure proper age tracking for GC.
func (cm *ContextManager) AdvanceIteration() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.currentIteration++
}

// GetRegionTokens returns the total token count for a region
func (cm *ContextManager) GetRegionTokens(region RegionType) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := 0
	for _, m := range cm.regions[region] {
		total += m.Tokens
	}
	return total
}

// GCToolResults garbage collects old tool results based on age
func (cm *ContextManager) GCToolResults(currentIteration int) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	toolResults := cm.regions[RegionToolResults]
	if len(toolResults) == 0 {
		return 0
	}

	evicted := 0
	remaining := []contextMessage{}

	for _, m := range toolResults {
		age := currentIteration - m.Iteration
		if age <= cm.config.ToolResultMaxAge {
			remaining = append(remaining, m)
		} else {
			evicted++
		}
	}

	cm.regions[RegionToolResults] = remaining

	// Update current iteration for future AddMessage calls
	cm.currentIteration = currentIteration

	return evicted
}

// estimateTokens estimates the token count for a string
// Uses a simple heuristic of ~4 characters per token
func estimateTokens(content string) int {
	return (len(content) + 3) / 4 // Round up
}
