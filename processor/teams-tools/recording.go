package teamtools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semteams/teams"
)

// ToolCallRecord captures a tool call and its result for auditing
type ToolCallRecord struct {
	Call      agentic.ToolCall   `json:"call"`
	Result    agentic.ToolResult `json:"result"`
	StartTime time.Time          `json:"start_time"`
	EndTime   time.Time          `json:"end_time"`
	Duration  time.Duration      `json:"duration"`
}

// ToolCallStore interface for persisting tool call records
type ToolCallStore interface {
	Store(ctx context.Context, record ToolCallRecord) error
	Close() error
}

// RecordingExecutor wraps a ToolExecutor and records all calls to a store.
// It uses a buffered channel and background goroutine for non-blocking recording.
type RecordingExecutor struct {
	wrapped  ToolExecutor
	store    ToolCallStore
	logger   *slog.Logger
	wg       sync.WaitGroup
	shutdown chan struct{}
	records  chan ToolCallRecord
}

// NewRecordingExecutor creates a new recording executor that wraps the given
// executor and records all calls to the provided store.
func NewRecordingExecutor(wrapped ToolExecutor, store ToolCallStore, logger *slog.Logger) *RecordingExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	r := &RecordingExecutor{
		wrapped:  wrapped,
		store:    store,
		logger:   logger,
		shutdown: make(chan struct{}),
		records:  make(chan ToolCallRecord, 100),
	}
	r.wg.Add(1)
	go r.recordingLoop()
	return r
}

// recordingLoop processes records from the channel until shutdown
func (r *RecordingExecutor) recordingLoop() {
	defer r.wg.Done()
	for {
		select {
		case record := <-r.records:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.store.Store(ctx, record); err != nil {
				r.logger.Warn("failed to store tool call record",
					slog.String("call_id", record.Call.ID),
					slog.Any("error", err))
			}
			cancel()
		case <-r.shutdown:
			// Drain remaining records before exiting
			r.drainRecords()
			return
		}
	}
}

// drainRecords processes any remaining records in the channel
func (r *RecordingExecutor) drainRecords() {
	for {
		select {
		case record := <-r.records:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.store.Store(ctx, record); err != nil {
				r.logger.Warn("failed to store tool call record during drain",
					slog.String("call_id", record.Call.ID),
					slog.Any("error", err))
			}
			cancel()
		default:
			return
		}
	}
}

// Execute executes the wrapped tool and records the call
func (r *RecordingExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	startTime := time.Now()
	result, err := r.wrapped.Execute(ctx, call)
	endTime := time.Now()

	record := ToolCallRecord{
		Call:      call,
		Result:    result,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
	}

	// Non-blocking send to record channel
	select {
	case r.records <- record:
	default:
		r.logger.Warn("recording buffer full, dropping record",
			slog.String("call_id", call.ID))
	}

	return result, err
}

// ListTools returns tools from the wrapped executor
func (r *RecordingExecutor) ListTools() []teams.ToolDefinition {
	return r.wrapped.ListTools()
}

// Stop gracefully shuts down the recording executor, waiting for pending
// records to be stored up to the specified timeout.
func (r *RecordingExecutor) Stop(timeout time.Duration) error {
	close(r.shutdown)

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return r.store.Close()
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for recording goroutine to finish")
	}
}
