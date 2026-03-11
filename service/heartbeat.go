// Package service provides the Heartbeat service for emitting periodic system health logs.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// HeartbeatConfig holds configuration for the Heartbeat service
type HeartbeatConfig struct {
	// Interval between heartbeat logs (e.g., "30s", "1m")
	// Default: "30s"
	Interval string `json:"interval"`
}

// Validate checks if the configuration is valid
func (c HeartbeatConfig) Validate() error {
	if c.Interval == "" {
		return nil // Will use default
	}

	duration, err := time.ParseDuration(c.Interval)
	if err != nil {
		return fmt.Errorf("invalid interval: %w", err)
	}
	if duration <= 0 {
		return fmt.Errorf("invalid interval: must be positive")
	}
	if duration < time.Second {
		return fmt.Errorf("invalid interval: must be at least 1s")
	}

	return nil
}

// componentHealthGetter defines the interface for getting component health
type componentHealthGetter interface {
	GetComponentHealth() map[string]bool
}

// HeartbeatService emits periodic system heartbeat logs
type HeartbeatService struct {
	*BaseService

	config    HeartbeatConfig
	interval  time.Duration
	startTime time.Time

	// Dependencies for gathering health info
	componentManager componentHealthGetter

	// Ticker for periodic heartbeat
	ticker *time.Ticker

	// Stop channel for goroutine coordination
	stopChan chan struct{}

	// WaitGroup for goroutine tracking
	wg sync.WaitGroup

	// Internal logger
	logger *slog.Logger
}

// NewHeartbeatService creates a new heartbeat service using the standard constructor pattern
func NewHeartbeatService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	// Parse config
	var cfg HeartbeatConfig
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("parse heartbeat config: %w", err)
		}
	}

	// Apply defaults
	if cfg.Interval == "" {
		cfg.Interval = "30s"
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate heartbeat config: %w", err)
	}

	// Parse interval
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid interval: %w", err)
	}

	// Create base service with options
	var opts []Option
	if deps != nil && deps.Logger != nil {
		opts = append(opts, WithLogger(deps.Logger))
	}

	baseService := NewBaseServiceWithOptions("heartbeat", nil, opts...)

	hb := &HeartbeatService{
		BaseService: baseService,
		config:      cfg,
		interval:    interval,
		stopChan:    make(chan struct{}),
		logger:      slog.Default().With("source", "heartbeat"),
	}

	// Try to get component manager from dependencies for health info
	if deps != nil && deps.ServiceManager != nil {
		if cm, ok := deps.ServiceManager.GetService("component-manager"); ok {
			if getter, ok := cm.(componentHealthGetter); ok {
				hb.componentManager = getter
			}
		}
	}

	return hb, nil
}

// Start begins the heartbeat service
func (hb *HeartbeatService) Start(ctx context.Context) error {
	if hb.Status() == StatusRunning {
		return fmt.Errorf("heartbeat service already running")
	}

	if err := hb.BaseService.Start(ctx); err != nil {
		return err
	}

	hb.startTime = time.Now()
	hb.logger.Info("Heartbeat service started",
		"interval", hb.config.Interval)

	// Start heartbeat loop
	hb.ticker = time.NewTicker(hb.interval)
	hb.wg.Add(1)
	go hb.heartbeatLoop(ctx)

	return nil
}

// Stop gracefully stops the heartbeat service
func (hb *HeartbeatService) Stop(timeout time.Duration) error {
	status := hb.Status()
	if status != StatusRunning && status != StatusStarting {
		return fmt.Errorf("heartbeat service not running (status: %v)", status)
	}

	hb.logger.Info("Heartbeat service stopping")

	// Stop the ticker
	if hb.ticker != nil {
		hb.ticker.Stop()
	}

	// Signal stop and wait for goroutine
	close(hb.stopChan)

	// Wait for goroutine with timeout
	done := make(chan struct{})
	go func() {
		hb.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutine finished
	case <-time.After(timeout):
		hb.logger.Warn("Heartbeat service stop timeout waiting for goroutine")
	}

	return hb.BaseService.Stop(timeout)
}

// heartbeatLoop emits periodic heartbeat logs
func (hb *HeartbeatService) heartbeatLoop(ctx context.Context) {
	defer hb.wg.Done()

	// Emit initial heartbeat on start
	hb.emitHeartbeat()

	for {
		select {
		case <-hb.stopChan:
			return
		case <-ctx.Done():
			return
		case <-hb.ticker.C:
			hb.emitHeartbeat()
		}
	}
}

// emitHeartbeat logs the system heartbeat
func (hb *HeartbeatService) emitHeartbeat() {
	uptime := time.Since(hb.startTime).Round(time.Second)
	goroutines := runtime.NumGoroutine()

	// Get component health if available
	healthyCount := 0
	totalCount := 0
	if hb.componentManager != nil {
		health := hb.componentManager.GetComponentHealth()
		totalCount = len(health)
		for _, healthy := range health {
			if healthy {
				healthyCount++
			}
		}
	}

	// Log heartbeat with structured fields
	hb.logger.Debug("System heartbeat",
		"uptime", uptime.String(),
		"goroutines", goroutines,
		"components_healthy", healthyCount,
		"components_total", totalCount,
	)
}

// newHeartbeatServiceForTest creates a HeartbeatService for testing
func newHeartbeatServiceForTest(config *HeartbeatConfig, componentManager componentHealthGetter) (*HeartbeatService, error) {
	if config == nil {
		config = &HeartbeatConfig{
			Interval: "30s",
		}
	}

	interval, err := time.ParseDuration(config.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid interval: %w", err)
	}

	baseService := NewBaseServiceWithOptions("heartbeat", nil)

	return &HeartbeatService{
		BaseService:      baseService,
		config:           *config,
		interval:         interval,
		stopChan:         make(chan struct{}),
		logger:           slog.Default().With("source", "heartbeat"),
		componentManager: componentManager,
	}, nil
}
