// Package rule - Tests for Rule Actions (TDD - RED Phase)
package rule

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360/semstreams/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T039: Test Action AddTriple - creates a relationship triple
func TestAction_AddTriple(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name       string
		action     Action
		entityID   string
		relatedID  string
		wantTriple message.Triple
		wantErr    bool
	}{
		{
			name: "create proximity relationship",
			action: Action{
				Type:      ActionTypeAddTriple,
				Predicate: "proximity.near",
				Object:    "$related.id",
				TTL:       "5m",
			},
			entityID:  "c360.platform1.robotics.mav1.drone.001",
			relatedID: "c360.platform1.robotics.mav1.drone.002",
			wantTriple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
			},
			wantErr: false,
		},
		{
			name: "create fleet membership",
			action: Action{
				Type:      ActionTypeAddTriple,
				Predicate: "fleet.member_of",
				Object:    "fleet.alpha",
			},
			entityID:  "c360.platform1.robotics.mav1.drone.003",
			relatedID: "",
			wantTriple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.003",
				Predicate: "fleet.member_of",
				Object:    "fleet.alpha",
			},
			wantErr: false,
		},
		{
			name: "missing predicate should fail",
			action: Action{
				Type:   ActionTypeAddTriple,
				Object: "test.value",
			},
			entityID: "c360.platform1.robotics.mav1.drone.004",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create action executor (will fail - type doesn't exist yet)
			executor := &ActionExecutor{}

			// Execute action
			triple, err := executor.ExecuteAddTriple(ctx, tt.action, tt.entityID, tt.relatedID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTriple.Subject, triple.Subject)
				assert.Equal(t, tt.wantTriple.Predicate, triple.Predicate)
				assert.Equal(t, tt.wantTriple.Object, triple.Object)

				// Verify TTL is set if specified
				if tt.action.TTL != "" {
					assert.NotNil(t, triple.ExpiresAt, "Triple should have expiration time")
					assert.True(t, triple.ExpiresAt.After(time.Now()), "Expiration should be in the future")
				}
			}
		})
	}
}

// T040: Test Action RemoveTriple - removes a relationship triple
func TestAction_RemoveTriple(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		action    Action
		entityID  string
		relatedID string
		wantErr   bool
	}{
		{
			name: "remove proximity relationship",
			action: Action{
				Type:      ActionTypeRemoveTriple,
				Predicate: "proximity.near",
				Object:    "$related.id",
			},
			entityID:  "c360.platform1.robotics.mav1.drone.001",
			relatedID: "c360.platform1.robotics.mav1.drone.002",
			wantErr:   false,
		},
		{
			name: "remove static relationship",
			action: Action{
				Type:      ActionTypeRemoveTriple,
				Predicate: "fleet.member_of",
				Object:    "fleet.alpha",
			},
			entityID: "c360.platform1.robotics.mav1.drone.003",
			wantErr:  false,
		},
		{
			name: "missing predicate should fail",
			action: Action{
				Type:   ActionTypeRemoveTriple,
				Object: "test.value",
			},
			entityID: "c360.platform1.robotics.mav1.drone.004",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &ActionExecutor{}

			err := executor.ExecuteRemoveTriple(ctx, tt.action, tt.entityID, tt.relatedID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// T040a: Test Action struct
func TestAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action Action
		valid  bool
	}{
		{
			name: "valid add_triple action",
			action: Action{
				Type:      ActionTypeAddTriple,
				Predicate: "proximity.near",
				Object:    "$related.id",
				TTL:       "5m",
			},
			valid: true,
		},
		{
			name: "valid remove_triple action",
			action: Action{
				Type:      ActionTypeRemoveTriple,
				Predicate: "proximity.near",
				Object:    "$related.id",
			},
			valid: true,
		},
		{
			name: "valid publish action",
			action: Action{
				Type:    ActionTypePublish,
				Subject: "alerts.low-battery",
				Properties: map[string]any{
					"severity": "high",
					"message":  "Battery critically low",
				},
			},
			valid: true,
		},
		{
			name: "valid update_triple action",
			action: Action{
				Type:      ActionTypeUpdateTriple,
				Predicate: "proximity.near",
				Object:    "$related.id",
				Properties: map[string]any{
					"distance": 50.0,
				},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify action type is one of the valid constants
			validTypes := []string{
				ActionTypePublish,
				ActionTypeAddTriple,
				ActionTypeRemoveTriple,
				ActionTypeUpdateTriple,
			}

			if tt.valid {
				assert.Contains(t, validTypes, tt.action.Type)
			}
		})
	}
}

// T040b: Test Action constants
func TestActionConstants(t *testing.T) {
	t.Parallel()

	// Verify constants exist
	assert.Equal(t, "publish", ActionTypePublish)
	assert.Equal(t, "add_triple", ActionTypeAddTriple)
	assert.Equal(t, "remove_triple", ActionTypeRemoveTriple)
	assert.Equal(t, "update_triple", ActionTypeUpdateTriple)
}

// T040c: Test Action TTL parsing
func TestAction_TTLParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ttl         string
		wantError   bool
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{
			name:        "5 minutes",
			ttl:         "5m",
			wantError:   false,
			minDuration: 4 * time.Minute,
			maxDuration: 6 * time.Minute,
		},
		{
			name:        "1 hour",
			ttl:         "1h",
			wantError:   false,
			minDuration: 55 * time.Minute,
			maxDuration: 65 * time.Minute,
		},
		{
			name:        "30 seconds",
			ttl:         "30s",
			wantError:   false,
			minDuration: 25 * time.Second,
			maxDuration: 35 * time.Second,
		},
		{
			name:      "invalid format",
			ttl:       "invalid",
			wantError: true,
		},
		{
			name:      "negative duration",
			ttl:       "-5m",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := Action{
				Type:      ActionTypeAddTriple,
				Predicate: "test.predicate",
				Object:    "test.value",
				TTL:       tt.ttl,
			}

			// Parse TTL (function doesn't exist yet)
			duration, err := action.ParseTTL()

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, duration >= tt.minDuration, "Duration should be >= %v", tt.minDuration)
				assert.True(t, duration <= tt.maxDuration, "Duration should be <= %v", tt.maxDuration)
			}
		})
	}
}

