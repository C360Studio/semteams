package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/pkg/errs"
)

// actionPublisher implements the Publisher interface for ActionExecutor.
// It wraps the Processor's NATS client and port configuration to provide
// transparent publishing to either core NATS or JetStream based on config.
type actionPublisher struct {
	processor *Processor
}

// newActionPublisher creates a new publisher that uses the processor's NATS client.
func newActionPublisher(processor *Processor) *actionPublisher {
	return &actionPublisher{processor: processor}
}

// Publish sends a message to a NATS subject.
// It checks port configuration to determine whether to use JetStream or core NATS.
func (p *actionPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	if p.processor.natsClient == nil {
		return errs.WrapFatal(fmt.Errorf("NATS client not configured"), "actionPublisher", "Publish", "client check")
	}

	var publishErr error
	if p.processor.isJetStreamPortBySubject(subject) {
		publishErr = p.processor.natsClient.PublishToStream(ctx, subject, data)
	} else {
		publishErr = p.processor.natsClient.Publish(ctx, subject, data)
	}

	if publishErr != nil {
		return errs.WrapTransient(publishErr, "actionPublisher", "Publish", fmt.Sprintf("publish to %s", subject))
	}

	// Update metrics
	if p.processor.metrics != nil {
		p.processor.metrics.eventsPublishedTotal.WithLabelValues(subject, "action_publish").Inc()
	}

	atomic.AddInt64(&p.processor.eventsPublished, 1)
	return nil
}

// isJetStreamPortBySubject checks if an output port with the given subject is configured for JetStream
func (rp *Processor) isJetStreamPortBySubject(subject string) bool {
	if rp.config == nil || rp.config.Ports == nil {
		return false
	}
	for _, port := range rp.config.Ports.Outputs {
		if port.Subject == subject {
			return port.Type == "jetstream"
		}
	}
	return false
}

// publishGraphEvents publishes a batch of graph events to appropriate NATS subjects
// Accepts generic Event interface, works with any event type that implements it
func (rp *Processor) publishGraphEvents(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}

	if !rp.config.EnableGraphIntegration {
		return nil // Graph integration disabled
	}

	for _, event := range events {
		// Validate event
		if err := event.Validate(); err != nil {
			return errs.WrapInvalid(err, "RuleProcessor", "publishEvent", "validate event")
		}

		// Get appropriate subject (e.g., "graph.events.entity.create")
		subject := event.Subject()

		// Marshal event
		data, err := json.Marshal(event)
		if err != nil {
			return errs.Wrap(err, "RuleProcessor", "publishEvent", "marshal event")
		}

		// Publish to NATS, respecting port type configuration
		var publishErr error
		if rp.isJetStreamPortBySubject(subject) {
			publishErr = rp.natsClient.PublishToStream(ctx, subject, data)
		} else {
			publishErr = rp.natsClient.Publish(ctx, subject, data)
		}
		if publishErr != nil {
			return errs.WrapTransient(publishErr, "RuleProcessor", "publishEvent", fmt.Sprintf("publish to %s", subject))
		}

		// Update metrics
		if rp.metrics != nil {
			rp.metrics.eventsPublishedTotal.WithLabelValues(subject, event.EventType()).Inc()
		}

		atomic.AddInt64(&rp.eventsPublished, 1)
	}

	return nil
}

// publishRuleEvent publishes a rule-specific event to the configured output port
func (rp *Processor) publishRuleEvent(ctx context.Context, ruleName, eventType string) error {

	// Create rule event payload
	ruleEvent := map[string]any{
		"rule_name":  ruleName,
		"event_type": eventType,
		"timestamp":  time.Now().Format(time.RFC3339),
		"processor":  "rule-processor",
		"action":     eventType, // For backward compatibility with tests
	}

	// Marshal event
	data, err := json.Marshal(ruleEvent)
	if err != nil {
		return errs.Wrap(err, "RuleProcessor", "publishRuleEvent", "marshal rule event")
	}

	// Use configured output port subject, fallback to rule-specific subject
	subject := "events.rule.triggered" // Default subject
	if rp.config != nil && rp.config.Ports != nil {
		for _, port := range rp.config.Ports.Outputs {
			if port.Name == "rule_events" && port.Subject != "" {
				subject = port.Subject
				break
			}
		}
	}

	// Publish to NATS, respecting port type configuration
	var publishErr error
	if rp.isJetStreamPortBySubject(subject) {
		publishErr = rp.natsClient.PublishToStream(ctx, subject, data)
	} else {
		publishErr = rp.natsClient.Publish(ctx, subject, data)
	}
	if publishErr != nil {
		return errs.WrapTransient(publishErr, "RuleProcessor", "publishRuleEvent", fmt.Sprintf("publish to %s", subject))
	}

	// Update metrics
	if rp.metrics != nil {
		rp.metrics.eventsPublishedTotal.WithLabelValues(subject, eventType).Inc()
	}

	atomic.AddInt64(&rp.eventsPublished, 1)
	return nil
}
