package slim

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SessionState represents the state of a SLIM group session.
type SessionState string

const (
	// SessionStateJoining indicates the session is being established.
	SessionStateJoining SessionState = "joining"

	// SessionStateActive indicates the session is active and ready for messages.
	SessionStateActive SessionState = "active"

	// SessionStateRekeying indicates the session is ratcheting keys.
	SessionStateRekeying SessionState = "rekeying"

	// SessionStateLeaving indicates the session is being terminated.
	SessionStateLeaving SessionState = "leaving"

	// SessionStateError indicates the session encountered an error.
	SessionStateError SessionState = "error"
)

// GroupSession represents an active SLIM group session.
type GroupSession struct {
	// GroupID is the DID-based group identifier.
	GroupID string `json:"group_id"`

	// State is the current session state.
	State SessionState `json:"state"`

	// JoinedAt is when the session was established.
	JoinedAt time.Time `json:"joined_at"`

	// LastActive is when the last message was sent/received.
	LastActive time.Time `json:"last_active"`

	// LastKeyRatchet is when keys were last ratcheted.
	LastKeyRatchet time.Time `json:"last_key_ratchet"`

	// MemberCount is the number of members in the group.
	MemberCount int `json:"member_count"`

	// ErrorMessage contains error details if state is error.
	ErrorMessage string `json:"error_message,omitempty"`
}

// SessionManager manages SLIM MLS group sessions.
// This is a stub implementation - full MLS integration requires the SLIM SDK.
type SessionManager struct {
	config Config
	logger *slog.Logger

	// Active sessions
	sessions map[string]*GroupSession // groupID -> session
	mu       sync.RWMutex

	// Key ratchet management
	ratchetStop chan struct{}
	ratchetWg   sync.WaitGroup

	// SLIM client (stub - would be actual SLIM SDK client)
	client Client
}

// Client defines the interface for SLIM protocol operations.
// This is a stub interface - implementation requires the SLIM SDK.
type Client interface {
	// Connect establishes connection to the SLIM service.
	Connect(ctx context.Context) error

	// Disconnect closes the connection to the SLIM service.
	Disconnect(ctx context.Context) error

	// JoinGroup joins a SLIM group.
	JoinGroup(ctx context.Context, groupID string) error

	// LeaveGroup leaves a SLIM group.
	LeaveGroup(ctx context.Context, groupID string) error

	// SendMessage sends a message to a group.
	SendMessage(ctx context.Context, groupID string, message []byte) error

	// ReceiveMessages returns a channel for receiving messages.
	ReceiveMessages() <-chan *Message

	// RatchetKeys performs key ratcheting for a group.
	RatchetKeys(ctx context.Context, groupID string) error

	// GetGroupMembers returns the members of a group.
	GetGroupMembers(ctx context.Context, groupID string) ([]string, error)
}

// Message represents a message received from SLIM.
type Message struct {
	// GroupID is the group the message was received from.
	GroupID string `json:"group_id"`

	// SenderDID is the DID of the message sender.
	SenderDID string `json:"sender_did"`

	// Content is the decrypted message content.
	Content []byte `json:"content"`

	// Timestamp is when the message was sent.
	Timestamp time.Time `json:"timestamp"`

	// MessageID is a unique identifier for the message.
	MessageID string `json:"message_id"`
}

// NewSessionManager creates a new SLIM session manager.
func NewSessionManager(config Config, client Client, logger *slog.Logger) *SessionManager {
	return &SessionManager{
		config:   config,
		logger:   logger,
		sessions: make(map[string]*GroupSession),
		client:   client,
	}
}

// Start begins the session manager and key ratchet loop.
func (sm *SessionManager) Start(ctx context.Context) error {
	// Connect to SLIM service
	if sm.client != nil {
		if err := sm.client.Connect(ctx); err != nil {
			return fmt.Errorf("connect to SLIM: %w", err)
		}
	}

	// Join configured groups
	for _, groupID := range sm.config.GroupIDs {
		if err := sm.JoinGroup(ctx, groupID); err != nil {
			sm.logger.Warn("Failed to join SLIM group",
				slog.String("group_id", groupID),
				slog.Any("error", err))
		}
	}

	// Start key ratchet loop
	sm.ratchetStop = make(chan struct{})
	sm.ratchetWg.Add(1)
	go sm.keyRatchetLoop(ctx)

	return nil
}

// Stop gracefully stops the session manager.
func (sm *SessionManager) Stop(ctx context.Context) error {
	// Stop key ratchet loop
	if sm.ratchetStop != nil {
		close(sm.ratchetStop)
		sm.ratchetWg.Wait()
	}

	// Leave all groups
	sm.mu.RLock()
	groupIDs := make([]string, 0, len(sm.sessions))
	for groupID := range sm.sessions {
		groupIDs = append(groupIDs, groupID)
	}
	sm.mu.RUnlock()

	for _, groupID := range groupIDs {
		if err := sm.LeaveGroup(ctx, groupID); err != nil {
			sm.logger.Warn("Failed to leave SLIM group",
				slog.String("group_id", groupID),
				slog.Any("error", err))
		}
	}

	// Disconnect from SLIM service
	if sm.client != nil {
		if err := sm.client.Disconnect(ctx); err != nil {
			sm.logger.Warn("Failed to disconnect from SLIM", slog.Any("error", err))
		}
	}

	return nil
}