// T040d: Test variable substitution in actions
func TestAction_VariableSubstitution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		template  string
		entityID  string
		relatedID string
		want      string
	}{
		{
			name:      "substitute related.id",
			template:  "$related.id",
			entityID:  "c360.platform1.robotics.mav1.drone.001",
			relatedID: "c360.platform1.robotics.mav1.drone.002",
			want:      "c360.platform1.robotics.mav1.drone.002",
		},
		{
			name:      "substitute entity.id",
			template:  "$entity.id",
			entityID:  "c360.platform1.robotics.mav1.drone.001",
			relatedID: "",
			want:      "c360.platform1.robotics.mav1.drone.001",
		},
		{
			name:      "no substitution",
			template:  "static.value",
			entityID:  "c360.platform1.robotics.mav1.drone.001",
			relatedID: "",
			want:      "static.value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Function doesn't exist yet
			result := substituteVariables(tt.template, tt.entityID, tt.relatedID)
			assert.Equal(t, tt.want, result)
		})
	}
}

// T040e: Test action execution context
func TestActionExecutor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() *ActionExecutor
		wantErr bool
	}{
		{
			name: "valid executor",
			setup: func() *ActionExecutor {
				return &ActionExecutor{}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := tt.setup()
			assert.NotNil(t, executor)
		})
	}
}

// mockPublisher implements Publisher interface for testing
type mockPublisher struct {
	published []publishedMessage
	err       error
}

type publishedMessage struct {
	subject string
	data    []byte
}

func (m *mockPublisher) Publish(_ context.Context, subject string, data []byte) error {
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, publishedMessage{subject: subject, data: data})
	return nil
}

