package agenticloop

import (
	"fmt"
	"sync"

	"github.com/c360/semstreams/agentic"
	"github.com/google/uuid"
)

// LoopManager manages loop entity lifecycle and state
type LoopManager struct {
	loops          map[string]*agentic.LoopEntity
	pendingTools   map[string]map[string]bool // loopID -> map[callID]bool
	requestToLoop  map[string]string          // requestID -> loopID
	toolCallToLoop map[string]string          // callID -> loopID
	mu             sync.RWMutex
}

// NewLoopManager creates a new LoopManager
func NewLoopManager() *LoopManager {
	return &LoopManager{
		loops:          make(map[string]*agentic.LoopEntity),
		pendingTools:   make(map[string]map[string]bool),
		requestToLoop:  make(map[string]string),
		toolCallToLoop: make(map[string]string),
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
	return nil
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
