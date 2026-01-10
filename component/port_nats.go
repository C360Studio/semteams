package component

import "fmt"

// NATSPort - NATS pub/sub
type NATSPort struct {
	Subject   string             `json:"subject"`
	Queue     string             `json:"queue,omitempty"`
	Interface *InterfaceContract `json:"interface,omitempty"`
}

// NATSStreamPortConfig represents a NATS streaming port configuration
// Used for stream-based message delivery patterns
type NATSStreamPortConfig struct {
	Subject  string `json:"subject"`
	Consumer string `json:"consumer,omitempty"`
}

// ResourceID returns unique identifier for NATS stream ports
func (n NATSStreamPortConfig) ResourceID() string {
	return fmt.Sprintf("nats-stream:%s", n.Subject)
}

// IsExclusive returns false as multiple components can subscribe
func (n NATSStreamPortConfig) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (n NATSStreamPortConfig) Type() string {
	return "stream"
}

// NATSRequestPortConfig represents a NATS request/reply port configuration
// Type alias for NATSRequestPort for test compatibility
type NATSRequestPortConfig struct {
	Subject string `json:"subject"`
	Timeout string `json:"timeout,omitempty"`
}

// ResourceID returns unique identifier for NATS request ports
func (n NATSRequestPortConfig) ResourceID() string {
	return fmt.Sprintf("nats-request:%s", n.Subject)
}

// IsExclusive returns false as multiple components can handle requests
func (n NATSRequestPortConfig) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (n NATSRequestPortConfig) Type() string {
	return "request"
}

// ResourceID returns unique identifier for NATS ports
func (n NATSPort) ResourceID() string {
	return fmt.Sprintf("nats:%s", n.Subject)
}

// IsExclusive returns false as multiple components can subscribe
func (n NATSPort) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (n NATSPort) Type() string {
	return "nats"
}

// NATSRequestPort - NATS Request/Response pattern for synchronous operations
type NATSRequestPort struct {
	Subject   string             `json:"subject"`
	Timeout   string             `json:"timeout,omitempty"` // Duration string e.g. "1s", "500ms"
	Retries   int                `json:"retries,omitempty"`
	Interface *InterfaceContract `json:"interface,omitempty"`
}

// ResourceID returns unique identifier for NATS request ports
func (n NATSRequestPort) ResourceID() string {
	return fmt.Sprintf("nats-request:%s", n.Subject)
}

// IsExclusive returns false as multiple components can handle requests
func (n NATSRequestPort) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (n NATSRequestPort) Type() string {
	return "nats-request"
}