// T041: Test Action Publish - sends message to NATS subject
func TestAction_Publish(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		action      Action
		entityID    string
		relatedID   string
		wantSubject string
		wantErr     bool
		errMsg      string
	}{
		{
			name: "publish to static subject",
			action: Action{
				Type:    ActionTypePublish,
				Subject: "alerts.battery.low",
				Properties: map[string]any{
					"severity": "critical",
				},
			},
			entityID:    "c360.platform.robotics.mav1.drone.001",
			relatedID:   "",
			wantSubject: "alerts.battery.low",
			wantErr:     false,
		},
		{
			name: "publish with entity variable substitution",
			action: Action{
				Type:    ActionTypePublish,
				Subject: "events.$entity.id",
			},
			entityID:    "c360.platform.robotics.mav1.drone.001",
			relatedID:   "",
			wantSubject: "events.c360.platform.robotics.mav1.drone.001",
			wantErr:     false,
		},
		{
			name: "publish with related variable substitution",
			action: Action{
				Type:    ActionTypePublish,
				Subject: "proximity.$related.id",
			},
			entityID:    "c360.platform.robotics.mav1.drone.001",
			relatedID:   "c360.platform.robotics.mav1.drone.002",
			wantSubject: "proximity.c360.platform.robotics.mav1.drone.002",
			wantErr:     false,
		},
		{
			name: "missing subject should fail",
			action: Action{
				Type: ActionTypePublish,
			},
			entityID: "c360.platform.robotics.mav1.drone.001",
			wantErr:  true,
			errMsg:   "subject is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPublisher{}
			executor := NewActionExecutorFull(nil, nil, mock)

			err := executor.Execute(ctx, tt.action, tt.entityID, tt.relatedID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.Len(t, mock.published, 1, "should have published one message")
				assert.Equal(t, tt.wantSubject, mock.published[0].subject)
			}
		})
	}
}

// T042: Test Publish action payload format
func TestAction_Publish_PayloadFormat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := &mockPublisher{}
	executor := NewActionExecutorFull(nil, nil, mock)

	action := Action{
		Type:    ActionTypePublish,
		Subject: "test.subject",
		Properties: map[string]any{
			"custom_field": "custom_value",
			"priority":     1,
		},
	}

	err := executor.Execute(ctx, action, "entity.001", "related.002")
	require.NoError(t, err)
	require.Len(t, mock.published, 1)

	// Parse the published payload
	var payload map[string]any
	err = json.Unmarshal(mock.published[0].data, &payload)
	require.NoError(t, err)

	// Verify required fields
	assert.Equal(t, "entity.001", payload["entity_id"])
	assert.Equal(t, "related.002", payload["related_id"])
	assert.Equal(t, "test.subject", payload["subject"])
	assert.Equal(t, "rule_engine", payload["source"])
	assert.NotEmpty(t, payload["timestamp"])

	// Verify properties are included
	props, ok := payload["properties"].(map[string]any)
	require.True(t, ok, "properties should be a map")
	assert.Equal(t, "custom_value", props["custom_field"])
	assert.Equal(t, float64(1), props["priority"]) // JSON numbers are float64
}

// T043: Test Publish action without publisher (no-op)
func TestAction_Publish_NoPublisher(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	executor := NewActionExecutor(nil) // No publisher configured

	action := Action{
		Type:    ActionTypePublish,
		Subject: "test.subject",
	}

	// Should not error, just log and return
	err := executor.Execute(ctx, action, "entity.001", "")
	require.NoError(t, err)
}

// T044: Test Publish action error handling
func TestAction_Publish_ErrorHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expectedErr := assert.AnError
	mock := &mockPublisher{err: expectedErr}
	executor := NewActionExecutorFull(nil, nil, mock)

	action := Action{
		Type:    ActionTypePublish,
		Subject: "test.subject",
	}

	err := executor.Execute(ctx, action, "entity.001", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish to test.subject")
}

// mockTripleMutator implements TripleMutator interface for testing
type mockTripleMutator struct {
	addedTriples   []message.Triple
	removedTriples []struct {
		subject   string
		predicate string
	}
	addErr    error
	removeErr error
}

func (m *mockTripleMutator) AddTriple(_ context.Context, triple message.Triple) (uint64, error) {
	if m.addErr != nil {
		return 0, m.addErr
	}
	m.addedTriples = append(m.addedTriples, triple)
	return uint64(len(m.addedTriples)), nil
}

func (m *mockTripleMutator) RemoveTriple(_ context.Context, subject, predicate string) (uint64, error) {
	if m.removeErr != nil {
		return 0, m.removeErr
	}
	m.removedTriples = append(m.removedTriples, struct {
		subject   string
		predicate string
	}{subject, predicate})
	return 1, nil
}

