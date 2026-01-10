package component

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// KVLifecycleReporter implements LifecycleReporter using NATS KV storage.
// It writes component status to the COMPONENT_STATUS bucket.
type KVLifecycleReporter struct {
	kv            jetstream.KeyValue
	componentName string
	status        *ComponentStatus
	mu            sync.Mutex
	logger        *slog.Logger
}

// NewKVLifecycleReporter creates a new lifecycle reporter that writes to NATS KV.
func NewKVLifecycleReporter(kv jetstream.KeyValue, componentName string, logger *slog.Logger) *KVLifecycleReporter {
	if logger == nil {
		logger = slog.Default()
	}
	return &KVLifecycleReporter{
		kv:            kv,
		componentName: componentName,
		status: &ComponentStatus{
			Component:      componentName,
			Stage:          "idle",
			StageStartedAt: time.Now(),
		},
		logger: logger,
	}
}

// ReportStage updates the component's current processing stage.
func (r *KVLifecycleReporter) ReportStage(ctx context.Context, stage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.status.Stage = stage
	r.status.StageStartedAt = time.Now()

	return r.writeStatus(ctx)
}

// ReportCycleStart marks the beginning of a new processing cycle.
func (r *KVLifecycleReporter) ReportCycleStart(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.status.CycleID = uuid.New().String()[:8]
	r.status.CycleStartedAt = time.Now()
	r.status.StageStartedAt = time.Now()

	return r.writeStatus(ctx)
}

// ReportCycleComplete marks successful cycle completion.
func (r *KVLifecycleReporter) ReportCycleComplete(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.status.LastCompletedAt = time.Now()
	r.status.LastResult = "success"
	r.status.LastError = ""
	r.status.Stage = "idle"
	r.status.StageStartedAt = time.Now()

	return r.writeStatus(ctx)
}

// ReportCycleError marks cycle failure with error details.
func (r *KVLifecycleReporter) ReportCycleError(ctx context.Context, err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.status.LastCompletedAt = time.Now()
	r.status.LastResult = "error"
	if err != nil {
		r.status.LastError = err.Error()
	}
	r.status.Stage = "idle"
	r.status.StageStartedAt = time.Now()

	return r.writeStatus(ctx)
}

// writeStatus writes the current status to KV storage.
func (r *KVLifecycleReporter) writeStatus(ctx context.Context) error {
	data, err := json.Marshal(r.status)
	if err != nil {
		r.logger.Error("Failed to marshal component status",
			slog.String("component", r.componentName),
			slog.String("error", err.Error()))
		return err
	}

	if _, err := r.kv.Put(ctx, r.componentName, data); err != nil {
		r.logger.Error("Failed to write component status",
			slog.String("component", r.componentName),
			slog.String("error", err.Error()))
		return err
	}

	r.logger.Debug("Component status updated",
		slog.String("component", r.componentName),
		slog.String("stage", r.status.Stage),
		slog.String("cycle_id", r.status.CycleID))

	return nil
}

// NoOpLifecycleReporter is a no-op implementation for when lifecycle reporting is disabled.
type NoOpLifecycleReporter struct{}

// NewNoOpLifecycleReporter creates a no-op lifecycle reporter.
func NewNoOpLifecycleReporter() *NoOpLifecycleReporter {
	return &NoOpLifecycleReporter{}
}

// ReportStage is a no-op.
func (r *NoOpLifecycleReporter) ReportStage(ctx context.Context, stage string) error {
	return nil
}

// ReportCycleStart is a no-op.
func (r *NoOpLifecycleReporter) ReportCycleStart(ctx context.Context) error {
	return nil
}

// ReportCycleComplete is a no-op.
func (r *NoOpLifecycleReporter) ReportCycleComplete(ctx context.Context) error {
	return nil
}

// ReportCycleError is a no-op.
func (r *NoOpLifecycleReporter) ReportCycleError(ctx context.Context, err error) error {
	return nil
}
