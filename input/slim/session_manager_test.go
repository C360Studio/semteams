package slim

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestNewSessionManager(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	sm := NewSessionManager(cfg, nil, logger)

	if sm == nil {
		t.Fatal("expected session manager, got nil")
	}

	if sm.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestSessionManagerStartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GroupIDs = []string{"test-group-1", "test-group-2"}
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Start
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify connected
	if !mockClient.IsConnected() {
		t.Error("expected client to be connected")
	}

	// Verify groups joined
	for _, groupID := range cfg.GroupIDs {
		if !mockClient.IsInGroup(groupID) {
			t.Errorf("expected to be in group %q", groupID)
		}
	}

	// Give key ratchet loop time to start
	time.Sleep(10 * time.Millisecond)

	// Stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = sm.Stop(stopCtx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify disconnected
	if mockClient.IsConnected() {
		t.Error("expected client to be disconnected")
	}
}

func TestSessionManagerJoinGroup(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	mockClient.GroupMembers["test-group"] = []string{"member1", "member2", "member3"}

	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Join group
	err := sm.JoinGroup(ctx, "test-group")
	if err != nil {
		t.Fatalf("JoinGroup failed: %v", err)
	}

	// Verify session exists
	session := sm.GetSession("test-group")
	if session == nil {
		t.Fatal("expected session, got nil")
	}

	if session.State != SessionStateActive {
		t.Errorf("expected state Active, got %v", session.State)
	}

	if session.MemberCount != 3 {
		t.Errorf("expected member count 3, got %d", session.MemberCount)
	}

	// Join same group again should be no-op
	err = sm.JoinGroup(ctx, "test-group")
	if err != nil {
		t.Errorf("joining same group should not error: %v", err)
	}
}

func TestSessionManagerJoinGroupError(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	mockClient.JoinGroupErr = errors.New("join failed")

	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	err := sm.JoinGroup(ctx, "test-group")
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Verify session is in error state
	session := sm.GetSession("test-group")
	if session == nil {
		t.Fatal("expected session even with error")
	}

	if session.State != SessionStateError {
		t.Errorf("expected state Error, got %v", session.State)
	}
}

func TestSessionManagerLeaveGroup(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Join first
	_ = sm.JoinGroup(ctx, "test-group")

	// Leave
	err := sm.LeaveGroup(ctx, "test-group")
	if err != nil {
		t.Fatalf("LeaveGroup failed: %v", err)
	}

	// Verify session removed
	session := sm.GetSession("test-group")
	if session != nil {
		t.Error("expected session to be removed")
	}

	// Leave group not joined should be no-op
	err = sm.LeaveGroup(ctx, "not-joined")
	if err != nil {
		t.Errorf("leaving non-joined group should not error: %v", err)
	}
}

func TestSessionManagerListSessions(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Initially empty
	sessions := sm.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}

	// Join some groups
	_ = sm.JoinGroup(ctx, "group-1")
	_ = sm.JoinGroup(ctx, "group-2")

	sessions = sm.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionManagerSendMessage(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Try to send without joining
	err := sm.SendMessage(ctx, "test-group", []byte("hello"))
	if err == nil {
		t.Error("expected error sending to non-joined group")
	}

	// Join group
	_ = sm.JoinGroup(ctx, "test-group")

	// Send message
	err = sm.SendMessage(ctx, "test-group", []byte("hello"))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify message was sent
	sent := mockClient.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}

	if string(sent[0].Content) != "hello" {
		t.Errorf("expected content 'hello', got %q", string(sent[0].Content))
	}
}

func TestSessionManagerSendMessageInactiveSession(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	mockClient.JoinGroupErr = errors.New("join failed")

	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Join will fail, putting session in error state
	_ = sm.JoinGroup(ctx, "test-group")

	// Try to send to error state session
	err := sm.SendMessage(ctx, "test-group", []byte("hello"))
	if err == nil {
		t.Error("expected error sending to inactive session")
	}
}

func TestSessionManagerUpdateActivity(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	_ = sm.JoinGroup(ctx, "test-group")

	session := sm.GetSession("test-group")
	originalTime := session.LastActive

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Update activity
	sm.UpdateActivity("test-group")

	session = sm.GetSession("test-group")
	if !session.LastActive.After(originalTime) {
		t.Error("expected LastActive to be updated")
	}
}

func TestSessionManagerActiveSessionCount(t *testing.T) {
	cfg := DefaultConfig()
	logger := slog.Default()

	mockClient := NewMockSLIMClient()
	sm := NewSessionManager(cfg, mockClient, logger)

	ctx := context.Background()

	// Initially 0
	count := sm.ActiveSessionCount()
	if count != 0 {
		t.Errorf("expected 0 active sessions, got %d", count)
	}

	// Join active group
	_ = sm.JoinGroup(ctx, "group-1")
	count = sm.ActiveSessionCount()
	if count != 1 {
		t.Errorf("expected 1 active session, got %d", count)
	}

	// Join another
	_ = sm.JoinGroup(ctx, "group-2")
	count = sm.ActiveSessionCount()
	if count != 2 {
		t.Errorf("expected 2 active sessions, got %d", count)
	}

	// Error session doesn't count
	mockClient.JoinGroupErr = errors.New("fail")
	_ = sm.JoinGroup(ctx, "group-3")
	count = sm.ActiveSessionCount()
	if count != 2 {
		t.Errorf("expected 2 active sessions (error session excluded), got %d", count)
	}
}

func TestSessionManagerWithNilClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GroupIDs = []string{"test-group"}
	logger := slog.Default()

	// Create with nil client
	sm := NewSessionManager(cfg, nil, logger)

	ctx := context.Background()

	// Start should work with nil client
	err := sm.Start(ctx)
	if err != nil {
		t.Fatalf("Start should work with nil client: %v", err)
	}

	// Session should be created and active
	session := sm.GetSession("test-group")
	if session == nil {
		t.Fatal("expected session to be created")
	}

	if session.State != SessionStateActive {
		t.Errorf("expected state Active, got %v", session.State)
	}

	// Send message with nil client
	err = sm.SendMessage(ctx, "test-group", []byte("hello"))
	if err != nil {
		t.Errorf("SendMessage should work with nil client: %v", err)
	}

	// Stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = sm.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop should work with nil client: %v", err)
	}
}

func TestSessionStates(t *testing.T) {
	// Verify state constants
	if SessionStateJoining != "joining" {
		t.Errorf("unexpected state value: %v", SessionStateJoining)
	}
	if SessionStateActive != "active" {
		t.Errorf("unexpected state value: %v", SessionStateActive)
	}
	if SessionStateRekeying != "rekeying" {
		t.Errorf("unexpected state value: %v", SessionStateRekeying)
	}
	if SessionStateLeaving != "leaving" {
		t.Errorf("unexpected state value: %v", SessionStateLeaving)
	}
	if SessionStateError != "error" {
		t.Errorf("unexpected state value: %v", SessionStateError)
	}
}
