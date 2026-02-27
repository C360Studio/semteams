package agenticloop

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/pkg/errs"
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
	RegionGraphEntities    RegionType = "graph_entities"    // Graph entity context (multi-agent)
)

// Region priorities (higher = more important, evict lower first)
// Graph-aware slicing adjusts these based on EntityPriority config
var regionPriorities = map[RegionType]int{
	RegionToolResults:      1, // Evict first
	RegionRecentHistory:    2,
	RegionHydratedContext:  3,
	RegionGraphEntities:    4, // Entity context (adjustable via EntityPriority)
	RegionCompactedHistory: 5,
	RegionSystemPrompt:     6, // Never evict
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

	// Order by region priority: System -> Compacted -> Graph Entities -> Hydrated -> Recent -> Tools
	order := []RegionType{
		RegionSystemPrompt,
		RegionCompactedHistory,
		RegionGraphEntities,
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
		return errs.WrapInvalid(fmt.Errorf("invalid region type: %s", region), "ContextManager", "AddMessage", "validate region type")
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

// ContextSlice defines which regions to include when slicing context
type ContextSlice struct {
	IncludeRegions   []RegionType // Regions to include
	ExcludeRegions   []RegionType // Regions to exclude (takes precedence)
	PreserveEntities []string     // Entity IDs to always keep

	// Boid steering configuration (from active steering signals)
	AvoidEntities []string // Entities to deprioritize (from separation signals)
}

// CheckBudget checks if current context is within the token budget.
// Returns (withinBudget, currentTokens).
func (cm *ContextManager) CheckBudget(budgetTokens int) (bool, int) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if budgetTokens <= 0 {
		// No budget limit
		return true, cm.getTotalTokensLocked()
	}

	current := cm.getTotalTokensLocked()
	return current <= budgetTokens, current
}

// getTotalTokensLocked returns total tokens (must hold read lock)
func (cm *ContextManager) getTotalTokensLocked() int {
	total := 0
	for _, messages := range cm.regions {
		for _, m := range messages {
			total += m.Tokens
		}
	}
	return total
}

// SliceForBudget removes content to fit within the budget.
// Uses graph-aware slicing that preserves entity context when EntityPriority is set.
// Boid steering signals influence eviction: AvoidEntities are evicted first.
func (cm *ContextManager) SliceForBudget(budget int, slice ContextSlice) error {
	if budget <= 0 {
		return nil // No budget limit
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	current := cm.getTotalTokensLocked()
	if current <= budget {
		return nil // Already within budget
	}

	toEvict := current - budget
	evicted := 0

	// Build avoid set from Boid separation signals
	avoidSet := make(map[string]bool)
	for _, e := range slice.AvoidEntities {
		avoidSet[e] = true
	}

	// Get effective priorities (adjusted for entity priority if set)
	priorities := cm.getEffectivePriorities()

	// Build eviction order (lowest priority first)
	order := cm.buildEvictionOrder(priorities, slice)

	// First pass: evict avoided entities from all regions
	if len(avoidSet) > 0 {
		for _, region := range order {
			if evicted >= toEvict {
				break
			}

			messages := cm.regions[region]
			if len(messages) == 0 {
				continue
			}

			// Partition: keep non-avoided, evict avoided
			var kept []contextMessage
			for _, msg := range messages {
				entityID := extractEntityIDFromMessage(msg.Message.Content)
				if avoidSet[entityID] && evicted < toEvict && !cm.shouldPreserve(msg, slice.PreserveEntities) {
					evicted += msg.Tokens
				} else {
					kept = append(kept, msg)
				}
			}
			cm.regions[region] = kept
		}
	}

	// Second pass: evict remaining messages if still over budget
	for _, region := range order {
		if evicted >= toEvict {
			break
		}

		messages := cm.regions[region]
		if len(messages) == 0 {
			continue
		}

		// Evict from the end (oldest first for most regions)
		newMessages := []contextMessage{}
		for i := len(messages) - 1; i >= 0 && evicted < toEvict; i-- {
			msg := messages[i]

			// Check if this message is for a preserved entity
			if cm.shouldPreserve(msg, slice.PreserveEntities) {
				newMessages = append([]contextMessage{msg}, newMessages...)
				continue
			}

			evicted += msg.Tokens
		}

		// Keep non-evicted messages
		for i := 0; i < len(messages)-len(newMessages); i++ {
			if i < len(messages) {
				msg := messages[i]
				if cm.shouldPreserve(msg, slice.PreserveEntities) {
					newMessages = append([]contextMessage{msg}, newMessages...)
				}
			}
		}

		cm.regions[region] = newMessages
	}

	cm.logger.Info("Context sliced for budget",
		"loop_id", cm.loopID,
		"budget", budget,
		"evicted_tokens", evicted,
		"new_total", cm.getTotalTokensLocked())

	return nil
}

// getEffectivePriorities returns region priorities adjusted for entity priority
func (cm *ContextManager) getEffectivePriorities() map[RegionType]int {
	priorities := make(map[RegionType]int)
	for k, v := range regionPriorities {
		priorities[k] = v
	}

	// Adjust entity priority based on config
	if cm.config.EntityPriority > 0 {
		// Higher EntityPriority = higher priority for graph entities
		// Scale: 1-10 maps to priority adjustment of 0-5
		adjustment := cm.config.EntityPriority / 2
		priorities[RegionGraphEntities] += adjustment
	}

	return priorities
}

// buildEvictionOrder builds the order in which to evict regions
func (cm *ContextManager) buildEvictionOrder(priorities map[RegionType]int, slice ContextSlice) []RegionType {
	// Collect regions that can be evicted
	var evictable []RegionType
	excludeSet := make(map[RegionType]bool)
	for _, r := range slice.ExcludeRegions {
		excludeSet[r] = true
	}

	for region := range cm.regions {
		// Never evict system prompt
		if region == RegionSystemPrompt {
			continue
		}
		// Skip excluded regions
		if excludeSet[region] {
			continue
		}
		evictable = append(evictable, region)
	}

	// Sort by priority (lowest first)
	for i := 0; i < len(evictable)-1; i++ {
		for j := i + 1; j < len(evictable); j++ {
			if priorities[evictable[i]] > priorities[evictable[j]] {
				evictable[i], evictable[j] = evictable[j], evictable[i]
			}
		}
	}

	return evictable
}

// shouldPreserve checks if a message should be preserved based on entity IDs
func (cm *ContextManager) shouldPreserve(msg contextMessage, preserveEntities []string) bool {
	if len(preserveEntities) == 0 {
		return false
	}

	// Check if message content contains any preserved entity ID
	content := msg.Message.Content
	for _, entityID := range preserveEntities {
		if strings.Contains(content, entityID) {
			return true
		}
	}

	return false
}

// AddGraphEntityContext adds context from graph entities to the dedicated region
func (cm *ContextManager) AddGraphEntityContext(entityID string, content string) error {
	msg := agentic.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("[Entity: %s]\n%s", entityID, content),
	}
	return cm.AddMessage(RegionGraphEntities, msg)
}

// GetGraphEntityTokens returns the total tokens in the graph entities region
func (cm *ContextManager) GetGraphEntityTokens() int {
	return cm.GetRegionTokens(RegionGraphEntities)
}

// ClearGraphEntities clears the graph entities region
func (cm *ContextManager) ClearGraphEntities() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.regions[RegionGraphEntities] = []contextMessage{}
}

