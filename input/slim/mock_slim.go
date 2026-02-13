package slim

import (
	"context"
	"sync"
	"time"
)

// MockSLIMClient is a mock implementation of SLIMClient for testing.
type MockSLIMClient struct {
	mu sync.RWMutex

	// Connection state
	connected bool

	// Joined groups
	groups map[string]bool

	// Message channel
	messageChan chan *Message
	chanClosed  bool

	// Sent messages
	SentMessages []SentMessage

	// Configurable errors
	ConnectErr     error
	DisconnectErr  error
	JoinGroupErr   error
	LeaveGroupErr  error
	SendMessageErr error
	RatchetKeysErr error
	GetMembersErr  error

	// Configurable members
	GroupMembers map[string][]string

	// Call tracking
	connectCalls    int
	disconnectCalls int
	joinGroupCalls  int
	leaveGroupCalls int
	sendCalls       int
	ratchetCalls    int
}

// SentMessage represents a message that was sent through the mock client.
type SentMessage struct {
	GroupID string
	Content []byte
	SentAt  time.Time
}

// NewMockSLIMClient creates a new mock SLIM client.
func NewMockSLIMClient() *MockSLIMClient {
	return &MockSLIMClient{
		groups:       make(map[string]bool),
		messageChan:  make(chan *Message, 100),
		GroupMembers: make(map[string][]string),
	}
}

// Connect implements SLIMClient.Connect.
func (m *MockSLIMClient) Connect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connectCalls++

	if m.ConnectErr != nil {
		return m.ConnectErr
	}

	m.connected = true
	return nil
}

// Disconnect implements SLIMClient.Disconnect.
func (m *MockSLIMClient) Disconnect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.disconnectCalls++

	if m.DisconnectErr != nil {
		return m.DisconnectErr
	}

	m.connected = false
	return nil
}

// JoinGroup implements SLIMClient.JoinGroup.
func (m *MockSLIMClient) JoinGroup(_ context.Context, groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.joinGroupCalls++

	if m.JoinGroupErr != nil {
		return m.JoinGroupErr
	}

	m.groups[groupID] = true
	return nil
}

// LeaveGroup implements SLIMClient.LeaveGroup.
func (m *MockSLIMClient) LeaveGroup(_ context.Context, groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.leaveGroupCalls++

	if m.LeaveGroupErr != nil {
		return m.LeaveGroupErr
	}

	delete(m.groups, groupID)
	return nil
}

// SendMessage implements SLIMClient.SendMessage.
func (m *MockSLIMClient) SendMessage(_ context.Context, groupID string, message []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sendCalls++

	if m.SendMessageErr != nil {
		return m.SendMessageErr
	}

	m.SentMessages = append(m.SentMessages, SentMessage{
		GroupID: groupID,
		Content: message,
		SentAt:  time.Now(),
	})

	return nil
}

// ReceiveMessages implements SLIMClient.ReceiveMessages.
func (m *MockSLIMClient) ReceiveMessages() <-chan *Message {
	return m.messageChan
}

// RatchetKeys implements SLIMClient.RatchetKeys.
func (m *MockSLIMClient) RatchetKeys(_ context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ratchetCalls++

	if m.RatchetKeysErr != nil {
		return m.RatchetKeysErr
	}

	return nil
}

// GetGroupMembers implements SLIMClient.GetGroupMembers.
func (m *MockSLIMClient) GetGroupMembers(_ context.Context, groupID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.GetMembersErr != nil {
		return nil, m.GetMembersErr
	}

	if members, ok := m.GroupMembers[groupID]; ok {
		return members, nil
	}

	// Return default members
	return []string{"did:key:mock-member-1", "did:key:mock-member-2"}, nil
}

// SimulateMessage simulates receiving a message from SLIM.
func (m *MockSLIMClient) SimulateMessage(msg *Message) {
	m.messageChan <- msg
}

// GetSentMessages returns all messages sent through the client.
func (m *MockSLIMClient) GetSentMessages() []SentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SentMessage, len(m.SentMessages))
	copy(result, m.SentMessages)
	return result
}

// IsConnected returns whether the client is connected.
func (m *MockSLIMClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// IsInGroup returns whether the client has joined the specified group.
func (m *MockSLIMClient) IsInGroup(groupID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.groups[groupID]
}

// GetJoinedGroups returns all joined groups.
func (m *MockSLIMClient) GetJoinedGroups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	groups := make([]string, 0, len(m.groups))
	for groupID := range m.groups {
		groups = append(groups, groupID)
	}
	return groups
}

// CallCounts returns the number of times each method was called.
func (m *MockSLIMClient) CallCounts() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]int{
		"Connect":    m.connectCalls,
		"Disconnect": m.disconnectCalls,
		"JoinGroup":  m.joinGroupCalls,
		"LeaveGroup": m.leaveGroupCalls,
		"SendMsg":    m.sendCalls,
		"Ratchet":    m.ratchetCalls,
	}
}

// Reset resets the mock client state.
func (m *MockSLIMClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connected = false
	m.groups = make(map[string]bool)
	m.SentMessages = nil
	m.connectCalls = 0
	m.disconnectCalls = 0
	m.joinGroupCalls = 0
	m.leaveGroupCalls = 0
	m.sendCalls = 0
	m.ratchetCalls = 0

	// Clear and recreate message channel (only close if not already closed)
	if !m.chanClosed {
		close(m.messageChan)
	}
	m.messageChan = make(chan *Message, 100)
	m.chanClosed = false
}

// Close closes the mock client's channels.
func (m *MockSLIMClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.chanClosed {
		close(m.messageChan)
		m.chanClosed = true
	}
}
