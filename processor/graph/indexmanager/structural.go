// Package indexmanager provides structural index integration for query optimization.
package indexmanager

import (
	"sync"

	"github.com/c360/semstreams/processor/graph/structuralindex"
)

// StructuralIndexHolder manages cached structural indices for query operations.
// Thread-safe access to k-core and pivot indices.
type StructuralIndexHolder struct {
	mu    sync.RWMutex
	kcore *structuralindex.KCoreIndex
	pivot *structuralindex.PivotIndex
}

// NewStructuralIndexHolder creates a new holder for structural indices.
func NewStructuralIndexHolder() *StructuralIndexHolder {
	return &StructuralIndexHolder{}
}

// SetKCoreIndex updates the k-core index (thread-safe).
func (h *StructuralIndexHolder) SetKCoreIndex(index *structuralindex.KCoreIndex) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.kcore = index
}

// SetPivotIndex updates the pivot index (thread-safe).
func (h *StructuralIndexHolder) SetPivotIndex(index *structuralindex.PivotIndex) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pivot = index
}

// GetKCoreIndex returns the current k-core index (thread-safe).
func (h *StructuralIndexHolder) GetKCoreIndex() *structuralindex.KCoreIndex {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.kcore
}

// GetPivotIndex returns the current pivot index (thread-safe).
func (h *StructuralIndexHolder) GetPivotIndex() *structuralindex.PivotIndex {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.pivot
}

// kcoreAdapter wraps *structuralindex.KCoreIndex to implement indexmanager.KCoreIndex interface.
type kcoreAdapter struct {
	index *structuralindex.KCoreIndex
}

func (a *kcoreAdapter) GetCore(entityID string) int {
	if a.index == nil {
		return 0
	}
	return a.index.GetCore(entityID)
}

func (a *kcoreAdapter) FilterByMinCore(entityIDs []string, minCore int) []string {
	if a.index == nil {
		return entityIDs
	}
	return a.index.FilterByMinCore(entityIDs, minCore)
}

func (a *kcoreAdapter) GetEntitiesInCore(core int) []string {
	if a.index == nil {
		return nil
	}
	return a.index.GetEntitiesInCore(core)
}

func (a *kcoreAdapter) GetEntitiesAboveCore(minCore int) []string {
	if a.index == nil {
		return nil
	}
	return a.index.GetEntitiesAboveCore(minCore)
}

// pivotAdapter wraps *structuralindex.PivotIndex to implement indexmanager.PivotIndex interface.
type pivotAdapter struct {
	index *structuralindex.PivotIndex
}

func (a *pivotAdapter) EstimateDistance(entityA, entityB string) (lower, upper int) {
	if a.index == nil {
		return structuralindex.MaxHopDistance, structuralindex.MaxHopDistance
	}
	return a.index.EstimateDistance(entityA, entityB)
}

func (a *pivotAdapter) IsWithinHops(entityA, entityB string, maxHops int) bool {
	if a.index == nil {
		return true // Conservative: assume reachable if no index
	}
	return a.index.IsWithinHops(entityA, entityB, maxHops)
}

func (a *pivotAdapter) GetReachableCandidates(source string, maxHops int) []string {
	if a.index == nil {
		return nil
	}
	return a.index.GetReachableCandidates(source, maxHops)
}

// GetKCoreIndex returns the k-core index interface for query filtering.
// Returns nil if structural indexing is not configured or index not yet computed.
func (m *Manager) GetKCoreIndex() KCoreIndex {
	if m.structuralIndices == nil {
		return nil
	}
	idx := m.structuralIndices.GetKCoreIndex()
	if idx == nil {
		return nil
	}
	return &kcoreAdapter{index: idx}
}

// GetPivotIndex returns the pivot index interface for distance estimation.
// Returns nil if structural indexing is not configured or index not yet computed.
func (m *Manager) GetPivotIndex() PivotIndex {
	if m.structuralIndices == nil {
		return nil
	}
	idx := m.structuralIndices.GetPivotIndex()
	if idx == nil {
		return nil
	}
	return &pivotAdapter{index: idx}
}

// FilterByKCore filters entity IDs to only those with core >= minCore.
// Returns unchanged input if k-core index is not available.
func (m *Manager) FilterByKCore(entityIDs []string, minCore int) []string {
	if m.structuralIndices == nil || minCore <= 0 {
		return entityIDs
	}
	idx := m.structuralIndices.GetKCoreIndex()
	if idx == nil {
		return entityIDs
	}
	return idx.FilterByMinCore(entityIDs, minCore)
}

// PruneByPivotDistance filters candidates to those potentially reachable from source.
// Returns unchanged input if pivot index is not available.
func (m *Manager) PruneByPivotDistance(source string, candidates []string, maxHops int) []string {
	if m.structuralIndices == nil || maxHops <= 0 {
		return candidates
	}
	idx := m.structuralIndices.GetPivotIndex()
	if idx == nil {
		return candidates
	}

	// Filter candidates using pivot distance bounds
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == source {
			result = append(result, candidate)
			continue
		}
		if idx.IsWithinHops(source, candidate, maxHops) {
			result = append(result, candidate)
		}
	}
	return result
}

// GetStructuralIndices returns the structural index holder for updating indices.
// The processor uses this to populate k-core and pivot indices after computation.
// Returns nil if structural indexing is not configured.
func (m *Manager) GetStructuralIndices() *StructuralIndexHolder {
	return m.structuralIndices
}
