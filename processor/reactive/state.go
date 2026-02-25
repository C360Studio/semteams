package reactive

import (
	"reflect"
	"time"
)

// StateAccessor provides a standard interface for accessing the embedded ExecutionState
// from custom workflow state structs. Implementing this interface allows the engine
// to access and modify the base execution state without reflection.
//
// Example implementation:
//
//	type PlanReviewState struct {
//	    reactive.ExecutionState
//	    PlanContent *PlanContent
//	}
//
//	func (s *PlanReviewState) GetExecutionState() *reactive.ExecutionState {
//	    return &s.ExecutionState
//	}
type StateAccessor interface {
	GetExecutionState() *ExecutionState
}

// ExtractExecutionState extracts the ExecutionState from a typed state struct.
// It first checks if the state implements StateAccessor, then falls back to reflection.
// Returns nil if the state is nil or doesn't contain an ExecutionState.
func ExtractExecutionState(state any) *ExecutionState {
	if state == nil {
		return nil
	}

	// Try direct type assertion first
	if es, ok := state.(*ExecutionState); ok {
		return es
	}

	// Try StateAccessor interface
	if accessor, ok := state.(StateAccessor); ok {
		return accessor.GetExecutionState()
	}

	// Fall back to reflection for embedded ExecutionState
	return extractExecutionStateViaReflection(state)
}

// extractExecutionStateViaReflection uses reflection to find an embedded ExecutionState.
func extractExecutionStateViaReflection(state any) *ExecutionState {
	v := reflect.ValueOf(state)

	// Dereference pointer if needed
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	// Must be a struct
	if v.Kind() != reflect.Struct {
		return nil
	}

	// Look for ExecutionState field (embedded or named)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Check if this field is ExecutionState or *ExecutionState
		if fieldType.Type == reflect.TypeOf(ExecutionState{}) {
			if field.CanAddr() {
				return field.Addr().Interface().(*ExecutionState)
			}
		}
		if fieldType.Type == reflect.TypeOf(&ExecutionState{}) {
			if !field.IsNil() {
				return field.Interface().(*ExecutionState)
			}
		}

		// Check for embedded anonymous struct
		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			if es := extractExecutionStateViaReflection(field.Addr().Interface()); es != nil {
				return es
			}
		}
	}

	return nil
}

// SetPhase updates the execution phase and records a timeline entry.
func SetPhase(state any, phase string, ruleID string, triggerMode TriggerMode, triggerInfo string, action string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Phase = phase
	es.UpdatedAt = time.Now()

	// Record timeline entry
	es.Timeline = append(es.Timeline, TimelineEntry{
		Timestamp:   time.Now(),
		RuleID:      ruleID,
		TriggerMode: triggerMode.String(),
		TriggerInfo: triggerInfo,
		Action:      action,
		Phase:       phase,
		Iteration:   es.Iteration,
	})
}

// SetStatus updates the execution status.
func SetStatus(state any, status ExecutionStatus) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Status = status
	es.UpdatedAt = time.Now()

	// Set completion time for terminal states
	if IsTerminalStatus(status) {
		now := time.Now()
		es.CompletedAt = &now
	}
}

// SetError sets the error message on the execution state.
func SetError(state any, errMsg string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Error = errMsg
	es.UpdatedAt = time.Now()
}

// ClearError clears the error message from the execution state.
func ClearError(state any) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Error = ""
	es.UpdatedAt = time.Now()
}

// IncrementIteration increments the iteration counter.
func IncrementIteration(state any) int {
	es := ExtractExecutionState(state)
	if es == nil {
		return 0
	}

	es.Iteration++
	es.UpdatedAt = time.Now()
	return es.Iteration
}

// SetIteration sets the iteration counter to a specific value.
func SetIteration(state any, iteration int) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Iteration = iteration
	es.UpdatedAt = time.Now()
}

// SetPendingTask marks the execution as waiting for an async callback.
func SetPendingTask(state any, taskID string, ruleID string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.PendingTaskID = taskID
	es.PendingRuleID = ruleID
	es.Status = StatusWaiting
	es.UpdatedAt = time.Now()
}

// ClearPendingTask clears the pending task after callback received.
func ClearPendingTask(state any) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.PendingTaskID = ""
	es.PendingRuleID = ""
	if es.Status == StatusWaiting {
		es.Status = StatusRunning
	}
	es.UpdatedAt = time.Now()
}

