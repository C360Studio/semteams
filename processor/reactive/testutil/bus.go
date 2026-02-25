package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Common errors for testutil package.
var (
	ErrNoMessages = errors.New("no messages found")
	ErrTimeout    = errors.New("timeout waiting for message")
)

// PublishedMessage represents a message that was published to the bus.
type PublishedMessage struct {
	Subject   string
	Data      []byte
	Timestamp time.Time
}

// InMemoryBus captures published messages for testing.
type InMemoryBus struct {
	mu       sync.RWMutex
	messages []PublishedMessage
	handlers map[string][]func(context.Context, []byte)
}

// NewInMemoryBus creates a new in-memory message bus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		messages: make([]PublishedMessage, 0),
		handlers: make(map[string][]func(context.Context, []byte)),
	}
}

// Publish stores a message and notifies handlers.
func (b *InMemoryBus) Publish(_ context.Context, subject string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	msg := PublishedMessage{
		Subject:   subject,
		Data:      data,
		Timestamp: time.Now(),
	}
	b.messages = append(b.messages, msg)

	// Notify any registered handlers
	for pattern, handlers := range b.handlers {
		if matchSubjectPattern(subject, pattern) {
			for _, h := range handlers {
				go h(context.Background(), data)
			}
		}
	}

	return nil
}

// Subscribe registers a handler for messages matching the pattern.
func (b *InMemoryBus) Subscribe(pattern string, handler func(context.Context, []byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[pattern] = append(b.handlers[pattern], handler)
}

// Messages returns all published messages.
func (b *InMemoryBus) Messages() []PublishedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]PublishedMessage, len(b.messages))
	copy(result, b.messages)
	return result
}

// MessagesForSubject returns messages matching the subject pattern.
func (b *InMemoryBus) MessagesForSubject(pattern string) []PublishedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []PublishedMessage
	for _, msg := range b.messages {
		if matchSubjectPattern(msg.Subject, pattern) {
			result = append(result, msg)
		}
	}
	return result
}

// LastMessage returns the most recent message, or nil if none.
func (b *InMemoryBus) LastMessage() *PublishedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.messages) == 0 {
		return nil
	}
	msg := b.messages[len(b.messages)-1]
	return &msg
}

// LastMessageForSubject returns the most recent message matching the pattern.
func (b *InMemoryBus) LastMessageForSubject(pattern string) *PublishedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for i := len(b.messages) - 1; i >= 0; i-- {
		if matchSubjectPattern(b.messages[i].Subject, pattern) {
			msg := b.messages[i]
			return &msg
		}
	}
	return nil
}

// Count returns the total number of messages.
func (b *InMemoryBus) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.messages)
}

// CountForSubject returns the number of messages matching the pattern.
func (b *InMemoryBus) CountForSubject(pattern string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, msg := range b.messages {
		if matchSubjectPattern(msg.Subject, pattern) {
			count++
		}
	}
	return count
}

// Clear removes all messages.
func (b *InMemoryBus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = make([]PublishedMessage, 0)
}

// HasMessage checks if any message matches the subject pattern.
func (b *InMemoryBus) HasMessage(pattern string) bool {
	return b.CountForSubject(pattern) > 0
}

// HasMessageWithType checks if any message matches the subject and type.
func (b *InMemoryBus) HasMessageWithType(pattern string, msgType message.Type) bool {
	msgs := b.MessagesForSubject(pattern)
	for _, msg := range msgs {
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(msg.Data, &baseMsg); err != nil {
			continue
		}
		actualType := baseMsg.Type()
		if actualType.Domain == msgType.Domain &&
			actualType.Category == msgType.Category &&
			actualType.Version == msgType.Version {
			return true
		}
	}
	return false
}

// GetPayload unmarshals and returns the payload from the last matching message.
func (b *InMemoryBus) GetPayload(pattern string, v any) error {
	msg := b.LastMessageForSubject(pattern)
	if msg == nil {
		return ErrNoMessages
	}

	var baseMsg message.BaseMessage
	if err := json.Unmarshal(msg.Data, &baseMsg); err != nil {
		return err
	}

	// Get the payload and re-marshal to target type
	payload := baseMsg.Payload()
	if payload == nil {
		return errors.New("message has no payload")
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(payloadData, v)
}

// WaitForMessage waits for a message matching the pattern within the timeout.
func (b *InMemoryBus) WaitForMessage(pattern string, timeout time.Duration) (*PublishedMessage, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if msg := b.LastMessageForSubject(pattern); msg != nil {
			return msg, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, ErrTimeout
}

// WaitForCount waits for at least n messages matching the pattern.
func (b *InMemoryBus) WaitForCount(pattern string, n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b.CountForSubject(pattern) >= n {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ErrTimeout
}

// matchSubjectPattern checks if a subject matches a NATS pattern.
// Supports * (single token) and > (multiple tokens) wildcards.
func matchSubjectPattern(subject, pattern string) bool {
	if pattern == subject {
		return true
	}
	if pattern == ">" {
		return true
	}

	// Simple implementation for common cases
	subjectParts := splitSubject(subject)
	patternParts := splitSubject(pattern)

	for i, pp := range patternParts {
		if pp == ">" {
			return true // Match rest
		}
		if i >= len(subjectParts) {
			return false
		}
		if pp != "*" && pp != subjectParts[i] {
			return false
		}
	}

	return len(subjectParts) == len(patternParts)
}

func splitSubject(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
