package resource

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcher_WaitForStartup_ImmediateSuccess(t *testing.T) {
	// Resource available immediately
	checkFn := func(ctx context.Context) error {
		return nil
	}

	w := NewWatcher("test-resource", checkFn, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !w.WaitForStartup(ctx) {
		t.Error("expected WaitForStartup to return true when resource is available")
	}

	if !w.IsAvailable() {
		t.Error("expected IsAvailable to return true after successful startup")
	}
}

func TestWatcher_WaitForStartup_EventualSuccess(t *testing.T) {
	// Resource becomes available after 3 attempts
	var attempts atomic.Int32
	checkFn := func(ctx context.Context) error {
		if attempts.Add(1) < 3 {
			return errors.New("not ready")
		}
		return nil
	}

	cfg := DefaultConfig()
	cfg.StartupInterval = 10 * time.Millisecond // Fast for testing
	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !w.WaitForStartup(ctx) {
		t.Error("expected WaitForStartup to return true after eventual availability")
	}

	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestWatcher_WaitForStartup_ExhaustsAttempts(t *testing.T) {
	// Resource never becomes available
	checkFn := func(ctx context.Context) error {
		return errors.New("never ready")
	}

	cfg := DefaultConfig()
	cfg.StartupAttempts = 3
	cfg.StartupInterval = 10 * time.Millisecond
	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if w.WaitForStartup(ctx) {
		t.Error("expected WaitForStartup to return false when attempts exhausted")
	}

	if w.IsAvailable() {
		t.Error("expected IsAvailable to return false after failed startup")
	}
}

func TestWatcher_WaitForStartup_ContextCancellation(t *testing.T) {
	// Context cancelled during startup
	checkFn := func(ctx context.Context) error {
		return errors.New("not ready")
	}

	cfg := DefaultConfig()
	cfg.StartupAttempts = 100 // Many attempts
	cfg.StartupInterval = 50 * time.Millisecond
	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := w.WaitForStartup(ctx)
	elapsed := time.Since(start)

	if result {
		t.Error("expected WaitForStartup to return false when context cancelled")
	}

	// Should have returned quickly due to cancellation, not waited for all attempts
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected early return on context cancellation, took %v", elapsed)
	}
}

func TestWatcher_BackgroundCheck_ResourceBecomesAvailable(t *testing.T) {
	// Resource unavailable at first, then becomes available in background
	var ready atomic.Bool
	var availableCalled atomic.Bool

	checkFn := func(ctx context.Context) error {
		if ready.Load() {
			return nil
		}
		return errors.New("not ready")
	}

	cfg := DefaultConfig()
	cfg.StartupAttempts = 1
	cfg.StartupInterval = 10 * time.Millisecond
	cfg.RecheckInterval = 50 * time.Millisecond
	cfg.OnAvailable = func() {
		availableCalled.Store(true)
	}

	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start up - resource not available
	if w.WaitForStartup(ctx) {
		t.Error("expected startup to fail initially")
	}

	// Start background checking
	w.StartBackgroundCheck(ctx)

	// Make resource available
	ready.Store(true)

	// Wait for background check to detect availability
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if w.IsAvailable() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !w.IsAvailable() {
		t.Error("expected resource to become available via background check")
	}

	if !availableCalled.Load() {
		t.Error("expected OnAvailable callback to be called")
	}

	w.Stop()
}

func TestWatcher_BackgroundCheck_ResourceLost(t *testing.T) {
	// Resource available initially, then lost
	var ready atomic.Bool
	ready.Store(true)

	var lostCalled atomic.Bool

	checkFn := func(ctx context.Context) error {
		if ready.Load() {
			return nil
		}
		return errors.New("resource lost")
	}

	cfg := DefaultConfig()
	cfg.StartupAttempts = 1
	cfg.HealthInterval = 50 * time.Millisecond
	cfg.OnLost = func() {
		lostCalled.Store(true)
	}

	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start up - resource available
	if !w.WaitForStartup(ctx) {
		t.Error("expected startup to succeed")
	}

	// Start background health checking
	w.StartBackgroundCheck(ctx)

	// Simulate resource loss
	ready.Store(false)

	// Wait for background check to detect loss
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !w.IsAvailable() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if w.IsAvailable() {
		t.Error("expected resource to be marked unavailable after loss")
	}

	if !lostCalled.Load() {
		t.Error("expected OnLost callback to be called")
	}

	w.Stop()
}

func TestWatcher_BackgroundCheck_Idempotent(t *testing.T) {
	// Calling StartBackgroundCheck multiple times should be safe
	checkFn := func(ctx context.Context) error {
		return nil
	}

	w := NewWatcher("test-resource", checkFn, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.WaitForStartup(ctx)

	// Start background check multiple times
	w.StartBackgroundCheck(ctx)
	w.StartBackgroundCheck(ctx)
	w.StartBackgroundCheck(ctx)

	// Should not panic or create multiple goroutines
	w.Stop()
}

func TestWatcher_Stop_Idempotent(t *testing.T) {
	checkFn := func(ctx context.Context) error {
		return nil
	}

	w := NewWatcher("test-resource", checkFn, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.WaitForStartup(ctx)
	w.StartBackgroundCheck(ctx)

	// Stop multiple times should be safe
	w.Stop()
	w.Stop()
	w.Stop()
}

func TestWatcher_Name(t *testing.T) {
	checkFn := func(ctx context.Context) error {
		return nil
	}

	w := NewWatcher("my-resource", checkFn, DefaultConfig())

	if w.Name() != "my-resource" {
		t.Errorf("expected name 'my-resource', got '%s'", w.Name())
	}
}

func TestWatcher_ConfigDefaults(t *testing.T) {
	checkFn := func(ctx context.Context) error {
		return nil
	}

	// Empty config should get defaults
	w := NewWatcher("test", checkFn, Config{})

	if w.startupAttempts != 10 {
		t.Errorf("expected startupAttempts=10, got %d", w.startupAttempts)
	}
	if w.startupInterval != 500*time.Millisecond {
		t.Errorf("expected startupInterval=500ms, got %v", w.startupInterval)
	}
	if w.recheckInterval != 60*time.Second {
		t.Errorf("expected recheckInterval=60s, got %v", w.recheckInterval)
	}
}

func TestWatcher_ConcurrentAccess(t *testing.T) {
	// Test that IsAvailable is safe for concurrent access
	var ready atomic.Bool
	ready.Store(true)

	checkFn := func(ctx context.Context) error {
		if ready.Load() {
			return nil
		}
		return errors.New("not ready")
	}

	cfg := DefaultConfig()
	cfg.HealthInterval = 10 * time.Millisecond
	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.WaitForStartup(ctx)
	w.StartBackgroundCheck(ctx)

	// Multiple goroutines reading availability
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = w.IsAvailable()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Toggle availability during concurrent reads
	go func() {
		for i := 0; i < 10; i++ {
			ready.Store(false)
			time.Sleep(10 * time.Millisecond)
			ready.Store(true)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()
	w.Stop()
}

func TestWatcher_HealthCheckDisabled(t *testing.T) {
	// When HealthInterval is 0, use recheck interval for available resources
	var checkCount atomic.Int32
	checkFn := func(ctx context.Context) error {
		checkCount.Add(1)
		return nil
	}

	cfg := DefaultConfig()
	cfg.HealthInterval = 0 // Disabled
	cfg.RecheckInterval = 50 * time.Millisecond
	w := NewWatcher("test-resource", checkFn, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.WaitForStartup(ctx)
	w.StartBackgroundCheck(ctx)

	// Wait for a few recheck intervals
	time.Sleep(200 * time.Millisecond)
	w.Stop()

	// Should have done startup check plus a few background checks
	if checkCount.Load() < 2 {
		t.Errorf("expected at least 2 checks, got %d", checkCount.Load())
	}
}
