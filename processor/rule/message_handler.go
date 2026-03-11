package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// reportEvaluating reports the evaluating stage (throttled to avoid KV spam)
func (rp *Processor) reportEvaluating(ctx context.Context) {
	if rp.lifecycleReporter != nil {
		if err := rp.lifecycleReporter.ReportStage(ctx, "evaluating"); err != nil {
			rp.logger.Debug("failed to report lifecycle stage", slog.String("stage", "evaluating"), slog.Any("error", err))
		}
	}
}

// handleMessage processes incoming NATS messages with dual-format support
func (rp *Processor) handleMessage(ctx context.Context, subject string, data []byte) {
	// Report evaluating stage for lifecycle observability
	rp.reportEvaluating(ctx)

	// Update metrics for received messages
	if rp.metrics != nil {
		rp.metrics.messagesReceived.WithLabelValues(subject).Inc()
	}

	rp.logger.Debug("Received message", "subject", subject)

	rp.mu.Lock()
	rp.lastActivity = time.Now()
	rp.mu.Unlock()

	// All messages are now semantic messages since entity events come via KV watch
	rp.handleSemanticMessage(ctx, subject, data)
}

// handleSemanticMessage processes semantic messages (BaseMessage format)
func (rp *Processor) handleSemanticMessage(ctx context.Context, subject string, data []byte) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		rp.recordError(fmt.Sprintf("failed to unmarshal BaseMessage: %v", err))
		return
	}

	rp.logger.Debug("Successfully unmarshaled BaseMessage", "type", baseMsg.Type().String())

	// Process through rules
	rp.evaluateRulesForMessage(ctx, subject, &baseMsg)
}

// evaluateRulesForMessage performs rule evaluation for any message type
func (rp *Processor) evaluateRulesForMessage(ctx context.Context, subject string, msg message.Message) {
	// Increment evaluation counter for all messages (NATS and KV watcher)
	atomic.AddInt64(&rp.messagesEvaluated, 1)

	// Cache the message if needed
	if rp.messageCache != nil {
		cacheKey := fmt.Sprintf("%s_%d", subject, time.Now().UnixNano())
		rp.messageCache.Set(cacheKey, msg)
	}

	// Process through each rule
	for ruleName, ruleInstance := range rp.rules {
		// Check if rule is interested in this subject
		if !rp.matchesRuleSubject(ruleInstance, subject) {
			continue
		}

		// TODO: Time-window buffering removed - use pkg/buffer if needed for aggregation rules
		// For now, evaluate rules on single messages
		messages := []message.Message{msg}

		// Evaluate rule with metrics timing
		rp.logger.Debug("Evaluating rule", "rule_name", ruleName)
		start := time.Now()
		triggered := ruleInstance.Evaluate(messages)
		evaluationDuration := time.Since(start)

		// Update metrics
		if rp.metrics != nil {
			rp.metrics.evaluationDuration.WithLabelValues(ruleName).Observe(evaluationDuration.Seconds())
			if triggered {
				rp.metrics.evaluationsTotal.WithLabelValues(ruleName, "triggered").Inc()
				// For severity, we'll use a default "info" since we don't have severity in the interface
				rp.metrics.triggersTotal.WithLabelValues(ruleName, "info").Inc()
			} else {
				rp.metrics.evaluationsTotal.WithLabelValues(ruleName, "not_triggered").Inc()
			}
		}

		// Get rule definition for stateful evaluation
		ruleDef, hasDefinition := rp.ruleDefinitions[ruleName]
		hasStatefulActions := hasDefinition && (len(ruleDef.OnEnter) > 0 || len(ruleDef.OnExit) > 0 || len(ruleDef.WhileTrue) > 0)

		// Handle stateful evaluation if rule has OnEnter/OnExit/WhileTrue actions
		if hasStatefulActions && rp.statefulEvaluator != nil {
			// Extract entity ID from message payload for state tracking
			entityID := extractEntityID(msg)

			// Perform stateful evaluation
			transition, err := rp.statefulEvaluator.EvaluateWithState(ctx, ruleDef, entityID, "", triggered)
			if err != nil {
				rp.logger.Warn("Stateful evaluation failed", "rule_name", ruleName, "error", err)
			} else if transition != TransitionNone {
				rp.logger.Debug("Rule state transition",
					"rule_name", ruleName,
					"transition", transition,
					"entity_id", entityID)

				// Update state transition metrics
				if rp.metrics != nil {
					rp.metrics.stateTransitionsTotal.WithLabelValues(ruleName, string(transition)).Inc()
				}
			}
		}

		if triggered {
			rp.logger.Debug("Rule triggered", "rule_name", ruleName)

			// Execute rule events
			events, err := ruleInstance.ExecuteEvents(messages)
			if err != nil {
				rp.recordError(fmt.Sprintf("rule %s execution failed: %v", ruleName, err))
				continue
			}

			// Publish rule event notification
			if err := rp.publishRuleEvent(ctx, ruleName, "triggered"); err != nil {
				rp.logger.Warn("Failed to publish rule event", "error", err)
			}

			// Publish graph events
			if err := rp.publishGraphEvents(ctx, events); err != nil {
				rp.recordError(fmt.Sprintf("failed to publish events from rule %s: %v", ruleName, err))
			} else {
				atomic.AddInt64(&rp.rulesTriggered, 1)
			}
		} else {
			rp.logger.Debug("Rule did not trigger", "rule_name", ruleName)
		}
	}
}

