package iotsensor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
)

// ComponentConfig holds configuration for the IoT sensor processor component.
// This wraps the domain-specific processor configuration with port information
// required by the component framework.
type ComponentConfig struct {
	// Ports defines NATS input/output subjects for message routing
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// OrgID is the organization identifier for federated entity IDs
	OrgID string `json:"org_id" schema:"type:string,description:Organization identifier,category:basic,required:true"`

	// Platform is the platform/product identifier for federated entity IDs
	Platform string `json:"platform" schema:"type:string,description:Platform identifier,category:basic,required:true"`
}

// DefaultConfig returns the default configuration for IoT sensor processor
func DefaultConfig() ComponentConfig {
	inputDefs := []component.PortDefinition{
		{
			Name:        "nats_input",
			Type:        "nats",
			Subject:     "raw.sensor.>",
			Required:    true,
			Description: "NATS subjects with sensor JSON data",
		},
	}

	outputDefs := []component.PortDefinition{
		{
			Name:        "nats_output",
			Type:        "nats",
			Subject:     "events.graph.entity.sensor",
			Interface:   "domain.iot.sensor.v1",
			Required:    true,
			Description: "NATS subject for Graphable sensor readings",
		},
	}

	return ComponentConfig{
		Ports: &component.PortConfig{
			Inputs:  inputDefs,
			Outputs: outputDefs,
		},
		OrgID:    "default-org",
		Platform: "default-platform",
	}
}

// iotSensorSchema defines the configuration schema for IoT sensor processor
var iotSensorSchema = component.GenerateConfigSchema(reflect.TypeOf(ComponentConfig{}))

// Component wraps the domain-specific IoT sensor processor with component lifecycle.
// It bridges the gap between the stateless domain processor and the stateful
// component framework that handles NATS messaging and lifecycle management.
type Component struct {
	name        string
	subjects    []string
	outputSubj  string
	outputPorts []component.PortDefinition // Store full port definitions for OutputPorts()
	config      ComponentConfig            // Store full config for port type checking
	natsClient  *natsclient.Client
	logger      *slog.Logger

	// Domain processor (stateless, pure business logic)
	processor *Processor

	// Lifecycle management
	shutdown    chan struct{}
	done        chan struct{}
	running     bool
	startTime   time.Time
	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	wg          *sync.WaitGroup

	// Metrics
	messagesProcessed int64
	messagesWrapped   int64
	errors            int64
	lastActivity      time.Time
}

// NewComponent creates a new IoT sensor processor component from configuration.
// This is the factory function registered with the component registry.
func NewComponent(
	rawConfig json.RawMessage, deps component.Dependencies,
) (component.Discoverable, error) {
	var config ComponentConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "IoTSensorComponent", "NewComponent", "config unmarshal")
	}

	if config.Ports == nil {
		config = DefaultConfig()
	}

	// Validate configuration
	if config.OrgID == "" {
		return nil, errs.WrapInvalid(
			errs.ErrInvalidConfig, "IoTSensorComponent", "NewComponent",
			"OrgID is required")
	}

	if config.Platform == "" {
		return nil, errs.WrapInvalid(
			errs.ErrInvalidConfig, "IoTSensorComponent", "NewComponent",
			"Platform is required")
	}

	// Extract subjects from port configuration
	var inputSubjects []string
	var outputSubject string

	for _, input := range config.Ports.Inputs {
		if input.Type == "nats" || input.Type == "jetstream" {
			inputSubjects = append(inputSubjects, input.Subject)
		}
	}

	if len(config.Ports.Outputs) > 0 {
		outputSubject = config.Ports.Outputs[0].Subject
	}

	if len(inputSubjects) == 0 {
		return nil, errs.WrapInvalid(
			errs.ErrInvalidConfig, "IoTSensorComponent", "NewComponent",
			"no input subjects configured")
	}

	// Create domain processor with organizational context
	processor := NewProcessor(Config{
		OrgID:    config.OrgID,
		Platform: config.Platform,
	})

	return &Component{
		name:        "iot-sensor-processor",
		subjects:    inputSubjects,
		outputSubj:  outputSubject,
		outputPorts: config.Ports.Outputs, // Store full port definitions
		config:      config,               // Store full config for port type checking
		natsClient:  deps.NATSClient,
		logger:      deps.GetLogger(),
		processor:   processor,
		shutdown:    make(chan struct{}),
		done:        make(chan struct{}),
		wg:          &sync.WaitGroup{},
	}, nil
}

// Initialize prepares the component (no-op for IoT sensor processor)
func (c *Component) Initialize() error {
	return nil
}

// Start begins processing sensor messages
func (c *Component) Start(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	if c.running {
		return errs.WrapFatal(errs.ErrAlreadyStarted, "IoTSensorComponent", "Start", "check running state")
	}

	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrMissingConfig, "IoTSensorComponent", "Start", "NATS client required")
	}

	// Subscribe to input subjects
	for _, subject := range c.subjects {
		c.logger.Debug("Subscribing to NATS subject",
			"component", c.name,
			"subject", subject)

		if err := c.natsClient.Subscribe(ctx, subject, c.handleMessage); err != nil {
			c.logger.Error("Failed to subscribe to NATS subject",
				"component", c.name,
				"subject", subject,
				"error", err)
			return errs.WrapTransient(err, "IoTSensorComponent", "Start", fmt.Sprintf("subscribe to %s", subject))
		}

		c.logger.Debug("Subscribed to NATS subject successfully",
			"component", c.name,
			"subject", subject,
			"output_subject", c.outputSubj)
	}

	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	c.logger.Info("IoT sensor processor started",
		"component", c.name,
		"input_subjects", c.subjects,
		"output_subject", c.outputSubj)

	return nil
}