// SetDeadline sets the execution deadline.
func SetDeadline(state any, deadline time.Time) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Deadline = &deadline
	es.UpdatedAt = time.Now()
}

// IsExpired checks if the execution has exceeded its deadline.
func IsExpired(state any) bool {
	es := ExtractExecutionState(state)
	if es == nil || es.Deadline == nil {
		return false
	}

	return time.Now().After(*es.Deadline)
}

// IsTerminalStatus returns true if the status is a terminal state.
func IsTerminalStatus(status ExecutionStatus) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusEscalated, StatusTimedOut:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if the execution is in a terminal state.
func IsTerminal(state any) bool {
	es := ExtractExecutionState(state)
	if es == nil {
		return false
	}
	return IsTerminalStatus(es.Status)
}

// CanProceed returns true if the execution can proceed (not terminal, not waiting, not expired).
func CanProceed(state any) bool {
	es := ExtractExecutionState(state)
	if es == nil {
		return false
	}

	if IsTerminalStatus(es.Status) {
		return false
	}

	if es.Status == StatusWaiting {
		return false
	}

	if es.Deadline != nil && time.Now().After(*es.Deadline) {
		return false
	}

	return true
}

// InitializeExecution initializes a new execution state.
func InitializeExecution(state any, id, workflowID string, timeout time.Duration) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	now := time.Now()
	es.ID = id
	es.WorkflowID = workflowID
	es.Phase = "pending"
	es.Iteration = 0
	es.Status = StatusRunning
	es.Error = ""
	es.PendingTaskID = ""
	es.PendingRuleID = ""
	es.CreatedAt = now
	es.UpdatedAt = now
	es.CompletedAt = nil
	es.Timeline = nil

	if timeout > 0 {
		deadline := now.Add(timeout)
		es.Deadline = &deadline
	}
}

// CompleteExecution marks the execution as completed successfully.
func CompleteExecution(state any, phase string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	now := time.Now()
	es.Phase = phase
	es.Status = StatusCompleted
	es.CompletedAt = &now
	es.UpdatedAt = now
	es.PendingTaskID = ""
	es.PendingRuleID = ""
}

// FailExecution marks the execution as failed.
func FailExecution(state any, errMsg string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	now := time.Now()
	es.Status = StatusFailed
	es.Error = errMsg
	es.CompletedAt = &now
	es.UpdatedAt = now
	es.PendingTaskID = ""
	es.PendingRuleID = ""
}

// EscalateExecution marks the execution as escalated.
func EscalateExecution(state any, reason string) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	now := time.Now()
	es.Status = StatusEscalated
	es.Error = reason
	es.CompletedAt = &now
	es.UpdatedAt = now
	es.PendingTaskID = ""
	es.PendingRuleID = ""
}

// TimeoutExecution marks the execution as timed out.
func TimeoutExecution(state any) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	now := time.Now()
	es.Status = StatusTimedOut
	es.Error = "execution timed out"
	es.CompletedAt = &now
	es.UpdatedAt = now
	es.PendingTaskID = ""
	es.PendingRuleID = ""
}

// RecordTimelineEntry adds a timeline entry to the execution state.
func RecordTimelineEntry(state any, entry TimelineEntry) {
	es := ExtractExecutionState(state)
	if es == nil {
		return
	}

	es.Timeline = append(es.Timeline, entry)
	es.UpdatedAt = time.Now()
}

// GetPhase returns the current phase of the execution.
func GetPhase(state any) string {
	es := ExtractExecutionState(state)
	if es == nil {
		return ""
	}
	return es.Phase
}

// GetStatus returns the current status of the execution.
func GetStatus(state any) ExecutionStatus {
	es := ExtractExecutionState(state)
	if es == nil {
		return ""
	}
	return es.Status
}

// GetIteration returns the current iteration count.
func GetIteration(state any) int {
	es := ExtractExecutionState(state)
	if es == nil {
		return 0
	}
	return es.Iteration
}

// GetID returns the execution ID.
func GetID(state any) string {
	es := ExtractExecutionState(state)
	if es == nil {
		return ""
	}
	return es.ID
}

// GetWorkflowID returns the workflow ID.
func GetWorkflowID(state any) string {
	es := ExtractExecutionState(state)
	if es == nil {
		return ""
	}
	return es.WorkflowID
}
