package agenticloop

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/google/uuid"
)

// LoopManager manages loop entity lifecycle and state
type LoopManager struct {
	loops           map[string]*agentic.LoopEntity
	contextManagers map[string]*ContextManager // loopID -> ContextManager
	pendingTools    map[string]map[string]bool // loopID -> map[callID]bool
	requestToLoop   map[string]string          // requestID -> loopID
	toolCallToLoop  map[string]string          // callID -> loopID
	contextConfig   ContextConfig              // shared context config
	mu              sync.RWMutex
}

// NewLoopManager creates a new LoopManager
func NewLoopManager() *LoopManager {
	return &LoopManager{
		loops:           make(map[string]*agentic.LoopEntity),
		contextManagers: make(map[string]*ContextManager),
		pendingTools:    make(map[string]map[string]bool),
		requestToLoop:   make(map[string]string),
		toolCallToLoop:  make(map[string]string),
		contextConfig:   DefaultContextConfig(),
	}
}

// NewLoopManagerWithConfig creates a new LoopManager with custom context config
func NewLoopManagerWithConfig(contextConfig ContextConfig) *LoopManager {
	return &LoopManager{
		loops:           make(map[string]*agentic.LoopEntity),
		contextManagers: make(map[string]*ContextManager),
		pendingTools:    make(map[string]map[string]bool),
		requestToLoop:   make(map[string]string),
		toolCallToLoop:  make(map[string]string),
		contextConfig:   contextConfig,
	}
}

// CreateLoop creates a new loop entity with a generated UUID
func (m *LoopManager) CreateLoop(taskID, role, model string, maxIterations ...int) (string, error) {
	loopID := uuid.New().String()
	return m.CreateLoopWithID(loopID, taskID, role, model, maxIterations...)
}

// CreateLoopWithID creates a new loop entity with a specific ID
func (m *LoopManager) CreateLoopWithID(loopID, taskID, role, model string, maxIterations ...int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Determine max iterations
	maxIter := 20 // default
	if len(maxIterations) > 0 && maxIterations[0] > 0 {
		maxIter = maxIterations[0]
	}

	entity := agentic.NewLoopEntity(loopID, taskID, role, model, maxIter)

	m.loops[loopID] = &entity
	m.pendingTools[loopID] = make(map[string]bool)

	// Create context manager for this loop if context management is enabled
	if m.contextConfig.Enabled {
		m.contextManagers[loopID] = NewContextManager(loopID, model, m.contextConfig)
	}

	return loopID, nil
}

// GetLoop retrieves a loop entity by ID
func (m *LoopManager) GetLoop(loopID string) (agentic.LoopEntity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if loopID == "" {
		return agentic.LoopEntity{}, fmt.Errorf("loop ID cannot be empty")
	}

	entity, exists := m.loops[loopID]
	if !exists {
		return agentic.LoopEntity{}, fmt.Errorf("loop %s not found", loopID)
	}

	return *entity, nil
}

// UpdateLoop updates an existing loop entity
func (m *LoopManager) UpdateLoop(entity agentic.LoopEntity) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.loops[entity.ID]; !exists {
		return fmt.Errorf("loop %s not found", entity.ID)
	}

	m.loops[entity.ID] = &entity
	return nil
}

// DeleteLoop deletes a loop entity
func (m *LoopManager) DeleteLoop(loopID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.loops, loopID)
	delete(m.pendingTools, loopID)
	delete(m.contextManagers, loopID)
	return nil
}

// GetContextManager retrieves the context manager for a loop
func (m *LoopManager) GetContextManager(loopID string) *ContextManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contextManagers[loopID]
}

// GetCurrentIteration returns the current iteration for a loop
func (m *LoopManager) GetCurrentIteration(loopID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return 0
	}
	return entity.Iterations
}

// TransitionLoop transitions a loop to a new state
func (m *LoopManager) TransitionLoop(loopID string, newState agentic.LoopState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	return entity.TransitionTo(newState)
}

// IncrementIteration increments the loop iteration counter
func (m *LoopManager) IncrementIteration(loopID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	return entity.IncrementIteration()
}

// AddPendingTool adds a pending tool call to the loop
func (m *LoopManager) AddPendingTool(loopID, callID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.loops[loopID]; !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	if m.pendingTools[loopID] == nil {
		m.pendingTools[loopID] = make(map[string]bool)
	}

	m.pendingTools[loopID][callID] = true
	return nil
}

// RemovePendingTool removes a pending tool call from the loop
func (m *LoopManager) RemovePendingTool(loopID, callID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pendingTools[loopID] != nil {
		delete(m.pendingTools[loopID], callID)
	}

	return nil
}

// GetPendingTools returns all pending tool calls for a loop
func (m *LoopManager) GetPendingTools(loopID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending := m.pendingTools[loopID]
	if pending == nil {
		return []string{}
	}

	result := make([]string, 0, len(pending))
	for callID := range pending {
		result = append(result, callID)
	}

	return result
}

// AllToolsComplete returns true if there are no pending tool calls
func (m *LoopManager) AllToolsComplete(loopID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending := m.pendingTools[loopID]
	return len(pending) == 0
}

// TrackRequest associates a request ID with a loop ID
func (m *LoopManager) TrackRequest(requestID, loopID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestToLoop[requestID] = loopID
}

// GetLoopForRequest retrieves the loop ID for a request ID
func (m *LoopManager) GetLoopForRequest(requestID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	loopID, exists := m.requestToLoop[requestID]
	return loopID, exists
}