// Stop gracefully stops the component
func (c *Component) Stop(timeout time.Duration) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	if !c.running {
		return nil
	}

	// Signal shutdown
	close(c.shutdown)

	// Wait for goroutines with timeout
	waitCh := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Clean shutdown
	case <-time.After(timeout):
		return errs.WrapTransient(
			fmt.Errorf("shutdown timeout after %v", timeout),
			"IoTSensorComponent", "Stop", "graceful shutdown")
	}

	c.mu.Lock()
	c.running = false
	close(c.done)
	c.mu.Unlock()

	return nil
}

// IsStarted returns whether the component is running
func (c *Component) IsStarted() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// isJetStreamPortBySubject checks if an output port with the given subject is configured for JetStream
func (c *Component) isJetStreamPortBySubject(subject string) bool {
	if c.config.Ports == nil {
		return false
	}
	for _, port := range c.config.Ports.Outputs {
		if port.Subject == subject {
			return port.Type == "jetstream"
		}
	}
	return false
}

// handleMessage processes incoming sensor JSON messages.
// This is the bridge between NATS transport and domain logic:
//  1. Parse incoming JSON
//  2. Call domain processor (pure business logic)
//  3. Wrap result in BaseMessage for transport
//  4. Publish to output NATS subject
func (c *Component) handleMessage(ctx context.Context, msgData []byte) {
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()

	c.logger.Debug("Received message",
		"component", c.name,
		"size_bytes", len(msgData))

	// Parse incoming JSON into map
	var data map[string]any
	if err := json.Unmarshal(msgData, &data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Debug("Failed to parse message as JSON",
			"component", c.name,
			"error", err)
		return
	}

	// Use domain processor to transform data
	reading, err := c.processor.Process(data)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("Failed to process sensor data",
			"component", c.name,
			"error", err)
		return
	}

	// Wrap Graphable payload in BaseMessage for transport
	// Type format: domain.category.version → iot.sensor.v1
	baseMsg := message.NewBaseMessage(
		message.Type{
			Domain:   "iot",
			Category: "sensor",
			Version:  "v1",
		},
		reading, // SensorReading implements Graphable
		c.name,  // source component name
	)

	// Marshal the BaseMessage
	wrappedData, err := json.Marshal(baseMsg)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		c.logger.Error("Failed to marshal BaseMessage",
			"component", c.name,
			"error", err)
		return
	}

	atomic.AddInt64(&c.messagesWrapped, 1)

	c.logger.Debug("Message wrapped in BaseMessage with SensorReading payload",
		"component", c.name,
		"output_subject", c.outputSubj,
		"original_size", len(msgData),
		"wrapped_size", len(wrappedData),
		"entity_id", reading.EntityID())

	// Publish to output subject
	if c.outputSubj != "" {
		var publishErr error
		if c.isJetStreamPortBySubject(c.outputSubj) {
			publishErr = c.natsClient.PublishToStream(ctx, c.outputSubj, wrappedData)
		} else {
			publishErr = c.natsClient.Publish(ctx, c.outputSubj, wrappedData)
		}
		if publishErr != nil {
			atomic.AddInt64(&c.errors, 1)
			c.logger.Error("Failed to publish wrapped message",
				"component", c.name,
				"output_subject", c.outputSubj,
				"error", publishErr)
		} else {
			c.logger.Debug("Published wrapped message",
				"component", c.name,
				"output_subject", c.outputSubj)
		}
	}
}

// Discoverable interface implementation

// Meta returns metadata describing this processor component.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        c.name,
		Type:        "processor",
		Description: "Transforms incoming JSON sensor data into Graphable SensorReading payloads",
		Version:     "0.1.0",
	}
}

// InputPorts returns the NATS input ports this processor subscribes to.
func (c *Component) InputPorts() []component.Port {
	ports := make([]component.Port, len(c.subjects))
	for i, subj := range c.subjects {
		ports[i] = component.Port{
			Name:      fmt.Sprintf("input_%d", i),
			Direction: component.DirectionInput,
			Required:  true,
			Config: component.NATSPort{
				Subject: subj,
			},
		}
	}
	return ports
}

// OutputPorts returns the NATS output port for Graphable sensor readings.
func (c *Component) OutputPorts() []component.Port {
	if len(c.outputPorts) == 0 {
		return []component.Port{}
	}

	ports := make([]component.Port, 0, len(c.outputPorts))
	for _, def := range c.outputPorts {
		port := component.Port{
			Name:      def.Name,
			Direction: component.DirectionOutput,
			Required:  def.Required,
		}

		// Build appropriate port config based on type from config
		switch def.Type {
		case "jetstream":
			port.Config = component.JetStreamPort{
				Subjects:   []string{def.Subject},
				StreamName: def.StreamName,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		default:
			// Default to NATS port
			port.Config = component.NATSPort{
				Subject: def.Subject,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		}

		ports = append(ports, port)
	}

	return ports
}

// ConfigSchema returns the configuration schema for this processor.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return iotSensorSchema
}

// Health returns the current health status of this processor.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return component.HealthStatus{
		Healthy:    c.running,
		LastCheck:  time.Now(),
		ErrorCount: int(atomic.LoadInt64(&c.errors)),
		Uptime:     time.Since(c.startTime),
	}
}

// DataFlow returns current data flow metrics for this processor.
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	processed := atomic.LoadInt64(&c.messagesProcessed)
	errorCount := atomic.LoadInt64(&c.errors)

	var errorRate float64
	if processed > 0 {
		errorRate = float64(errorCount) / float64(processed)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0, // TODO: Calculate rate
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      c.lastActivity,
	}
}
