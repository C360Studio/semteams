package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// Compile-time assertions.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// federationSchema is the pre-generated config schema for this component.
var federationSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the federation processor.
// It subscribes to incoming EventPayload messages on a JetStream subject, applies
// federation merge policy via Merger, and republishes the filtered/merged event
// to the output subject.
type Component struct {
	name       string
	cfg        Config
	merger     *Merger
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Lifecycle state
	mu        sync.RWMutex
	running   bool
	startTime time.Time
	cancel    context.CancelFunc

	// Metrics
	processed    atomic.Int64
	filtered     atomic.Int64
	errors       atomic.Int64
	bytesIn      atomic.Int64
	lastActivity atomic.Value // time.Time
}

// NewComponent creates a federation processor component from raw JSON config.
// Follows the semstreams factory pattern: unmarshal → apply defaults → validate.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()

	// Unmarshal user-provided values on top of defaults.
	if len(rawConfig) > 0 && string(rawConfig) != "{}" {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("federation: unmarshal config: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("federation: invalid config: %w", err)
	}

	// Sync port definitions with config values.
	syncPortsWithConfig(&cfg)

	logger := deps.GetLogger().With("component", "federation-processor")

	return &Component{
		name:       "federation-processor",
		cfg:        cfg,
		merger:     NewMerger(cfg),
		natsClient: deps.NATSClient,
		logger:     logger,
	}, nil
}

// syncPortsWithConfig updates port definitions to match configured subjects/streams.
func syncPortsWithConfig(cfg *Config) {
	if cfg.Ports == nil {
		cfg.Ports = &component.PortConfig{}
	}
	if len(cfg.Ports.Inputs) > 0 {
		cfg.Ports.Inputs[0].Subject = cfg.InputSubject
		cfg.Ports.Inputs[0].StreamName = cfg.InputStream
	}
	if len(cfg.Ports.Outputs) > 0 {
		cfg.Ports.Outputs[0].Subject = cfg.OutputSubject
		cfg.Ports.Outputs[0].StreamName = cfg.OutputStream
	}
}

// Initialize performs pre-start initialization.
func (c *Component) Initialize() error {
	c.logger.Info("federation processor initialized",
		"namespace", c.cfg.LocalNamespace,
		"policy", c.cfg.MergePolicy,
		"input_subject", c.cfg.InputSubject,
		"output_subject", c.cfg.OutputSubject)
	return nil
}

// Start subscribes to the input JetStream subject and begins processing
// EventPayload messages in the background.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("federation: component already running")
	}
	if c.natsClient == nil {
		return fmt.Errorf("federation: NATS client required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	procCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	err := c.natsClient.ConsumeStreamWithConfig(procCtx, natsclient.StreamConsumerConfig{
		StreamName:    c.cfg.InputStream,
		ConsumerName:  "federation-processor",
		FilterSubject: c.cfg.InputSubject,
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
	}, c.handleMessage)
	if err != nil {
		cancel()
		return fmt.Errorf("federation: subscribe input: %w", err)
	}

	c.running = true
	c.startTime = time.Now()
	c.logger.Info("federation processor started",
		"namespace", c.cfg.LocalNamespace,
		"policy", c.cfg.MergePolicy)
	return nil
}

// Stop cancels the background goroutine and marks the component as stopped.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	c.running = false
	c.logger.Info("federation processor stopped",
		"processed", c.processed.Load(),
		"filtered", c.filtered.Load(),
		"errors", c.errors.Load())
	return nil
}

// handleMessage is the JetStream message handler. It deserialises the incoming
// BaseMessage, extracts the EventPayload, applies merge policy, and
// publishes the result to the output subject.
func (c *Component) handleMessage(ctx context.Context, msg jetstream.Msg) {
	c.bytesIn.Add(int64(len(msg.Data())))
	c.lastActivity.Store(time.Now())

	var base message.BaseMessage
	if err := json.Unmarshal(msg.Data(), &base); err != nil {
		c.logger.Warn("federation: unmarshal BaseMessage failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	// BaseMessage.Payload() returns message.Payload — assert to EventPayload.
	payload, ok := base.Payload().(*federation.EventPayload)
	if !ok {
		c.logger.Warn("federation: unexpected payload type",
			"got", fmt.Sprintf("%T", base.Payload()))
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	// TODO: Wire up federation.Store for stateful merge (edge union against
	// existing entities, provenance chain append). Currently stateless — each
	// event is merged without prior entity state.
	merged, err := c.merger.ApplyEvent(&payload.Event, nil)
	if err != nil {
		c.logger.Warn("federation: ApplyEvent failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	// Publish merged event.
	outPayload := &federation.EventPayload{Event: *merged}
	outType := message.Type{Domain: "federation", Category: "graph_event", Version: "v1"}
	outMsg := message.NewBaseMessage(outType, outPayload, c.name)
	data, err := json.Marshal(outMsg)
	if err != nil {
		c.logger.Warn("federation: marshal output failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	if err := c.natsClient.PublishToStream(ctx, c.cfg.OutputSubject, data); err != nil {
		c.logger.Warn("federation: publish merged event failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	c.processed.Add(1)
	_ = msg.Ack()
}

// --- component.Discoverable implementation ---

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "federation-processor",
		Type:        "processor",
		Description: "Applies federation merge policy to incoming graph event payloads",
		Version:     "1.0.0",
	}
}

// InputPorts returns the input port definitions.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "federation_events_in",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "JetStream input for incoming federation graph events",
			Config: component.JetStreamPort{
				StreamName: c.cfg.InputStream,
				Subjects:   []string{c.cfg.InputSubject},
			},
		},
	}
}

// OutputPorts returns the output port definitions.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "federation_events_out",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "JetStream output for merged federation graph events",
			Config: component.JetStreamPort{
				StreamName: c.cfg.OutputStream,
				Subjects:   []string{c.cfg.OutputSubject},
			},
		},
	}
}

// ConfigSchema returns the pre-generated configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return federationSchema
}

// Health returns the current health status of the component.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running
	startTime := c.startTime
	c.mu.RUnlock()

	status := "stopped"
	if running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors.Load()),
		Uptime:     time.Since(startTime),
		Status:     status,
	}
}

// DataFlow returns current message flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	running := c.running
	startTime := c.startTime
	c.mu.RUnlock()

	messages := c.processed.Load()
	bytes := c.bytesIn.Load()
	errorCount := c.errors.Load()

	var messagesPerSec, bytesPerSec, errorRate float64
	if running && !startTime.IsZero() {
		seconds := time.Since(startTime).Seconds()
		if seconds > 0 {
			messagesPerSec = float64(messages) / seconds
			bytesPerSec = float64(bytes) / seconds
		}
		total := messages + errorCount
		if total > 0 {
			errorRate = float64(errorCount) / float64(total)
		}
	}

	var lastAct time.Time
	if v := c.lastActivity.Load(); v != nil {
		lastAct = v.(time.Time)
	}

	return component.FlowMetrics{
		MessagesPerSecond: messagesPerSec,
		BytesPerSecond:    bytesPerSec,
		ErrorRate:         errorRate,
		LastActivity:      lastAct,
	}
}
