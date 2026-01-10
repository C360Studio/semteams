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

// ============================================================================
// ThrottledLifecycleReporter - Rate-limited wrapper
// ============================================================================

// ThrottledLifecycleReporter wraps any LifecycleReporter to enforce minimum intervals
// between KV writes. Important events (CycleComplete, CycleError) are never throttled.
type ThrottledLifecycleReporter struct {
	delegate    LifecycleReporter
	minInterval time.Duration

	mu              sync.Mutex
	lastWriteTime   time.Time
	pendingStage    string
	hasPendingStage bool
	logger          *slog.Logger
}

// NewThrottledLifecycleReporter creates a throttled wrapper around any LifecycleReporter.
// minInterval is the minimum time between writes (default 1 second if zero).
func NewThrottledLifecycleReporter(delegate LifecycleReporter, minInterval time.Duration, logger *slog.Logger) *ThrottledLifecycleReporter {
	if minInterval == 0 {
		minInterval = 1 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ThrottledLifecycleReporter{
		delegate:    delegate,
		minInterval: minInterval,
		logger:      logger,
	}
}

// ReportStage updates the component's current processing stage.
// Throttled: if within minInterval, queues stage for next write window.
func (r *ThrottledLifecycleReporter) ReportStage(ctx context.Context, stage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	timeSinceLastWrite := now.Sub(r.lastWriteTime)

	if timeSinceLastWrite >= r.minInterval {
		// Outside throttle window - write immediately
		r.hasPendingStage = false
		r.pendingStage = ""
		r.lastWriteTime = now
		return r.delegate.ReportStage(ctx, stage)
	}

	// Within throttle window - queue for later
	r.pendingStage = stage
	r.hasPendingStage = true

	r.logger.Debug("stage throttled, queued for next window",
		slog.String("stage", stage),
		slog.Duration("time_until_window", r.minInterval-timeSinceLastWrite))

	return nil
}

// ReportCycleStart marks the beginning of a new processing cycle.
// Throttled: queued if within minInterval window.
func (r *ThrottledLifecycleReporter) ReportCycleStart(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	timeSinceLastWrite := now.Sub(r.lastWriteTime)

	if timeSinceLastWrite >= r.minInterval {
		// Outside throttle window - write immediately
		r.lastWriteTime = now
		return r.delegate.ReportCycleStart(ctx)
	}

	// Within throttle window - skip (cycle start is less critical)
	r.logger.Debug("cycle start throttled",
		slog.Duration("time_until_window", r.minInterval-timeSinceLastWrite))

	return nil
}

// ReportCycleComplete marks successful cycle completion.
// NEVER throttled - always written immediately. Also flushes any pending stage.
func (r *ThrottledLifecycleReporter) ReportCycleComplete(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear pending state - cycle is complete
	r.hasPendingStage = false
	r.pendingStage = ""
	r.lastWriteTime = time.Now()

	return r.delegate.ReportCycleComplete(ctx)
}

// ReportCycleError marks cycle failure with error details.
// NEVER throttled - always written immediately. Also flushes any pending stage.
func (r *ThrottledLifecycleReporter) ReportCycleError(ctx context.Context, err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear pending state - cycle has errored
	r.hasPendingStage = false
	r.pendingStage = ""
	r.lastWriteTime = time.Now()

	return r.delegate.ReportCycleError(ctx, err)
}

// FlushPending writes any pending stage to storage immediately.
// Call this before component shutdown or when immediate status update is needed.
func (r *ThrottledLifecycleReporter) FlushPending(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.hasPendingStage {
		return nil
	}

	r.lastWriteTime = time.Now()
	stage := r.pendingStage
	r.hasPendingStage = false
	r.pendingStage = ""

	return r.delegate.ReportStage(ctx, stage)
}

// ============================================================================
// LifecycleReporterConfig - Factory for creating reporters
// ============================================================================

// LifecycleReporterConfig holds configuration for creating lifecycle reporters.
type LifecycleReporterConfig struct {
	// KV is the NATS KV bucket for status storage. Required for KV reporter.
	KV jetstream.KeyValue

	// ComponentName is the name of the component. Required.
	ComponentName string

	// Logger for the reporter. Optional.
	Logger *slog.Logger

	// EnableThrottling enables throttled writes. Default: true.
	EnableThrottling bool

	// ThrottleInterval is the minimum interval between writes.
	// Only used if EnableThrottling is true. Default: 1 second.
	ThrottleInterval time.Duration

	// Disabled returns a no-op reporter if true.
	Disabled bool
}

// DefaultLifecycleReporterConfig returns configuration with sensible defaults.
// EnableThrottling is true with 1 second interval.
func DefaultLifecycleReporterConfig(componentName string) LifecycleReporterConfig {
	return LifecycleReporterConfig{
		ComponentName:    componentName,
		EnableThrottling: true,
		ThrottleInterval: 1 * time.Second,
		Disabled:         false,
	}
}

// NewLifecycleReporterFromConfig creates a lifecycle reporter based on configuration.
// Returns either a throttled KV reporter, plain KV reporter, or no-op reporter.
func NewLifecycleReporterFromConfig(cfg LifecycleReporterConfig) LifecycleReporter {
	// Return no-op if disabled
	if cfg.Disabled {
		return NewNoOpLifecycleReporter()
	}

	// Return no-op if no KV bucket provided
	if cfg.KV == nil {
		return NewNoOpLifecycleReporter()
	}

	// Create base KV reporter
	baseReporter := NewKVLifecycleReporter(cfg.KV, cfg.ComponentName, cfg.Logger)

	// Wrap with throttling if enabled
	if cfg.EnableThrottling {
		interval := cfg.ThrottleInterval
		if interval == 0 {
			interval = 1 * time.Second
		}
		return NewThrottledLifecycleReporter(baseReporter, interval, cfg.Logger)
	}

	return baseReporter
}
