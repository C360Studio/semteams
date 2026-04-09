// Package cronticker provides a time-based input component that publishes
// clock-tick entities at configurable intervals. This enables time-triggered
// rules — the missing piece for cron-like behavior in SemStreams.
//
// Each tick publishes an entity to the configured NATS subject with a predictable
// entity ID pattern, allowing rules to watch for ticks and fire actions.
package cronticker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

var _ component.Discoverable = (*Component)(nil)

// Config holds configuration for the cron-ticker component.
type Config struct {
	// Interval between ticks (e.g., "1m", "5m", "1h")
	Interval string `json:"interval" schema:"type:string,description:Tick interval (e.g. 1m 5m 1h),default:5m,category:basic,required"`

	// TickerName identifies this ticker for entity ID generation.
	TickerName string `json:"ticker_name" schema:"type:string,description:Unique ticker name for entity IDs,default:default,category:basic"`

	// Subject is the NATS subject to publish tick entities to.
	Subject string `json:"subject" schema:"type:string,description:NATS subject for tick entities,default:cron.tick,category:basic"`

	// Ports configuration
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration,category:basic"`
}

// Validate checks the configuration.
func (c Config) Validate() error {
	if c.Interval == "" {
		return errs.WrapInvalid(fmt.Errorf("interval is required"), "Config", "Validate", "check interval")
	}
	if _, err := time.ParseDuration(c.Interval); err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "parse interval")
	}
	return nil
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Interval:   "5m",
		TickerName: "default",
		Subject:    "cron.tick",
	}
}

// tickEntityType is the message.Type for clock tick entities.
var tickEntityType = message.Type{
	Domain:   "cron",
	Category: "tick",
	Version:  "v1",
}

// Component publishes periodic clock tick entities.
type Component struct {
	config     Config
	interval   time.Duration
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Lifecycle
	running bool
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	// Metrics
	tickCount    int64
	lastTickTime time.Time
	startTime    time.Time
}

// NewComponent creates a new cron-ticker component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "unmarshal config")
	}

	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "validate config")
	}

	interval, _ := time.ParseDuration(config.Interval)

	return &Component{
		config:     config,
		interval:   interval,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
	}, nil
}

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "cron-ticker",
		Type:        "input",
		Description: "Publishes periodic clock-tick entities for time-triggered rules",
		Version:     "1.0.0",
	}
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return componentSchema
}

// InputPorts returns no input ports (this is an input component).
func (c *Component) InputPorts() []component.Port { return nil }

// OutputPorts returns the tick output port.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{{
		Name:      "tick",
		Direction: component.DirectionOutput,
	}}
}

// Health returns component health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return component.HealthStatus{
		Healthy:   c.running,
		LastCheck: time.Now(),
		Uptime:    time.Since(c.startTime),
		Status:    fmt.Sprintf("ticking every %s, %d ticks emitted", c.interval, c.tickCount),
	}
}

// DataFlow returns data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return component.FlowMetrics{
		LastActivity: c.lastTickTime,
	}
}

// Start begins the tick loop.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return errs.WrapInvalid(errs.ErrAlreadyStarted, "Component", "Start", "already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true
	c.startTime = time.Now()

	loopCtx := c.ctx // capture under lock to avoid race
	go c.tickLoop(loopCtx)

	c.logger.Info("Cron-ticker started",
		slog.String("interval", c.config.Interval),
		slog.String("ticker_name", c.config.TickerName),
		slog.String("subject", c.config.Subject))

	return nil
}

// Stop halts the tick loop.
func (c *Component) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.cancel()
	c.running = false

	c.logger.Info("Cron-ticker stopped", slog.Int64("total_ticks", c.tickCount))
	return nil
}

// tickLoop runs the periodic tick publisher. ctx is captured at Start() time
// to avoid an unsynchronized read of c.ctx from the goroutine.
func (c *Component) tickLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.emitTick(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.emitTick(ctx)
		}
	}
}

// emitTick publishes a single clock tick entity.
func (c *Component) emitTick(ctx context.Context) {
	now := time.Now().UTC()
	entityID := fmt.Sprintf("local.semstreams.cron.ticker.tick.%s", c.config.TickerName) // 6-part entity ID

	// Read tick count under lock for the triple value
	c.mu.RLock()
	nextCount := c.tickCount + 1
	c.mu.RUnlock()

	triples := []message.Triple{
		{Subject: entityID, Predicate: "cron.type", Object: "tick"},
		{Subject: entityID, Predicate: "cron.ticker_name", Object: c.config.TickerName},
		{Subject: entityID, Predicate: "cron.interval", Object: c.config.Interval},
		{Subject: entityID, Predicate: "cron.timestamp", Object: now.Format(time.RFC3339)},
		{Subject: entityID, Predicate: "cron.tick_count", Object: fmt.Sprintf("%d", nextCount)},
	}

	payload := &tickPayload{EntityID: entityID, Triples: triples}
	baseMsg := message.NewBaseMessage(tickEntityType, payload, "cron-ticker")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("Failed to marshal tick entity", slog.Any("error", err))
		return
	}

	if err := c.natsClient.Publish(ctx, c.config.Subject, data); err != nil {
		c.logger.Error("Failed to publish tick", slog.Any("error", err))
		return
	}

	c.mu.Lock()
	c.tickCount++
	count := c.tickCount
	c.lastTickTime = now
	c.mu.Unlock()

	c.logger.Debug("Tick emitted",
		slog.String("entity_id", entityID),
		slog.Int64("tick_count", count))
}

// tickPayload wraps tick triples as a message payload.
type tickPayload struct {
	EntityID string           `json:"entity_id"`
	Triples  []message.Triple `json:"triples"`
}

func (p *tickPayload) Schema() message.Type { return tickEntityType }
func (p *tickPayload) Validate() error      { return nil }

func (p *tickPayload) MarshalJSON() ([]byte, error) {
	type Alias tickPayload
	return json.Marshal((*Alias)(p))
}

func (p *tickPayload) UnmarshalJSON(data []byte) error {
	type Alias tickPayload
	return json.Unmarshal(data, (*Alias)(p))
}