// TrackToolCall associates a tool call ID with a loop ID
func (m *LoopManager) TrackToolCall(callID, loopID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCallToLoop[callID] = loopID
}

// GetLoopForToolCall retrieves the loop ID for a tool call ID
func (m *LoopManager) GetLoopForToolCall(callID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	loopID, exists := m.toolCallToLoop[callID]
	return loopID, exists
}

// StoreToolResult stores a tool result in the loop entity for later retrieval
func (m *LoopManager) StoreToolResult(loopID string, result agentic.ToolResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	if entity.PendingToolResults == nil {
		entity.PendingToolResults = make(map[string]agentic.ToolResult)
	}
	entity.PendingToolResults[result.CallID] = result
	return nil
}

// GetAndClearToolResults retrieves all accumulated tool results and clears them
func (m *LoopManager) GetAndClearToolResults(loopID string) []agentic.ToolResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return nil
	}

	results := make([]agentic.ToolResult, 0, len(entity.PendingToolResults))
	for _, r := range entity.PendingToolResults {
		results = append(results, r)
	}
	entity.PendingToolResults = nil
	return results
}

// SetTimeout sets the timeout for a loop
func (m *LoopManager) SetTimeout(loopID string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	now := time.Now()
	entity.StartedAt = now
	entity.TimeoutAt = now.Add(timeout)
	return nil
}

// IsTimedOut checks if a loop has exceeded its timeout
func (m *LoopManager) IsTimedOut(loopID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return false
	}

	// If no timeout set, not timed out
	if entity.TimeoutAt.IsZero() {
		return false
	}

	return time.Now().After(entity.TimeoutAt)
}

// SetParentLoop sets the parent loop ID for tracking architect->editor relationships
func (m *LoopManager) SetParentLoop(loopID, parentLoopID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	entity.ParentLoopID = parentLoopID
	return nil
}

// SetWorkflowContext sets the workflow slug and step for loops created by workflow commands
func (m *LoopManager) SetWorkflowContext(loopID, workflowSlug, workflowStep string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	entity.WorkflowSlug = workflowSlug
	entity.WorkflowStep = workflowStep
	return nil
}

// SetUserContext sets the user routing info for error notifications
func (m *LoopManager) SetUserContext(loopID, channelType, channelID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, exists := m.loops[loopID]
	if !exists {
		return fmt.Errorf("loop %s not found", loopID)
	}

	entity.ChannelType = channelType
	entity.ChannelID = channelID
	entity.UserID = userID
	return nil
}

// GenerateRequestID creates a structured request ID that embeds the loop ID.
// Format: loopID:req:shortUUID
// This allows recovery of loop ID from request ID if in-memory maps are lost.
func (m *LoopManager) GenerateRequestID(loopID string) string {
	shortID := uuid.New().String()[:8]
	return fmt.Sprintf("%s:req:%s", loopID, shortID)
}

// GenerateToolCallID creates a structured tool call ID that embeds the loop ID.
// Format: loopID:tool:shortUUID
// This allows recovery of loop ID from tool call ID if in-memory maps are lost.
func (m *LoopManager) GenerateToolCallID(loopID string) string {
	shortID := uuid.New().String()[:8]
	return fmt.Sprintf("%s:tool:%s", loopID, shortID)
}

// ExtractLoopIDFromRequest extracts the loop ID from a structured request ID.
// Returns empty string if the ID is not in structured format.
func (m *LoopManager) ExtractLoopIDFromRequest(requestID string) string {
	parts := strings.Split(requestID, ":req:")
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// ExtractLoopIDFromToolCall extracts the loop ID from a structured tool call ID.
// Returns empty string if the ID is not in structured format.
func (m *LoopManager) ExtractLoopIDFromToolCall(toolCallID string) string {
	parts := strings.Split(toolCallID, ":tool:")
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// GetLoopForRequestWithRecovery retrieves the loop ID for a request ID,
// attempting recovery from structured ID if not found in cache.
func (m *LoopManager) GetLoopForRequestWithRecovery(requestID string) (string, bool) {
	// Try cache first
	if loopID, exists := m.GetLoopForRequest(requestID); exists {
		return loopID, true
	}

	// Try to extract from structured ID
	if loopID := m.ExtractLoopIDFromRequest(requestID); loopID != "" {
		// Verify loop exists
		m.mu.RLock()
		_, exists := m.loops[loopID]
		m.mu.RUnlock()
		if exists {
			// Re-establish the mapping
			m.TrackRequest(requestID, loopID)
			return loopID, true
		}
	}

	return "", false
}

// GetLoopForToolCallWithRecovery retrieves the loop ID for a tool call ID,
// attempting recovery from structured ID if not found in cache.
func (m *LoopManager) GetLoopForToolCallWithRecovery(toolCallID string) (string, bool) {
	// Try cache first
	if loopID, exists := m.GetLoopForToolCall(toolCallID); exists {
		return loopID, true
	}

	// Try to extract from structured ID
	if loopID := m.ExtractLoopIDFromToolCall(toolCallID); loopID != "" {
		// Verify loop exists
		m.mu.RLock()
		_, exists := m.loops[loopID]
		m.mu.RUnlock()
		if exists {
			// Re-establish the mapping
			m.TrackToolCall(toolCallID, loopID)
			return loopID, true
		}
	}

	return "", false
}
