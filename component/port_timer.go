package component

import "fmt"

// TimerPort represents a periodic timer trigger port
type TimerPort struct {
	Interval  string             `json:"interval"` // Duration string e.g. "30s", "1m"
	Interface *InterfaceContract `json:"interface,omitempty"`
}

// ResourceID returns unique identifier for timer ports
func (t TimerPort) ResourceID() string {
	return fmt.Sprintf("timer:%s", t.Interval)
}

// IsExclusive returns false as multiple timers can run independently
func (t TimerPort) IsExclusive() bool {
	return false
}

// Type returns the port type identifier
func (t TimerPort) Type() string {
	return "timer"
}
