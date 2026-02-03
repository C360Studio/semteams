package agenticdispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// LoopInfo contains information about an active loop
type LoopInfo struct {
	LoopID        string    `json:"loop_id"`
	TaskID        string    `json:"task_id"`
	UserID        string    `json:"user_id"`
	ChannelType   string    `json:"channel_type"`
	ChannelID     string    `json:"channel_id"`
	State         string    `json:"state"`
	Iterations    int       `json:"iterations"`
	MaxIterations int       `json:"max_iterations"`
	CreatedAt     time.Time `json:"created_at"`
}

// LoopTracker tracks active loops per user and channel
type LoopTracker struct {
	mu           sync.RWMutex
	userLoops    map[string]string    // user_id -> most recent loop_id
	channelLoops map[string]string    // channel_id -> most recent loop_id
	loops        map[string]*LoopInfo // loop_id -> LoopInfo
}

// NewLoopTracker creates a new LoopTracker
func NewLoopTracker() *LoopTracker {
	return &LoopTracker{
		userLoops:    make(map[string]string),
		channelLoops: make(map[string]string),
		loops:        make(map[string]*LoopInfo),
	}
}

// Track adds or updates a loop in the tracker
func (t *LoopTracker) Track(info *LoopInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.loops[info.LoopID] = info
	t.userLoops[info.UserID] = info.LoopID
	if info.ChannelID != "" {
		t.channelLoops[info.ChannelID] = info.LoopID
	}
}

// Get retrieves loop info by ID
func (t *LoopTracker) Get(loopID string) *LoopInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.loops[loopID]
}

// GetActiveLoop returns the most recent active loop for a user/channel
func (t *LoopTracker) GetActiveLoop(userID, channelID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Prefer channel-specific loop
	if channelID != "" {
		if loopID, ok := t.channelLoops[channelID]; ok {
			if info := t.loops[loopID]; info != nil && !isTerminalState(info.State) {
				return loopID
			}
		}
	}

	// Fall back to user's most recent loop
	if loopID, ok := t.userLoops[userID]; ok {
		if info := t.loops[loopID]; info != nil && !isTerminalState(info.State) {
			return loopID
		}
	}

	return ""
}

// UpdateState updates the state of a loop
func (t *LoopTracker) UpdateState(loopID, state string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if info, ok := t.loops[loopID]; ok {
		info.State = state
	}
}

// UpdateIterations updates the iteration count of a loop
func (t *LoopTracker) UpdateIterations(loopID string, iterations int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if info, ok := t.loops[loopID]; ok {
		info.Iterations = iterations
	}
}

// Remove removes a loop from the tracker
func (t *LoopTracker) Remove(loopID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.loops[loopID]
	if !ok {
		return
	}

	// Clean up user mapping if this was their most recent loop
	if t.userLoops[info.UserID] == loopID {
		delete(t.userLoops, info.UserID)
	}

	// Clean up channel mapping if this was the channel's most recent loop
	if info.ChannelID != "" && t.channelLoops[info.ChannelID] == loopID {
		delete(t.channelLoops, info.ChannelID)
	}

	delete(t.loops, loopID)
}

// GetUserLoops returns all loops for a specific user
func (t *LoopTracker) GetUserLoops(userID string) []*LoopInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*LoopInfo
	for _, info := range t.loops {
		if info.UserID == userID {
			result = append(result, info)
		}
	}
	return result
}

// GetAllLoops returns all tracked loops
func (t *LoopTracker) GetAllLoops() []*LoopInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*LoopInfo, 0, len(t.loops))
	for _, info := range t.loops {
		result = append(result, info)
	}
	return result
}

// Count returns the number of tracked loops
func (t *LoopTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.loops)
}

// isTerminalState checks if a state is terminal
func isTerminalState(state string) bool {
	switch state {
	case "complete", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// SignalMessage represents a control signal sent to a loop.
type SignalMessage struct {
	LoopID    string    `json:"loop_id"`
	Type      string    `json:"type"`   // pause, resume, cancel
	Reason    string    `json:"reason"` // optional reason
	Timestamp time.Time `json:"timestamp"`
}

// SendSignal publishes a control signal to a loop via NATS.
func (t *LoopTracker) SendSignal(ctx context.Context, nc *natsclient.Client, loopID, signalType, reason string) error {
	if nc == nil {
		return ErrNATSClientNil
	}

	signal := SignalMessage{
		LoopID:    loopID,
		Type:      signalType,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(signal)
	if err != nil {
		return err
	}

	subject := "agent.signal." + loopID
	return nc.PublishToStream(ctx, subject, data)
}

// ErrNATSClientNil is returned when NATS client is nil.
var ErrNATSClientNil = fmt.Errorf("NATS client is nil")
