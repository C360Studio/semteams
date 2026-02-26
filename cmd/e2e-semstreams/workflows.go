package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/reactive"
	"github.com/c360studio/semstreams/processor/rule"
)

// registerE2EWorkflows registers test workflows that mirror the old inline_rules.
// These workflows watch ENTITY_STATES and fire based on sensor readings,
// producing the same metrics that e2e tests validate.
func registerE2EWorkflows(engine *reactive.Engine) error {
	workflows := []*reactive.Definition{
		buildColdStorageAlertWorkflow(),
		buildHighHumidityAlertWorkflow(),
		buildLowPressureAlertWorkflow(),
		buildNotifyTechnicianWorkflow(),
	}

	for _, wf := range workflows {
		if err := engine.RegisterWorkflow(wf); err != nil {
			return err
		}
	}

	return nil
}

// AlertState is the state type for all alert workflows.
// It embeds ExecutionState and captures the entity that triggered the alert.
type AlertState struct {
	reactive.ExecutionState
	EntityID  string `json:"entity_id"`
	AlertType string `json:"alert_type"`
	Severity  string `json:"severity"`
}

// GetExecutionState implements reactive.StateAccessor to avoid reflection in hotpath.
func (s *AlertState) GetExecutionState() *reactive.ExecutionState {
	return &s.ExecutionState
}