// T045: Test Action UpdateTriple - updates a triple (remove + add)
func TestAction_UpdateTriple(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name          string
		action        Action
		entityID      string
		relatedID     string
		wantPredicate string
		wantObject    string
		wantErr       bool
		errMsg        string
	}{
		{
			name: "update status triple",
			action: Action{
				Type:      ActionTypeUpdateTriple,
				Predicate: "status.battery",
				Object:    "low",
			},
			entityID:      "c360.platform.robotics.mav1.drone.001",
			wantPredicate: "status.battery",
			wantObject:    "low",
			wantErr:       false,
		},
		{
			name: "update with variable substitution",
			action: Action{
				Type:      ActionTypeUpdateTriple,
				Predicate: "fleet.membership",
				Object:    "$related.id",
			},
			entityID:      "c360.platform.robotics.mav1.drone.001",
			relatedID:     "c360.platform.fleet.alpha",
			wantPredicate: "fleet.membership",
			wantObject:    "c360.platform.fleet.alpha",
			wantErr:       false,
		},
		{
			name: "update with TTL",
			action: Action{
				Type:      ActionTypeUpdateTriple,
				Predicate: "alert.status",
				Object:    "active",
				TTL:       "5m",
			},
			entityID:      "c360.platform.robotics.mav1.drone.001",
			wantPredicate: "alert.status",
			wantObject:    "active",
			wantErr:       false,
		},
		{
			name: "missing predicate should fail",
			action: Action{
				Type:   ActionTypeUpdateTriple,
				Object: "test.value",
			},
			entityID: "c360.platform.robotics.mav1.drone.001",
			wantErr:  true,
			errMsg:   "predicate is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTripleMutator{}
			executor := NewActionExecutorWithMutator(nil, mock)

			err := executor.Execute(ctx, tt.action, tt.entityID, tt.relatedID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)

				// Verify remove was called
				require.Len(t, mock.removedTriples, 1, "should have removed one triple")
				assert.Equal(t, tt.entityID, mock.removedTriples[0].subject)
				assert.Equal(t, tt.wantPredicate, mock.removedTriples[0].predicate)

				// Verify add was called
				require.Len(t, mock.addedTriples, 1, "should have added one triple")
				assert.Equal(t, tt.entityID, mock.addedTriples[0].Subject)
				assert.Equal(t, tt.wantPredicate, mock.addedTriples[0].Predicate)
				assert.Equal(t, tt.wantObject, mock.addedTriples[0].Object)

				// Verify TTL if specified
				if tt.action.TTL != "" {
					assert.NotNil(t, mock.addedTriples[0].ExpiresAt, "Triple should have expiration")
				}
			}
		})
	}
}

// T046: Test UpdateTriple without mutator (no-op)
func TestAction_UpdateTriple_NoMutator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	executor := NewActionExecutor(nil) // No mutator configured

	action := Action{
		Type:      ActionTypeUpdateTriple,
		Predicate: "test.predicate",
		Object:    "test.value",
	}

	// Should not error, just log and return
	err := executor.Execute(ctx, action, "entity.001", "")
	require.NoError(t, err)
}

// T047: Test UpdateTriple continues even if remove fails (triple may not exist)
func TestAction_UpdateTriple_RemoveFailsContinues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := &mockTripleMutator{
		removeErr: assert.AnError, // Simulate remove failure
	}
	executor := NewActionExecutorWithMutator(nil, mock)

	action := Action{
		Type:      ActionTypeUpdateTriple,
		Predicate: "test.predicate",
		Object:    "test.value",
	}

	// Should still succeed - add should still be called
	err := executor.Execute(ctx, action, "entity.001", "")
	require.NoError(t, err)

	// Add should still have been called
	require.Len(t, mock.addedTriples, 1, "should have added triple even if remove failed")
}

// T048: Test UpdateTriple fails if add fails
func TestAction_UpdateTriple_AddFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := &mockTripleMutator{
		addErr: assert.AnError,
	}
	executor := NewActionExecutorWithMutator(nil, mock)

	action := Action{
		Type:      ActionTypeUpdateTriple,
		Predicate: "test.predicate",
		Object:    "test.value",
	}

	err := executor.Execute(ctx, action, "entity.001", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add updated triple")
}