// evaluateRulesForEntityState performs rule evaluation directly against EntityState triples.
// This bypasses the message transformation layer for more efficient and direct evaluation.
func (rp *Processor) evaluateRulesForEntityState(ctx context.Context, entityKey, action string, entityState *gtypes.EntityState) {
	// Skip evaluation for deleted entities
	if entityState == nil {
		rp.logger.Debug("Skipping rule evaluation for deleted entity", "entity_key", entityKey)
		return
	}

	// Increment evaluation counter
	atomic.AddInt64(&rp.messagesEvaluated, 1)

	// Process through each rule
	for ruleName, ruleInstance := range rp.rules {
		// Evaluate rule with metrics timing
		rp.logger.Debug("Evaluating rule against EntityState",
			"rule_name", ruleName,
			"entity_id", entityState.ID,
			"action", action)

		start := time.Now()
		var triggered bool

		// Try direct EntityState evaluation first (preferred path)
		if entityEval, ok := ruleInstance.(EntityStateEvaluator); ok {
			triggered = entityEval.EvaluateEntityState(entityState)
		} else {
			// Fallback to message-based evaluation for rules that don't support EntityState
			rp.logger.Debug("Rule doesn't support EntityState evaluation, skipping",
				"rule_name", ruleName)
			continue
		}

		evaluationDuration := time.Since(start)

		// Update metrics
		if rp.metrics != nil {
			rp.metrics.evaluationDuration.WithLabelValues(ruleName).Observe(evaluationDuration.Seconds())
			if triggered {
				rp.metrics.evaluationsTotal.WithLabelValues(ruleName, "triggered").Inc()
				rp.metrics.triggersTotal.WithLabelValues(ruleName, "info").Inc()
			} else {
				rp.metrics.evaluationsTotal.WithLabelValues(ruleName, "not_triggered").Inc()
			}
		}

		// Get rule definition for stateful evaluation
		ruleDef, hasDefinition := rp.ruleDefinitions[ruleName]
		hasStatefulActions := hasDefinition && (len(ruleDef.OnEnter) > 0 || len(ruleDef.OnExit) > 0 || len(ruleDef.WhileTrue) > 0)

		// Handle stateful evaluation if rule has OnEnter/OnExit/WhileTrue actions
		if hasStatefulActions && rp.statefulEvaluator != nil {
			// Use EntityState ID directly for state tracking
			entityID := entityState.ID

			// Perform stateful evaluation
			transition, err := rp.statefulEvaluator.EvaluateWithState(ctx, ruleDef, entityID, "", triggered)
			if err != nil {
				rp.logger.Warn("Stateful evaluation failed", "rule_name", ruleName, "error", err)
			} else if transition != TransitionNone {
				rp.logger.Debug("Rule state transition",
					"rule_name", ruleName,
					"transition", transition,
					"entity_id", entityID)

				// Update state transition metrics
				if rp.metrics != nil {
					rp.metrics.stateTransitionsTotal.WithLabelValues(ruleName, string(transition)).Inc()
				}
			}
		}

		if triggered {
			rp.logger.Debug("Rule triggered from EntityState",
				"rule_name", ruleName,
				"entity_id", entityState.ID)

			// For EntityState-based rules, we create a minimal message wrapper for event execution
			// This maintains compatibility with the existing ExecuteEvents interface
			msg := rp.entityStateToMinimalMessage(entityState)
			messages := []message.Message{msg}

			// Execute rule events
			events, err := ruleInstance.ExecuteEvents(messages)
			if err != nil {
				rp.recordError(fmt.Sprintf("rule %s execution failed: %v", ruleName, err))
				continue
			}

			// Publish rule event notification
			if err := rp.publishRuleEvent(ctx, ruleName, "triggered"); err != nil {
				rp.logger.Warn("Failed to publish rule event", "error", err)
			}

			// Publish graph events
			if err := rp.publishGraphEvents(ctx, events); err != nil {
				rp.recordError(fmt.Sprintf("failed to publish events from rule %s: %v", ruleName, err))
			} else {
				atomic.AddInt64(&rp.rulesTriggered, 1)
			}
		} else {
			rp.logger.Debug("Rule did not trigger", "rule_name", ruleName)
		}
	}
}

