package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// ExecutionState represents the state of a workflow execution
type ExecutionState string

// ExecutionState values represent the possible states of a workflow execution.
const (
	ExecutionStatePending   ExecutionState = "pending"
	ExecutionStateRunning   ExecutionState = "running"
	ExecutionStateCompleted ExecutionState = "completed"
	ExecutionStateFailed    ExecutionState = "failed"
	ExecutionStateTimedOut  ExecutionState = "timed_out"
)

// Special step name constants for workflow transitions
const (
	StepNameComplete = "complete"
	StepNameFail     = "fail"
)

// IsTerminal returns true if the state is terminal (no further transitions)
func (s ExecutionState) IsTerminal() bool {
	return s == ExecutionStateCompleted || s == ExecutionStateFailed || s == ExecutionStateTimedOut
}

// Execution represents a running workflow instance
type Execution struct {
	mu sync.RWMutex `json:"-"` // Protects all fields

	ID            string                `json:"id"`
	WorkflowID    string                `json:"workflow_id"`
	WorkflowName  string                `json:"workflow_name,omitempty"`
	State         ExecutionState        `json:"state"`
	CurrentStep   int                   `json:"current_step"`
	CurrentName   string                `json:"current_name,omitempty"`
	Iteration     int                   `json:"iteration"`
	Trigger       TriggerContext        `json:"trigger"`
	StepResults   map[string]StepResult `json:"step_results"`
	StartedAt     time.Time             `json:"started_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	CompletedAt   *time.Time            `json:"completed_at,omitempty"`
	Deadline      time.Time             `json:"deadline"`
	Error         string                `json:"error,omitempty"`
	PendingTaskID string                `json:"pending_task_id,omitempty"` // Task ID waiting for agent completion
}

// NewExecution creates a new workflow execution
func NewExecution(workflowID, workflowName string, trigger TriggerContext, timeout time.Duration) *Execution {
	now := time.Now()
	return &Execution{
		ID:           generateExecutionID(),
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		State:        ExecutionStatePending,
		CurrentStep:  0,
		Iteration:    1,
		Trigger:      trigger,
		StepResults:  make(map[string]StepResult),
		StartedAt:    now,
		UpdatedAt:    now,
		Deadline:     now.Add(timeout),
	}
}

// IsTimedOut checks if the execution has exceeded its deadline
func (e *Execution) IsTimedOut() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return time.Now().After(e.Deadline)
}

// GetState returns the current execution state
func (e *Execution) GetState() ExecutionState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.State
}

// GetIteration returns the current iteration count
func (e *Execution) GetIteration() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Iteration
}

// GetCurrentName returns the current step name
func (e *Execution) GetCurrentName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.CurrentName
}

// MarkRunning transitions the execution to running state
func (e *Execution) MarkRunning() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.State = ExecutionStateRunning
	e.UpdatedAt = time.Now()
}

// MarkCompleted transitions the execution to completed state
func (e *Execution) MarkCompleted() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	e.State = ExecutionStateCompleted
	e.UpdatedAt = now
	e.CompletedAt = &now
}

// MarkFailed transitions the execution to failed state
func (e *Execution) MarkFailed(err string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	e.State = ExecutionStateFailed
	e.Error = err
	e.UpdatedAt = now
	e.CompletedAt = &now
}

// MarkTimedOut transitions the execution to timed out state
func (e *Execution) MarkTimedOut() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	e.State = ExecutionStateTimedOut
	e.Error = "workflow timeout exceeded"
	e.UpdatedAt = now
	e.CompletedAt = &now
}

// RecordStepResult records the result of a step execution
func (e *Execution) RecordStepResult(stepName string, result StepResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.StepResults[stepName] = result
	e.UpdatedAt = time.Now()
}

// IncrementIteration increments the iteration counter for loop workflows
func (e *Execution) IncrementIteration() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Iteration++
	e.UpdatedAt = time.Now()
}

// SetCurrentStep updates the current step information
func (e *Execution) SetCurrentStep(index int, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.CurrentStep = index
	e.CurrentName = name
	e.UpdatedAt = time.Now()
}

// SetPendingTaskID sets the task ID that this execution is waiting for
func (e *Execution) SetPendingTaskID(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PendingTaskID = taskID
	e.UpdatedAt = time.Now()
}

// ClearPendingTaskID clears the pending task ID after completion
func (e *Execution) ClearPendingTaskID() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PendingTaskID = ""
	e.UpdatedAt = time.Now()
}

// GetPendingTaskID returns the pending task ID
func (e *Execution) GetPendingTaskID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.PendingTaskID
}

// GetStepResult returns the result for a specific step
func (e *Execution) GetStepResult(stepName string) (StepResult, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result, ok := e.StepResults[stepName]
	return result, ok
}

// Clone returns a deep copy of the execution for safe reading
func (e *Execution) Clone() *Execution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	clone := &Execution{
		ID:           e.ID,
		WorkflowID:   e.WorkflowID,
		WorkflowName: e.WorkflowName,
		State:        e.State,
		CurrentStep:  e.CurrentStep,
		CurrentName:  e.CurrentName,
		Iteration:    e.Iteration,
		Trigger:      e.Trigger,
		StepResults:  make(map[string]StepResult, len(e.StepResults)),
		StartedAt:    e.StartedAt,
		UpdatedAt:    e.UpdatedAt,
		Deadline:     e.Deadline,
		Error:        e.Error,
	}

	if e.CompletedAt != nil {
		t := *e.CompletedAt
		clone.CompletedAt = &t
	}

	for k, v := range e.StepResults {
		clone.StepResults[k] = v
	}

	return clone
}

// TriggerContext contains context from the trigger event
type TriggerContext struct {
	Subject   string            `json:"subject"`
	Payload   json.RawMessage   `json:"payload,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// MaxPayloadSize is the maximum allowed size for trigger payloads (1MB)
const MaxPayloadSize = 1024 * 1024

// ValidatePayloadSize checks if the payload is within acceptable limits
func (t *TriggerContext) ValidatePayloadSize() error {
	if len(t.Payload) > MaxPayloadSize {
		return errs.WrapInvalid(fmt.Errorf("payload size %d exceeds maximum %d bytes", len(t.Payload), MaxPayloadSize), "workflow-execution", "ValidatePayloadSize", "check payload size")
	}
	return nil
}

// StepResult represents the result of a step execution
type StepResult struct {
	StepName    string          `json:"step_name"`
	Status      string          `json:"status"` // success, failed, skipped
	Output      json.RawMessage `json:"output,omitempty"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
	Duration    time.Duration   `json:"duration"`
	Iteration   int             `json:"iteration"` // Which iteration this result is from
}

// ExecutionStore manages workflow execution persistence
type ExecutionStore struct {
	bucket jetstream.KeyValue
}

// NewExecutionStore creates a new execution store
func NewExecutionStore(bucket jetstream.KeyValue) *ExecutionStore {
	return &ExecutionStore{bucket: bucket}
}

// Save persists an execution to KV
func (s *ExecutionStore) Save(ctx context.Context, exec *Execution) error {
	// Use Clone to get a consistent snapshot for marshaling
	snapshot := exec.Clone()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return errs.WrapInvalid(err, "workflow-execution", "Save", "marshal execution")
	}

	if _, err := s.bucket.Put(ctx, snapshot.ID, data); err != nil {
		return errs.WrapTransient(err, "workflow-execution", "Save", "save to KV bucket")
	}

	return nil
}

// Get retrieves an execution from KV
func (s *ExecutionStore) Get(ctx context.Context, execID string) (*Execution, error) {
	entry, err := s.bucket.Get(ctx, execID)
	if err != nil {
		return nil, errs.WrapTransient(err, "workflow-execution", "Get", "get from KV bucket")
	}

	var exec Execution
	if err := json.Unmarshal(entry.Value(), &exec); err != nil {
		return nil, errs.WrapInvalid(err, "workflow-execution", "Get", "unmarshal execution")
	}

	// Initialize the map if nil (for executions created before map was initialized)
	if exec.StepResults == nil {
		exec.StepResults = make(map[string]StepResult)
	}

	return &exec, nil
}

// Delete removes an execution from KV
func (s *ExecutionStore) Delete(ctx context.Context, execID string) error {
	if err := s.bucket.Delete(ctx, execID); err != nil {
		return errs.WrapTransient(err, "workflow-execution", "Delete", "delete from KV bucket")
	}
	return nil
}

// SaveTaskIndex stores a task_id -> execution_id mapping for completion correlation
func (s *ExecutionStore) SaveTaskIndex(ctx context.Context, taskID, execID string) error {
	key := "TASK_" + taskID
	if _, err := s.bucket.Put(ctx, key, []byte(execID)); err != nil {
		return errs.WrapTransient(err, "workflow-execution", "SaveTaskIndex", "save task index")
	}
	return nil
}

// GetByTaskID finds an execution by its pending task ID using the secondary index
func (s *ExecutionStore) GetByTaskID(ctx context.Context, taskID string) (*Execution, error) {
	key := "TASK_" + taskID
	entry, err := s.bucket.Get(ctx, key)
	if err != nil {
		return nil, errs.Wrap(err, "workflow-execution", "GetByTaskID", "get task index")
	}

	execID := string(entry.Value())
	return s.Get(ctx, execID)
}

// DeleteTaskIndex removes the task_id -> execution_id mapping
func (s *ExecutionStore) DeleteTaskIndex(ctx context.Context, taskID string) error {
	key := "TASK_" + taskID
	_ = s.bucket.Delete(ctx, key) // Ignore error if key doesn't exist
	return nil
}

// generateExecutionID generates a unique execution ID using timestamp and random bytes
func generateExecutionID() string {
	// Use timestamp prefix for rough ordering plus random bytes for uniqueness
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to just timestamp if crypto/rand fails (extremely unlikely)
		return fmt.Sprintf("exec_%d", timestamp)
	}
	return fmt.Sprintf("exec_%d_%s", timestamp, hex.EncodeToString(randomBytes))
}

// event represents a workflow lifecycle event
type event struct {
	Type        string         `json:"type"` // started, step_started, step_completed, completed, failed, timed_out
	ExecutionID string         `json:"execution_id"`
	WorkflowID  string         `json:"workflow_id"`
	StepName    string         `json:"step_name,omitempty"`
	Iteration   int            `json:"iteration,omitempty"`
	State       ExecutionState `json:"state,omitempty"`
	Error       string         `json:"error,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

// StepCompleteMessage represents a step completion message from an agent
type StepCompleteMessage struct {
	// Required fields
	ExecutionID string `json:"execution_id"`
	StepName    string `json:"step_name"`
	Status      string `json:"status"` // success, failed

	// Timing fields
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Duration    string    `json:"duration,omitempty"` // Duration as string (e.g., "1.5s")

	// Iteration context
	Iteration int `json:"iteration,omitempty"` // Which loop iteration this belongs to

	// Output/Error
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Validate checks if the StepCompleteMessage is valid
func (m StepCompleteMessage) Validate() error {
	if m.ExecutionID == "" {
		return errs.WrapInvalid(fmt.Errorf("execution_id required"), "workflow-execution", "StepCompleteMessage.Validate", "validate execution_id")
	}
	if m.StepName == "" {
		return errs.WrapInvalid(fmt.Errorf("step_name required"), "workflow-execution", "StepCompleteMessage.Validate", "validate step_name")
	}
	if m.Status != "success" && m.Status != "failed" {
		return errs.WrapInvalid(fmt.Errorf("status must be one of: success, failed"), "workflow-execution", "StepCompleteMessage.Validate", "validate status")
	}
	// Timing fields are required
	if m.StartedAt.IsZero() {
		return errs.WrapInvalid(fmt.Errorf("started_at required"), "workflow-execution", "StepCompleteMessage.Validate", "validate started_at")
	}
	if m.CompletedAt.IsZero() {
		return errs.WrapInvalid(fmt.Errorf("completed_at required"), "workflow-execution", "StepCompleteMessage.Validate", "validate completed_at")
	}
	if m.CompletedAt.Before(m.StartedAt) {
		return errs.WrapInvalid(fmt.Errorf("completed_at cannot be before started_at"), "workflow-execution", "StepCompleteMessage.Validate", "validate timing")
	}
	if m.Duration == "" {
		return errs.WrapInvalid(fmt.Errorf("duration required"), "workflow-execution", "StepCompleteMessage.Validate", "validate duration")
	}
	if _, err := time.ParseDuration(m.Duration); err != nil {
		return errs.WrapInvalid(err, "workflow-execution", "StepCompleteMessage.Validate", "parse duration")
	}
	// Iteration must be positive (starts at 1)
	if m.Iteration < 1 {
		return errs.WrapInvalid(fmt.Errorf("iteration must be >= 1"), "workflow-execution", "StepCompleteMessage.Validate", "validate iteration")
	}
	return nil
}

// Schema implements message.Payload
func (m *StepCompleteMessage) Schema() message.Type {
	return message.Type{Domain: "workflow", Category: "step_complete", Version: "v1"}
}

// MarshalJSON implements json.Marshaler
func (m *StepCompleteMessage) MarshalJSON() ([]byte, error) {
	type Alias StepCompleteMessage
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler
func (m *StepCompleteMessage) UnmarshalJSON(data []byte) error {
	type Alias StepCompleteMessage
	return json.Unmarshal(data, (*Alias)(m))
}

// Validate checks if the Event is valid
func (e event) Validate() error {
	if e.Type == "" {
		return errs.WrapInvalid(fmt.Errorf("type required"), "workflow-execution", "event.Validate", "validate type")
	}
	if e.ExecutionID == "" {
		return errs.WrapInvalid(fmt.Errorf("execution_id required"), "workflow-execution", "event.Validate", "validate execution_id")
	}
	if e.WorkflowID == "" {
		return errs.WrapInvalid(fmt.Errorf("workflow_id required"), "workflow-execution", "event.Validate", "validate workflow_id")
	}
	return nil
}

// Schema implements message.Payload
func (e *event) Schema() message.Type {
	return message.Type{Domain: "workflow", Category: "event", Version: "v1"}
}

// MarshalJSON implements json.Marshaler
func (e *event) MarshalJSON() ([]byte, error) {
	type Alias event
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler
func (e *event) UnmarshalJSON(data []byte) error {
	type Alias event
	return json.Unmarshal(data, (*Alias)(e))
}

// TriggerPayload represents an incoming trigger to start a workflow
type TriggerPayload struct {
	// Required: workflow to execute
	WorkflowID string `json:"workflow_id"`

	// Agentic context (aligned with TaskMessage pattern)
	Role   string `json:"role,omitempty"`   // Agent role for agentic workflows
	Model  string `json:"model,omitempty"`  // LLM model to use
	Prompt string `json:"prompt,omitempty"` // Work instruction/prompt

	// User/session context (for response routing)
	UserID      string `json:"user_id,omitempty"`      // Who triggered this workflow
	ChannelType string `json:"channel_type,omitempty"` // Response channel (http, cli, slack)
	ChannelID   string `json:"channel_id,omitempty"`   // Specific channel/session ID

	// Correlation
	RequestID string `json:"request_id,omitempty"` // Correlation ID for tracing

	// Custom data (fallback for truly custom fields)
	Data json.RawMessage `json:"data,omitempty"`
}

// Validate checks if the TriggerPayload is valid
func (p TriggerPayload) Validate() error {
	if p.WorkflowID == "" {
		return errs.WrapInvalid(fmt.Errorf("workflow_id required"), "workflow-execution", "TriggerPayload.Validate", "validate workflow_id")
	}
	// Validate Data is valid JSON if present
	if p.Data != nil && len(p.Data) > 0 {
		var temp any
		if err := json.Unmarshal(p.Data, &temp); err != nil {
			return errs.WrapInvalid(err, "workflow-execution", "TriggerPayload.Validate", "validate data JSON")
		}
	}
	return nil
}

// Schema implements message.Payload
func (p *TriggerPayload) Schema() message.Type {
	return message.Type{Domain: "workflow", Category: "trigger", Version: "v1"}
}

// MarshalJSON implements json.Marshaler
func (p *TriggerPayload) MarshalJSON() ([]byte, error) {
	type Alias TriggerPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler
func (p *TriggerPayload) UnmarshalJSON(data []byte) error {
	type Alias TriggerPayload
	return json.Unmarshal(data, (*Alias)(p))
}