// BoidSteeringConfig holds Boid steering signal configuration for context building.
type BoidSteeringConfig struct {
	// PrioritizeEntities are entities to move earlier in context (cohesion).
	PrioritizeEntities []string

	// AvoidEntities are entities to deprioritize in context (separation).
	AvoidEntities []string

	// AlignPatterns are predicate patterns to favor (alignment).
	AlignPatterns []string
}

// ApplyBoidSteering reorders graph entity context based on Boid steering signals.
// This moves prioritized entities earlier and avoided entities later in the context.
func (cm *ContextManager) ApplyBoidSteering(steering BoidSteeringConfig) {
	if len(steering.PrioritizeEntities) == 0 && len(steering.AvoidEntities) == 0 {
		return
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	graphEntities := cm.regions[RegionGraphEntities]
	if len(graphEntities) == 0 {
		return
	}

	// Build lookup sets
	prioritizeSet := make(map[string]bool)
	for _, e := range steering.PrioritizeEntities {
		prioritizeSet[e] = true
	}
	avoidSet := make(map[string]bool)
	for _, e := range steering.AvoidEntities {
		avoidSet[e] = true
	}

	// Partition entities: prioritized, normal, avoided
	var prioritized, normal, avoided []contextMessage
	for _, msg := range graphEntities {
		// Extract entity ID from message content (format: "[Entity: {id}]\n...")
		entityID := extractEntityIDFromMessage(msg.Message.Content)

		switch {
		case prioritizeSet[entityID]:
			prioritized = append(prioritized, msg)
		case avoidSet[entityID]:
			avoided = append(avoided, msg)
		default:
			normal = append(normal, msg)
		}
	}

	// Reassemble: prioritized first, then normal, then avoided
	reordered := make([]contextMessage, 0, len(graphEntities))
	reordered = append(reordered, prioritized...)
	reordered = append(reordered, normal...)
	reordered = append(reordered, avoided...)

	cm.regions[RegionGraphEntities] = reordered

	cm.logger.Debug("Applied Boid steering to context",
		"loop_id", cm.loopID,
		"prioritized", len(prioritized),
		"avoided", len(avoided),
		"normal", len(normal))
}

// extractEntityIDFromMessage extracts the entity ID from a graph entity message.
// Expected format: "[Entity: some.entity.id]\nContent..."
func extractEntityIDFromMessage(content string) string {
	// Look for "[Entity: " prefix
	const prefix = "[Entity: "
	start := strings.Index(content, prefix)
	if start == -1 {
		return ""
	}

	// Find the closing bracket
	start += len(prefix)
	end := strings.Index(content[start:], "]")
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(content[start : start+end])
}
