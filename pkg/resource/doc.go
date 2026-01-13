// Package resource provides utilities for monitoring resource availability.
//
// # Overview
//
// The resource package provides a Watcher type that monitors the availability
// of external resources (KV buckets, streams, databases, etc.) and enables
// graceful degradation when resources are unavailable at startup.
//
// Key features:
//   - Bounded startup waiting with configurable retries
//   - Background re-checking when resources unavailable at startup
//   - Detection when resources disappear after becoming available
//   - Callbacks for availability state changes
//   - Graceful shutdown with goroutine cleanup
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        Resource Watcher                             │
//	├─────────────────────────────────────────────────────────────────────┤
//	│  WaitForStartup()   │  StartBackgroundCheck()  │  IsAvailable()     │
//	│  (bounded retries)  │  (periodic recheck)      │  (current state)   │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                    checkFn(ctx) → error                             │
//	│  Return nil if resource available, error otherwise                  │
//	└─────────────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Monitor a NATS KV bucket:
//
//	watcher := resource.NewWatcher(
//	    "COMMUNITY_INDEX",
//	    func(ctx context.Context) error {
//	        _, err := natsClient.GetKeyValueBucket(ctx, "COMMUNITY_INDEX")
//	        return err
//	    },
//	    resource.Config{
//	        StartupAttempts: 10,
//	        StartupInterval: 500 * time.Millisecond,
//	        RecheckInterval: 60 * time.Second,
//	        OnAvailable: func() {
//	            log.Println("COMMUNITY_INDEX bucket available, enabling GraphRAG")
//	        },
//	        OnLost: func() {
//	            log.Println("COMMUNITY_INDEX bucket lost, disabling GraphRAG")
//	        },
//	    },
//	)
//
//	// Wait for resource at startup
//	if available := watcher.WaitForStartup(ctx); !available {
//	    // Resource not available, start background monitoring
//	    log.Println("Starting in degraded mode, GraphRAG disabled")
//	    watcher.StartBackgroundCheck(ctx)
//	}
//
//	// Use throughout component lifecycle
//	if watcher.IsAvailable() {
//	    // Perform GraphRAG operations
//	}
//
//	// Cleanup
//	defer watcher.Stop()
//
// # Startup Behavior
//
// WaitForStartup() blocks until:
//   - checkFn returns nil (resource available), or
//   - All startup attempts exhausted
//
// Default timing:
//   - 10 attempts × 500ms = 5 seconds maximum wait
//
// This prevents services from hanging indefinitely on missing dependencies
// while giving resources time to initialize.
//
// # Background Checking
//
// When WaitForStartup() returns false (resource unavailable):
//
//  1. Service starts in degraded mode
//  2. StartBackgroundCheck() begins periodic checking
//  3. When resource becomes available, OnAvailable callback fires
//  4. Component can then enable full functionality
//
// This enables graceful degradation - services start quickly even when
// optional dependencies are unavailable, then upgrade functionality
// when dependencies appear.
//
// # Health Checking
//
// When resource is available, periodic health checks detect loss:
//
//	cfg := resource.Config{
//	    HealthInterval: 30 * time.Second,  // Check every 30s
//	    OnLost: func() {
//	        // Resource disappeared, switch to fallback behavior
//	    },
//	}
//
// Set HealthInterval to 0 to disable health checking.
//
// # Configuration
//
// Config options:
//
//	StartupAttempts  int           // Retries during WaitForStartup (default: 10)
//	StartupInterval  time.Duration // Delay between startup retries (default: 500ms)
//	RecheckInterval  time.Duration // Background recheck period (default: 60s)
//	HealthInterval   time.Duration // Health check period (default: 30s, 0 to disable)
//	OnAvailable      func()        // Called when resource becomes available
//	OnLost           func()        // Called when resource becomes unavailable
//	Logger           *slog.Logger  // Logger for events (default: slog.Default)
//
// Use DefaultConfig() for sensible defaults:
//
//	cfg := resource.DefaultConfig()
//	cfg.OnAvailable = func() { /* enable feature */ }
//
// # Common Patterns
//
// Optional dependency (graceful degradation):
//
//	// Component works without resource, but better with it
//	if watcher.IsAvailable() {
//	    return expensiveQuery(ctx, bucket)
//	}
//	return fallbackQuery(ctx)
//
// Required dependency (fail-fast):
//
//	// Component cannot function without resource
//	if !watcher.WaitForStartup(ctx) {
//	    return fmt.Errorf("required resource %s unavailable", watcher.Name())
//	}
//
// Multiple resources:
//
//	watchers := []*resource.Watcher{
//	    resource.NewWatcher("ENTITY_STATES", checkEntities, cfg),
//	    resource.NewWatcher("COMMUNITY_INDEX", checkCommunities, cfg),
//	}
//
//	for _, w := range watchers {
//	    if !w.WaitForStartup(ctx) {
//	        w.StartBackgroundCheck(ctx)
//	    }
//	    defer w.Stop()
//	}
//
// # Thread Safety
//
// Watcher is safe for concurrent use:
//   - IsAvailable() uses atomic.Bool
//   - Callbacks are invoked synchronously from background goroutine
//   - Stop() waits for background goroutine to exit
//
// # See Also
//
// Related packages:
//   - [github.com/c360/semstreams/processor/graph-query]: Uses Watcher for optional buckets
//   - [github.com/c360/semstreams/natsclient]: NATS KV bucket operations
package resource
