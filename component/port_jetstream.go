package component

import "fmt"

// JetStreamPort - NATS JetStream for durable, at-least-once messaging
type JetStreamPort struct {
	// Stream configuration (for outputs)
	StreamName      string   `json:"stream_name"`              // e.g., "ENTITY_EVENTS"
	Subjects        []string `json:"subjects"`                 // e.g., ["events.graph.entity.>"]
	Storage         string   `json:"storage,omitempty"`        // "file" or "memory", default "file"
	RetentionPolicy string   `json:"retention,omitempty"`      // "limits", "interest", "work_queue", default "limits"
	RetentionDays   int      `json:"retention_days,omitempty"` // Message retention in days, default 7
	MaxSizeGB       int      `json:"max_size_gb,omitempty"`    // Max stream size in GB, default 10
	Replicas        int      `json:"replicas,omitempty"`       // Number of replicas, default 1

	// Consumer configuration (for inputs)
	ConsumerName  string `json:"consumer_name,omitempty"`  // Durable consumer name
	DeliverPolicy string `json:"deliver_policy,omitempty"` // "all", "last", "new", default "new"
	AckPolicy     string `json:"ack_policy,omitempty"`     // "explicit", "none", "all", default "explicit"
	MaxDeliver    int    `json:"max_deliver,omitempty"`    // Max redelivery attempts, default 3

	// Interface contract
	Interface *InterfaceContract `json:"interface,omitempty"`
}

// ResourceID returns unique identifier for JetStream ports
func (j JetStreamPort) ResourceID() string {
	if j.StreamName != "" {
		return fmt.Sprintf("jetstream:%s", j.StreamName)
	}
	// For consumers without explicit stream name
	if len(j.Subjects) > 0 {
		return fmt.Sprintf("jetstream:%s", j.Subjects[0])
	}
	return "jetstream:unknown"
}

// IsExclusive returns false as JetStream manages consumer coordination
func (j JetStreamPort) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (j JetStreamPort) Type() string {
	return "jetstream"
}

// ConsumerConfig holds extracted JetStream consumer configuration.
type ConsumerConfig struct {
	DeliverPolicy string
	AckPolicy     string
	MaxDeliver    int
}

// GetConsumerConfig extracts JetStream consumer configuration from a port.
// Returns safe defaults if port doesn't have JetStream config:
// - DeliverPolicy: "new" (safe default - don't replay historical messages)
// - AckPolicy: "explicit"
// - MaxDeliver: 3
func GetConsumerConfig(port Port) ConsumerConfig {
	cfg := ConsumerConfig{
		DeliverPolicy: "new",     // Safe default
		AckPolicy:     "explicit",
		MaxDeliver:    3,
	}

	if jsPort, ok := port.Config.(JetStreamPort); ok {
		if jsPort.DeliverPolicy != "" {
			cfg.DeliverPolicy = jsPort.DeliverPolicy
		}
		if jsPort.AckPolicy != "" {
			cfg.AckPolicy = jsPort.AckPolicy
		}
		if jsPort.MaxDeliver > 0 {
			cfg.MaxDeliver = jsPort.MaxDeliver
		}
	}
	return cfg
}

// GetConsumerConfigFromDefinition extracts JetStream consumer configuration from a port definition.
// This is a convenience wrapper for use with PortDefinition instead of Port.
func GetConsumerConfigFromDefinition(portDef PortDefinition) ConsumerConfig {
	cfg := ConsumerConfig{
		DeliverPolicy: "new",     // Safe default
		AckPolicy:     "explicit",
		MaxDeliver:    3,
	}

	if jsPort, ok := portDef.Config.(JetStreamPort); ok {
		if jsPort.DeliverPolicy != "" {
			cfg.DeliverPolicy = jsPort.DeliverPolicy
		}
		if jsPort.AckPolicy != "" {
			cfg.AckPolicy = jsPort.AckPolicy
		}
		if jsPort.MaxDeliver > 0 {
			cfg.MaxDeliver = jsPort.MaxDeliver
		}
	}
	return cfg
}