// AlertPayload is published when an alert fires.
type AlertPayload struct {
	EntityID  string    `json:"entity_id"`
	AlertType string    `json:"alert_type"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// Schema returns the message type for alert payloads.
func (p *AlertPayload) Schema() message.Type {
	return message.Type{Domain: "alerts", Category: "sensor", Version: "v1"}
}

// Validate checks the alert payload for required fields (currently accepts all).
func (p *AlertPayload) Validate() error { return nil }

// MarshalJSON serializes the alert payload to JSON.
func (p *AlertPayload) MarshalJSON() ([]byte, error) {
	type Alias AlertPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON deserializes the alert payload from JSON.
func (p *AlertPayload) UnmarshalJSON(data []byte) error {
	type Alias AlertPayload
	return json.Unmarshal(data, (*Alias)(p))
}

// buildColdStorageAlertWorkflow creates a workflow that fires when:
// - sensor.measurement.fahrenheit >= 40.0
// - geo.location.zone contains "cold-storage"
func buildColdStorageAlertWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("cold-storage-alert").
		WithDescription("Cold storage temperature monitoring - fires when temp >= 40F in cold-storage zones").
		WithStateBucket("REACTIVE_WORKFLOW_STATE").
		WithStateFactory(func() any { return &AlertState{} }).
		WithTimeout(10 * time.Minute).
		AddRule(reactive.NewRule("check-cold-storage-temp").
			WatchKV("ENTITY_STATES", "c360.>").
			When("temp >= 40F", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				temp, found := entity.GetPropertyValue("sensor.measurement.fahrenheit")
				if !found {
					return false
				}
				tempFloat, ok := toFloat64(temp)
				if !ok {
					return false
				}
				return tempFloat >= 40.0
			}).
			When("zone is cold-storage", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				zone, found := entity.GetPropertyValue("geo.location.zone")
				if !found {
					return false
				}
				zoneStr, ok := zone.(string)
				if !ok {
					return false
				}
				return strings.Contains(zoneStr, "cold-storage")
			}).
			Publish("alerts.cold-storage", func(ctx *reactive.RuleContext) (message.Payload, error) {
				entity, _ := ctx.State.(*graph.EntityState)
				return &AlertPayload{
					EntityID:  entity.ID,
					AlertType: "cold-storage-violation",
					Severity:  "critical",
					Timestamp: time.Now(),
					Message:   "Cold storage temperature exceeded safe threshold (40F)",
				}, nil
			}).
			WithCooldown(5 * time.Second).
			MustBuild()).
		MustBuild()
}

// buildHighHumidityAlertWorkflow creates a workflow that fires when:
// - sensor.measurement.percent >= 50.0
// - sensor.classification.type == "humidity"
func buildHighHumidityAlertWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("high-humidity-alert").
		WithDescription("High humidity monitoring - fires when humidity >= 50%").
		WithStateBucket("REACTIVE_WORKFLOW_STATE").
		WithStateFactory(func() any { return &AlertState{} }).
		WithTimeout(10 * time.Minute).
		AddRule(reactive.NewRule("check-humidity").
			WatchKV("ENTITY_STATES", "c360.>").
			When("humidity >= 50%", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				humidity, found := entity.GetPropertyValue("sensor.measurement.percent")
				if !found {
					return false
				}
				humidityFloat, ok := toFloat64(humidity)
				if !ok {
					return false
				}
				return humidityFloat >= 50.0
			}).
			When("type is humidity", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				sensorType, found := entity.GetPropertyValue("sensor.classification.type")
				if !found {
					return false
				}
				return sensorType == "humidity"
			}).
			Publish("alerts.humidity", func(ctx *reactive.RuleContext) (message.Payload, error) {
				entity, _ := ctx.State.(*graph.EntityState)
				return &AlertPayload{
					EntityID:  entity.ID,
					AlertType: "high-humidity-warning",
					Severity:  "warning",
					Timestamp: time.Now(),
					Message:   "Humidity exceeded threshold (50%)",
				}, nil
			}).
			WithCooldown(5 * time.Second).
			MustBuild()).
		MustBuild()
}

// buildLowPressureAlertWorkflow creates a workflow that fires when:
// - sensor.measurement.psi < 100.0
// - sensor.classification.type == "pressure"
func buildLowPressureAlertWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("low-pressure-alert").
		WithDescription("Low pressure monitoring - fires when pressure < 100 PSI").
		WithStateBucket("REACTIVE_WORKFLOW_STATE").
		WithStateFactory(func() any { return &AlertState{} }).
		WithTimeout(10 * time.Minute).
		AddRule(reactive.NewRule("check-pressure").
			WatchKV("ENTITY_STATES", "c360.>").
			When("pressure < 100 PSI", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				pressure, found := entity.GetPropertyValue("sensor.measurement.psi")
				if !found {
					return false
				}
				pressureFloat, ok := toFloat64(pressure)
				if !ok {
					return false
				}
				return pressureFloat < 100.0
			}).
			When("type is pressure", func(ctx *reactive.RuleContext) bool {
				entity, ok := ctx.State.(*graph.EntityState)
				if !ok {
					return false
				}
				sensorType, found := entity.GetPropertyValue("sensor.classification.type")
				if !found {
					return false
				}
				return sensorType == "pressure"
			}).
			Publish("alerts.pressure", func(ctx *reactive.RuleContext) (message.Payload, error) {
				entity, _ := ctx.State.(*graph.EntityState)
				return &AlertPayload{
					EntityID:  entity.ID,
					AlertType: "low-pressure-warning",
					Severity:  "warning",
					Timestamp: time.Now(),
					Message:   "Compressed air pressure dropped below threshold (100 PSI)",
				}, nil
			}).
			WithCooldown(5 * time.Second).
			MustBuild()).
		MustBuild()
}

// TechAlertState is the state type for the notify-technician workflow.
// It captures the entity that triggered the rule.
type TechAlertState struct {
	reactive.ExecutionState
	EntityID string `json:"entity_id"`
}

// GetExecutionState implements reactive.StateAccessor to avoid reflection in hotpath.
func (s *TechAlertState) GetExecutionState() *reactive.ExecutionState {
	return &s.ExecutionState
}

// buildNotifyTechnicianWorkflow creates a workflow that responds to rule triggers.
// This demonstrates the rule → workflow integration: the rule processor triggers
// this workflow when conditions are met, and the workflow handles the orchestrated response.
// Uses the registered rule.WorkflowTriggerPayload type for proper BaseMessage deserialization.
func buildNotifyTechnicianWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("notify-technician").
		WithDescription("Sends alert to technician when triggered by rules").
		WithStateBucket("REACTIVE_WORKFLOW_STATE").
		WithStateFactory(func() any { return &TechAlertState{} }).
		WithTimeout(1 * time.Minute).
		AddRule(reactive.NewRule("handle-trigger").
			OnJetStreamSubject("WORKFLOW", "workflow.trigger.notify-technician", func() any { return &rule.WorkflowTriggerPayload{} }).
			When("new trigger", func(ctx *reactive.RuleContext) bool {
				// Always handle incoming triggers - the rule processor already validated conditions
				return ctx.Message != nil
			}).
			Publish("alerts.technician", func(ctx *reactive.RuleContext) (message.Payload, error) {
				// Extract entity ID from trigger message
				entityID := ""
				if trigger, ok := ctx.Message.(*rule.WorkflowTriggerPayload); ok {
					entityID = trigger.EntityID
				}
				return &AlertPayload{
					EntityID:  entityID,
					AlertType: "temperature-violation",
					Severity:  "warning",
					Timestamp: time.Now(),
					Message:   "Cold storage temperature exceeded 40F threshold",
				}, nil
			}).
			Complete().
			MustBuild()).
		MustBuild()
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
