package agenticloop

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/rule/boid"
	"github.com/nats-io/nats.go/jetstream"
)

// BoidHandler manages agent position tracking and steering signal processing
// for Boids-style local coordination rules.
type BoidHandler struct {
	positionsBucket jetstream.KeyValue
	logger          *slog.Logger

	// Entity ID extraction patterns
	entityIDPattern *regexp.Regexp
}

// NewBoidHandler creates a new boid handler.
func NewBoidHandler(positionsBucket jetstream.KeyValue, logger *slog.Logger) *BoidHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BoidHandler{
		positionsBucket: positionsBucket,
		logger:          logger,
		// Pattern to extract entity IDs from content (6-part federated IDs)
		entityIDPattern: regexp.MustCompile(`([a-z0-9_-]+\.){5}[a-z0-9_-]+`),
	}
}

// UpdatePosition updates an agent's position in the AGENT_POSITIONS KV bucket.
func (h *BoidHandler) UpdatePosition(ctx context.Context, pos *boid.AgentPosition) error {
	if h.positionsBucket == nil {
		h.logger.Debug("Positions bucket not configured, skipping position update")
		return nil
	}

	pos.LastUpdate = time.Now()

	data, err := json.Marshal(pos)
	if err != nil {
		return err
	}

	_, err = h.positionsBucket.Put(ctx, pos.LoopID, data)
	if err != nil {
		h.logger.Error("Failed to update position",
			"loop_id", pos.LoopID,
			"error", err)
		return err
	}

	h.logger.Debug("Position updated",
		"loop_id", pos.LoopID,
		"role", pos.Role,
		"focus_count", len(pos.FocusEntities),
		"velocity", pos.Velocity)

	return nil
}

// GetPosition retrieves an agent's position from the KV bucket.
func (h *BoidHandler) GetPosition(ctx context.Context, loopID string) (*boid.AgentPosition, error) {
	if h.positionsBucket == nil {
		return nil, nil
	}

	entry, err := h.positionsBucket.Get(ctx, loopID)
	if err != nil {
		return nil, err
	}

	var pos boid.AgentPosition
	if err := json.Unmarshal(entry.Value(), &pos); err != nil {
		return nil, err
	}

	return &pos, nil
}

// DeletePosition removes an agent's position when the loop completes.
func (h *BoidHandler) DeletePosition(ctx context.Context, loopID string) error {
	if h.positionsBucket == nil {
		return nil
	}

	if err := h.positionsBucket.Delete(ctx, loopID); err != nil {
		// Ignore "key not found" errors
		if !strings.Contains(err.Error(), "key not found") {
			h.logger.Warn("Failed to delete position",
				"loop_id", loopID,
				"error", err)
			return err
		}
	}

	h.logger.Debug("Position deleted", "loop_id", loopID)
	return nil
}

// ExtractEntitiesFromToolResult extracts entity IDs from tool result content.
// This captures which entities an agent accessed during tool execution.
func (h *BoidHandler) ExtractEntitiesFromToolResult(content string) []string {
	if content == "" {
		return nil
	}

	matches := h.entityIDPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}

	return result
}

// ExtractEntitiesFromContext extracts entity IDs from context messages.
// Looks for "[Entity: {id}]" prefixes added by AddGraphEntityContext.
func (h *BoidHandler) ExtractEntitiesFromContext(content string) []string {
	if content == "" {
		return nil
	}

	// Pattern for "[Entity: {id}]" format
	pattern := regexp.MustCompile(`\[Entity:\s*([^\]]+)\]`)
	matches := pattern.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return nil
	}

	result := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, strings.TrimSpace(m[1]))
		}
	}

	return result
}

// ProcessSteeringSignal processes an incoming boid steering signal
// and applies it to the agent's context.
func (h *BoidHandler) ProcessSteeringSignal(_ context.Context, signal *boid.SteeringSignal, cm *ContextManager) error {
	if cm == nil || signal == nil {
		return nil
	}

	h.logger.Info("Processing steering signal",
		"loop_id", signal.LoopID,
		"signal_type", signal.SignalType,
		"strength", signal.Strength)

	switch signal.SignalType {
	case boid.SignalTypeSeparation:
		// For separation signals, we could deprioritize certain entities
		// This is handled via context slicing preferences
		h.logger.Debug("Separation signal received",
			"avoid_count", len(signal.AvoidEntities))

	case boid.SignalTypeCohesion:
		// For cohesion signals, we could add suggested entities to context
		if len(signal.SuggestedFocus) > 0 {
			h.logger.Debug("Cohesion signal received",
				"suggested_count", len(signal.SuggestedFocus))
		}

	case boid.SignalTypeAlignment:
		// For alignment signals, we note the suggested traversal patterns
		if len(signal.AlignWith) > 0 {
			h.logger.Debug("Alignment signal received",
				"align_patterns", len(signal.AlignWith))
		}
	}

	return nil
}

// HandleSteeringSignalMessage handles incoming boid steering signal messages from NATS.
func (h *BoidHandler) HandleSteeringSignalMessage(data []byte, getContextManager func(loopID string) *ContextManager) {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		h.logger.Error("Failed to unmarshal steering signal", "error", err)
		return
	}

	signalPtr, ok := baseMsg.Payload().(*boid.SteeringSignal)
	if !ok {
		h.logger.Warn("Unexpected payload type for steering signal",
			"type_got", baseMsg.Type().String())
		return
	}

	signal := *signalPtr
	cm := getContextManager(signal.LoopID)

	if err := h.ProcessSteeringSignal(context.Background(), &signal, cm); err != nil {
		h.logger.Error("Failed to process steering signal",
			"loop_id", signal.LoopID,
			"error", err)
	}
}

// CalculateVelocity computes velocity based on position changes.
func (h *BoidHandler) CalculateVelocity(oldFocus, newFocus []string) float64 {
	return boid.CalculateVelocity(oldFocus, newFocus)
}