// entityStateToMinimalMessage creates a minimal message wrapper for ExecuteEvents compatibility
func (rp *Processor) entityStateToMinimalMessage(entityState *gtypes.EntityState) message.Message {
	msgType := message.Type{
		Domain:   "entity",
		Category: "state",
		Version:  "v1",
	}

	payloadData := map[string]any{
		"entity_id":  entityState.ID,
		"timestamp":  time.Now(),
		"source":     "kv-watch",
		"version":    entityState.Version,
		"updated_at": entityState.UpdatedAt,
	}

	payload := message.NewGenericJSON(payloadData)
	return message.NewBaseMessage(msgType, payload, "kv-watch")
}

// matchesRuleSubject checks if a NATS subject matches the rule's subscription pattern
func (rp *Processor) matchesRuleSubject(r Rule, subject string) bool {
	ruleSubjects := r.Subscribe()

	// Check against all rule subscription patterns
	for _, ruleSubject := range ruleSubjects {
		// Simple wildcard matching - in production, use proper NATS subject matching
		if ruleSubject == ">" || ruleSubject == subject {
			return true
		}

		// Handle basic wildcard patterns like "process.robotics.>"
		if len(ruleSubject) > 2 && ruleSubject[len(ruleSubject)-2:] == ".>" {
			prefix := ruleSubject[:len(ruleSubject)-2]
			if len(subject) >= len(prefix) && subject[:len(prefix)] == prefix {
				return true
			}
		}
	}

	return false
}

// extractEntityID extracts the entity ID from a message for state tracking
func extractEntityID(msg message.Message) string {
	// Try to get entity_id from payload data
	if payload := msg.Payload(); payload != nil {
		if genericPayload, ok := payload.(*message.GenericJSONPayload); ok {
			if entityID, exists := genericPayload.Data["entity_id"]; exists {
				if id, ok := entityID.(string); ok {
					return id
				}
			}
		}
	}

	// Fallback to message ID if no entity_id in payload
	return msg.ID()
}

// recordError records an error and updates health status
func (rp *Processor) recordError(errorMsg string) {
	atomic.AddInt64(&rp.errorCount, 1)

	// Update metrics - try to extract rule name and error type from error message
	if rp.metrics != nil {
		ruleName := "unknown"
		errorType := "generic"

		// Try to extract rule name from error message patterns
		if strings.Contains(errorMsg, "rule ") {
			// Extract rule name between "rule " and next space or punctuation
			parts := strings.Split(errorMsg, "rule ")
			if len(parts) > 1 {
				ruleNamePart := strings.Fields(parts[1])
				if len(ruleNamePart) > 0 {
					ruleName = ruleNamePart[0]
				}
			}
		}

		// Categorize error type
		if strings.Contains(errorMsg, "unmarshal") || strings.Contains(errorMsg, "marshal") {
			errorType = "serialization"
		} else if strings.Contains(errorMsg, "publish") {
			errorType = "publishing"
		} else if strings.Contains(errorMsg, "execution") || strings.Contains(errorMsg, "evaluate") {
			errorType = "rule_execution"
		} else if strings.Contains(errorMsg, "validation") || strings.Contains(errorMsg, "validate") {
			errorType = "validation"
		}

		rp.metrics.errorsTotal.WithLabelValues(ruleName, errorType).Inc()
	}

	rp.mu.Lock()
	rp.lastError = errorMsg
	rp.health.LastError = errorMsg
	rp.mu.Unlock()

	rp.logger.Error("Rule processor error", "error", errorMsg)
}
