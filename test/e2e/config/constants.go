// Package config provides configuration for SemStreams E2E tests
package config

import "time"

// DefaultEndpoints provides default SemStreams service endpoints
// Ports use 3xxxx range to avoid collisions with other local services
var DefaultEndpoints = struct {
	HTTP    string
	UDP     string
	NATS    string
	Metrics string
	PProf   string
}{
	HTTP:    "http://localhost:38080",
	UDP:     "localhost:34550",
	NATS:    "nats://localhost:34222",
	Metrics: "http://localhost:39090",
	PProf:   "http://localhost:36060",
}

// ComponentPaths defines API paths for component endpoints
var ComponentPaths = struct {
	Base   string
	Health string
	List   string
}{
	Base:   "/components",
	Health: "/components/health",
	List:   "/components/list",
}

// ServicePaths defines API paths for service endpoints
var ServicePaths = struct {
	Health string
}{
	Health: "/health",
}

// DefaultTestConfig provides default test configuration values
var DefaultTestConfig = struct {
	// Test execution
	Timeout         time.Duration
	RetryInterval   time.Duration
	MaxRetries      int
	ValidationDelay time.Duration

	// core data test config
	MessageCount    int
	MessageInterval time.Duration
	MinProcessed    int
}{
	// Test execution
	Timeout:         10 * time.Second,
	RetryInterval:   1 * time.Second,
	MaxRetries:      30,
	ValidationDelay: 5 * time.Second,

	// core data testing
	MessageCount:    10,
	MessageInterval: 100 * time.Millisecond,
	MinProcessed:    5,
}