// JoinGroup joins a SLIM group and creates a session.
func (sm *SessionManager) JoinGroup(ctx context.Context, groupID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if already in group
	if _, ok := sm.sessions[groupID]; ok {
		return nil // Already joined
	}

	// Create session in joining state
	session := &GroupSession{
		GroupID:    groupID,
		State:      SessionStateJoining,
		JoinedAt:   time.Now(),
		LastActive: time.Now(),
	}
	sm.sessions[groupID] = session

	// Join via SLIM client
	if sm.client != nil {
		if err := sm.client.JoinGroup(ctx, groupID); err != nil {
			session.State = SessionStateError
			session.ErrorMessage = err.Error()
			return fmt.Errorf("join group %s: %w", groupID, err)
		}

		// Get member count
		members, err := sm.client.GetGroupMembers(ctx, groupID)
		if err == nil {
			session.MemberCount = len(members)
		}
	}

	session.State = SessionStateActive
	session.LastKeyRatchet = time.Now()

	sm.logger.Info("Joined SLIM group",
		slog.String("group_id", groupID),
		slog.Int("member_count", session.MemberCount))

	return nil
}

// LeaveGroup leaves a SLIM group and removes the session.
func (sm *SessionManager) LeaveGroup(ctx context.Context, groupID string) error {
	sm.mu.Lock()
	session, ok := sm.sessions[groupID]
	if !ok {
		sm.mu.Unlock()
		return nil // Not in group
	}

	session.State = SessionStateLeaving
	sm.mu.Unlock()

	// Leave via SLIM client
	if sm.client != nil {
		if err := sm.client.LeaveGroup(ctx, groupID); err != nil {
			return fmt.Errorf("leave group %s: %w", groupID, err)
		}
	}

	sm.mu.Lock()
	delete(sm.sessions, groupID)
	sm.mu.Unlock()

	sm.logger.Info("Left SLIM group", slog.String("group_id", groupID))

	return nil
}

// GetSession returns a copy of the session for a group.
// Returns nil if the group is not found.
func (sm *SessionManager) GetSession(groupID string) *GroupSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[groupID]
	if !ok {
		return nil
	}
	// Return a copy to avoid race conditions
	sessionCopy := *session
	return &sessionCopy
}

// ListSessions returns copies of all active sessions.
func (sm *SessionManager) ListSessions() []*GroupSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*GroupSession, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		// Return copies to avoid race conditions
		sessionCopy := *session
		sessions = append(sessions, &sessionCopy)
	}
	return sessions
}

// SendMessage sends a message to a SLIM group.
func (sm *SessionManager) SendMessage(ctx context.Context, groupID string, content []byte) error {
	// Validate session state under lock
	sm.mu.RLock()
	session, ok := sm.sessions[groupID]
	if !ok {
		sm.mu.RUnlock()
		return fmt.Errorf("not a member of group %s", groupID)
	}
	state := session.State
	sm.mu.RUnlock()

	if state != SessionStateActive {
		return fmt.Errorf("group %s is not active (state: %s)", groupID, state)
	}

	if sm.client != nil {
		if err := sm.client.SendMessage(ctx, groupID, content); err != nil {
			return fmt.Errorf("send message to %s: %w", groupID, err)
		}
	}

	// Re-validate session exists before updating activity
	sm.mu.Lock()
	if session, ok := sm.sessions[groupID]; ok {
		session.LastActive = time.Now()
	}
	sm.mu.Unlock()

	return nil
}

// UpdateActivity updates the last activity time for a session.
func (sm *SessionManager) UpdateActivity(groupID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, ok := sm.sessions[groupID]; ok {
		session.LastActive = time.Now()
	}
}

// keyRatchetLoop periodically ratchets keys for active sessions.
func (sm *SessionManager) keyRatchetLoop(ctx context.Context) {
	defer sm.ratchetWg.Done()

	ticker := time.NewTicker(sm.config.GetKeyRatchetInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sm.ratchetStop:
			return
		case <-ticker.C:
			sm.ratchetAllSessions(ctx)
		}
	}
}

// ratchetAllSessions ratchets keys for all active sessions.
func (sm *SessionManager) ratchetAllSessions(ctx context.Context) {
	sm.mu.RLock()
	sessions := make([]*GroupSession, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		if session.State == SessionStateActive {
			sessions = append(sessions, session)
		}
	}
	sm.mu.RUnlock()

	for _, session := range sessions {
		if err := sm.ratchetSession(ctx, session.GroupID); err != nil {
			sm.logger.Warn("Failed to ratchet keys",
				slog.String("group_id", session.GroupID),
				slog.Any("error", err))
		}
	}
}

// ratchetSession ratchets keys for a single session.
func (sm *SessionManager) ratchetSession(ctx context.Context, groupID string) error {
	sm.mu.Lock()
	session, ok := sm.sessions[groupID]
	if !ok || session.State != SessionStateActive {
		sm.mu.Unlock()
		return nil
	}
	session.State = SessionStateRekeying
	sm.mu.Unlock()

	var ratchetErr error
	if sm.client != nil {
		ratchetErr = sm.client.RatchetKeys(ctx, groupID)
	}

	sm.mu.Lock()
	if ratchetErr != nil {
		session.State = SessionStateError
		session.ErrorMessage = ratchetErr.Error()
	} else {
		session.State = SessionStateActive
		session.LastKeyRatchet = time.Now()
	}
	sm.mu.Unlock()

	if ratchetErr == nil {
		sm.logger.Debug("Ratcheted keys", slog.String("group_id", groupID))
	}

	return ratchetErr
}

// ActiveSessionCount returns the number of active sessions.
func (sm *SessionManager) ActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := 0
	for _, session := range sm.sessions {
		if session.State == SessionStateActive {
			count++
		}
	}
	return count
}
