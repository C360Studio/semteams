package component

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLifecycleReporter tracks calls for testing
type mockLifecycleReporter struct {
	stageCalls         []string
	cycleStartCount    int32
	cycleCompleteCount int32
	cycleErrorCount    int32
}

func (m *mockLifecycleReporter) ReportStage(_ context.Context, stage string) error {
	m.stageCalls = append(m.stageCalls, stage)
	return nil
}

func (m *mockLifecycleReporter) ReportCycleStart(_ context.Context) error {
	atomic.AddInt32(&m.cycleStartCount, 1)
	return nil
}

func (m *mockLifecycleReporter) ReportCycleComplete(_ context.Context) error {
	atomic.AddInt32(&m.cycleCompleteCount, 1)
	return nil
}

func (m *mockLifecycleReporter) ReportCycleError(_ context.Context, _ error) error {
	atomic.AddInt32(&m.cycleErrorCount, 1)
	return nil
}

func TestThrottledLifecycleReporter_ReportStage_ThrottlesWithinInterval(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call should go through immediately
	err := throttled.ReportStage(ctx, "stage1")
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1"}, mock.stageCalls)

	// Rapid calls within throttle window should be queued
	err = throttled.ReportStage(ctx, "stage2")
	require.NoError(t, err)
	err = throttled.ReportStage(ctx, "stage3")
	require.NoError(t, err)

	// Only the first call should have made it through
	assert.Equal(t, []string{"stage1"}, mock.stageCalls)

	// Wait for throttle window to pass
	time.Sleep(120 * time.Millisecond)

	// Now another call should go through
	err = throttled.ReportStage(ctx, "stage4")
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1", "stage4"}, mock.stageCalls)
}

func TestThrottledLifecycleReporter_ReportCycleComplete_NeverThrottled(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call to establish throttle window
	err := throttled.ReportStage(ctx, "stage1")
	require.NoError(t, err)

	// ReportCycleComplete should always go through, even within throttle window
	err = throttled.ReportCycleComplete(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), mock.cycleCompleteCount)

	// Call again immediately - should still go through
	err = throttled.ReportCycleComplete(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(2), mock.cycleCompleteCount)
}

func TestThrottledLifecycleReporter_ReportCycleError_NeverThrottled(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call to establish throttle window
	err := throttled.ReportStage(ctx, "stage1")
	require.NoError(t, err)

	// ReportCycleError should always go through, even within throttle window
	err = throttled.ReportCycleError(ctx, assert.AnError)
	require.NoError(t, err)
	assert.Equal(t, int32(1), mock.cycleErrorCount)

	// Call again immediately - should still go through
	err = throttled.ReportCycleError(ctx, assert.AnError)
	require.NoError(t, err)
	assert.Equal(t, int32(2), mock.cycleErrorCount)
}

func TestThrottledLifecycleReporter_ReportCycleStart_Throttled(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call should go through
	err := throttled.ReportCycleStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), mock.cycleStartCount)

	// Rapid calls within throttle window should be skipped
	err = throttled.ReportCycleStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), mock.cycleStartCount) // Still 1

	// Wait for throttle window to pass
	time.Sleep(120 * time.Millisecond)

	// Now another call should go through
	err = throttled.ReportCycleStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(2), mock.cycleStartCount)
}

func TestThrottledLifecycleReporter_FlushPending(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call goes through
	err := throttled.ReportStage(ctx, "stage1")
	require.NoError(t, err)

	// Second call is queued
	err = throttled.ReportStage(ctx, "stage2")
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1"}, mock.stageCalls)

	// Flush pending should write the queued stage
	err = throttled.FlushPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1", "stage2"}, mock.stageCalls)

	// Flush again should do nothing (no pending)
	err = throttled.FlushPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1", "stage2"}, mock.stageCalls)
}

func TestThrottledLifecycleReporter_ClearsPendingOnCycleComplete(t *testing.T) {
	mock := &mockLifecycleReporter{}
	throttled := NewThrottledLifecycleReporter(mock, 100*time.Millisecond, nil)
	ctx := context.Background()

	// First call goes through
	err := throttled.ReportStage(ctx, "stage1")
	require.NoError(t, err)

	// Second call is queued
	err = throttled.ReportStage(ctx, "pending_stage")
	require.NoError(t, err)

	// CycleComplete should clear pending state
	err = throttled.ReportCycleComplete(ctx)
	require.NoError(t, err)

	// Flush should do nothing now (pending was cleared)
	err = throttled.FlushPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"stage1"}, mock.stageCalls)
}

func TestThrottledLifecycleReporter_DefaultInterval(t *testing.T) {
	mock := &mockLifecycleReporter{}
	// Pass 0 for interval - should default to 1 second
	throttled := NewThrottledLifecycleReporter(mock, 0, nil)

	assert.Equal(t, 1*time.Second, throttled.minInterval)
}

func TestNewLifecycleReporterFromConfig_Disabled(t *testing.T) {
	cfg := LifecycleReporterConfig{
		ComponentName: "test",
		Disabled:      true,
	}

	reporter := NewLifecycleReporterFromConfig(cfg)

	// Should return NoOpLifecycleReporter
	_, ok := reporter.(*NoOpLifecycleReporter)
	assert.True(t, ok, "expected NoOpLifecycleReporter when disabled")
}

func TestNewLifecycleReporterFromConfig_NoKV(t *testing.T) {
	cfg := LifecycleReporterConfig{
		ComponentName: "test",
		KV:            nil, // No KV bucket
	}

	reporter := NewLifecycleReporterFromConfig(cfg)

	// Should return NoOpLifecycleReporter
	_, ok := reporter.(*NoOpLifecycleReporter)
	assert.True(t, ok, "expected NoOpLifecycleReporter when KV is nil")
}

func TestDefaultLifecycleReporterConfig(t *testing.T) {
	cfg := DefaultLifecycleReporterConfig("my-component")

	assert.Equal(t, "my-component", cfg.ComponentName)
	assert.True(t, cfg.EnableThrottling)
	assert.Equal(t, 1*time.Second, cfg.ThrottleInterval)
	assert.False(t, cfg.Disabled)
}
