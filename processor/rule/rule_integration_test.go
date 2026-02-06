//go:build integration

package rule_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/component"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/processor/rule/expression"
)

// getTestNATSClient creates a NATS client for integration tests
func getTestNATSClient(t *testing.T) *natsclient.Client {
	// Create test client for this test
	// Build tag ensures this only runs with -tags=integration
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		testClient.Terminate()
	})

	return testClient.Client
}

// createSemanticMessage creates a test message for testing
func createSemanticMessage(data map[string]any) ([]byte, error) {
	// Create a GenericJSON payload
	payload := message.NewGenericJSON(data)

	// Create a BaseMessage with the payload
	msgType := message.Type{
		Domain:   "core",
		Category: "json",
		Version:  "v1",
	}

	msg := message.NewBaseMessage(msgType, payload, "test-integration")
	return json.Marshal(msg)
}

// TestIntegration_KVEntityStateWatch tests KV entity state watching and rule triggering
func TestIntegration_KVEntityStateWatch(t *testing.T) {
	natsClient := getTestNATSClient(t)
	ctx := context.Background()

	// Create ENTITY_STATES KV bucket if it doesn't exist
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	kv, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		// Bucket doesn't exist, create it
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: "ENTITY_STATES",
		})
		require.NoError(t, err)
	}

	// Create a rule that triggers on battery level changes
	// NOTE: Field must match the FULL triple predicate path
	ruleDef := rule.Definition{
		ID:   "battery_low_watch",
		Type: "test_rule",
		Name: "Battery Low Watcher",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "robotics.battery.level", // Must match full triple predicate
				Operator: "lte",
				Value:    25.0,
				Required: true,
			},
		},
		Logic:   "and",
		Enabled: true,
		Entity: rule.EntityConfig{
			Pattern: "c360.platform1.test.>",
		},
	}

	// Create rule processor config
	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{
				Name:      "entity_events",
				Type:      "nats",
				Subject:   "events.graph.entity.>",
				Interface: "core.entity.v1",
				Required:  true,
			},
		},
		Outputs: []component.PortDefinition{
			{
				Name:      "rule_events",
				Type:      "nats",
				Subject:   "events.rule.triggered",
				Interface: "core.rule.v1",
				Required:  true,
			},
		},
	}
	config.InlineRules = []rule.Definition{ruleDef}
	config.EntityWatchPatterns = []string{"c360.platform1.test.>"}
	config.EnableGraphIntegration = false // We're testing KV watch, not graph integration

	// Create processor with metrics
	metricsRegistry := metric.NewMetricsRegistry()
	processor, err := rule.NewProcessorWithMetrics(natsClient, &config, metricsRegistry)
	require.NoError(t, err)
	require.NotNil(t, processor)

	// Initialize and start
	err = processor.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = processor.Start(testCtx)
	require.NoError(t, err)
	defer processor.Stop(5 * time.Second)

	// Give processor time to set up watchers
	time.Sleep(200 * time.Millisecond)

	// Subscribe to rule events
	receivedEvents := make([]map[string]any, 0)
	var receiveMu sync.Mutex

	_, err = natsClient.Subscribe(testCtx, "events.rule.triggered", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			receiveMu.Lock()
			receivedEvents = append(receivedEvents, event)
			receiveMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Update entity state in KV bucket (simulating graph processor update)
	entityID := "c360.platform1.test.drone.drone1"
	entityState := gtypes.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{
				Subject:   entityID,
				Predicate: "robotics.battery.level",
				Object:    20.0, // Below threshold of 25
			},
			{
				Subject:   entityID,
				Predicate: "robotics.battery.voltage",
				Object:    3.2,
			},
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}

	entityJSON, err := json.Marshal(entityState)
	require.NoError(t, err)

	_, err = kv.Put(ctx, entityID, entityJSON)
	require.NoError(t, err)

	// Wait for watcher to process and rule to evaluate
	time.Sleep(500 * time.Millisecond)

	// Verify rule was triggered
	receiveMu.Lock()
	assert.Greater(t, len(receivedEvents), 0, "Should have triggered rule event")
	receiveMu.Unlock()

	// Verify metrics were updated
	if processor.GetRuleMetrics() != nil {
		metrics := processor.GetRuleMetrics()
		assert.Greater(t, metrics["total_evaluated"].(int64), int64(0), "Should have evaluated rules")
	}
}

// TestIntegration_DynamicRuleCRUD tests runtime rule configuration updates
func TestIntegration_DynamicRuleCRUD(t *testing.T) {
	natsClient := getTestNATSClient(t)

	// Create processor with initial configuration
	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{
				Name:      "semantic_input",
				Type:      "nats",
				Subject:   "process.test.crud",
				Interface: "core.semantic.v1",
				Required:  true,
			},
		},
		Outputs: []component.PortDefinition{
			{
				Name:      "rule_events",
				Type:      "nats",
				Subject:   "events.rule.crud",
				Interface: "core.rule.v1",
				Required:  true,
			},
		},
	}
	config.EnableGraphIntegration = false

	processor, err := rule.NewProcessor(natsClient, &config)
	require.NoError(t, err)
	require.NotNil(t, processor)

	err = processor.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = processor.Start(ctx)
	require.NoError(t, err)
	defer processor.Stop(5 * time.Second)

	time.Sleep(100 * time.Millisecond)

	// Get initial rule count
	initialCount := len(processor.GetRuntimeConfig()["rules"].(map[string]any))

	// CREATE - Add new rule via runtime config
	newRule := map[string]any{
		"type": "test_rule",
		"name": "Dynamic Battery Rule",
		"conditions": []any{
			map[string]any{
				"field":    "battery.level",
				"operator": "lt",
				"value":    30.0,
				"required": true,
			},
		},
		"logic":   "and",
		"enabled": true,
	}

	changes := map[string]any{
		"rules": map[string]any{
			"dynamic_rule_001": newRule,
		},
	}

	// Validate and apply
	err = processor.ValidateConfigUpdate(changes)
	require.NoError(t, err)

	err = processor.ApplyConfigUpdate(changes)
	require.NoError(t, err)

	// READ - Verify rule was added
	currentConfig := processor.GetRuntimeConfig()
	rules := currentConfig["rules"].(map[string]any)
	assert.Contains(t, rules, "dynamic_rule_001")
	assert.Equal(t, initialCount+1, len(rules))

	// UPDATE - Modify the rule
	updatedRule := map[string]any{
		"type": "test_rule",
		"name": "Updated Dynamic Rule",
		"conditions": []any{
			map[string]any{
				"field":    "battery.level",
				"operator": "lt",
				"value":    15.0, // Changed threshold
				"required": true,
			},
		},
		"logic":   "and",
		"enabled": false, // Disabled
	}

	updateChanges := map[string]any{
		"rules": map[string]any{
			"dynamic_rule_001": updatedRule,
		},
	}

	err = processor.ApplyConfigUpdate(updateChanges)
	require.NoError(t, err)

	currentConfig = processor.GetRuntimeConfig()
	rules = currentConfig["rules"].(map[string]any)
	updatedRuleData := rules["dynamic_rule_001"].(map[string]any)
	assert.Equal(t, "Updated Dynamic Rule", updatedRuleData["name"])

	// DELETE would require removing from rules map (not currently implemented)
	// This is tracked as a limitation in runtime_config.go
}

// TestIntegration_JSONDSLRuleLoading tests loading rules from JSON files
func TestIntegration_JSONDSLRuleLoading(t *testing.T) {
	natsClient := getTestNATSClient(t)

	// Create a temporary JSON rule file
	tempDir := t.TempDir()
	ruleFile := tempDir + "/test_rule.json"

	ruleDef := rule.Definition{
		ID:   "json_dsl_test",
		Type: "test_rule",
		Name: "JSON DSL Test Rule",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "battery.level",
				Operator: "lte",
				Value:    20.0,
				Required: true,
			},
		},
		Logic:   "and",
		Enabled: true,
	}

	ruleJSON, err := json.Marshal(ruleDef)
	require.NoError(t, err)

	err = os.WriteFile(ruleFile, ruleJSON, 0644)
	require.NoError(t, err)

	// Create processor with rules_files config
	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{
				Name:      "semantic_input",
				Type:      "nats",
				Subject:   "process.test.json",
				Interface: "core.semantic.v1",
				Required:  true,
			},
		},
		Outputs: []component.PortDefinition{
			{
				Name:      "rule_events",
				Type:      "nats",
				Subject:   "events.rule.json",
				Interface: "core.rule.v1",
				Required:  true,
			},
		},
	}
	config.RulesFiles = []string{ruleFile}
	config.EnableGraphIntegration = false

	processor, err := rule.NewProcessor(natsClient, &config)
	require.NoError(t, err)
	require.NotNil(t, processor)

	err = processor.Initialize()
	require.NoError(t, err)

	// Verify rule was loaded from file
	runtimeConfig := processor.GetRuntimeConfig()
	assert.Equal(t, 1, runtimeConfig["rule_count"])
}

// TestIntegration_PrometheusMetrics tests metrics recording during rule processing
func TestIntegration_PrometheusMetrics(t *testing.T) {
	natsClient := getTestNATSClient(t)

	// Create processor with metrics registry
	metricsRegistry := metric.NewMetricsRegistry()
	require.NotNil(t, metricsRegistry)

	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{
				Name:      "semantic_input",
				Type:      "nats",
				Subject:   "process.test.metrics",
				Interface: "core.semantic.v1",
				Required:  true,
			},
		},
		Outputs: []component.PortDefinition{
			{
				Name:      "rule_events",
				Type:      "nats",
				Subject:   "events.rule.metrics",
				Interface: "core.rule.v1",
				Required:  true,
			},
		},
	}

	// Add a test rule
	ruleDef := rule.Definition{
		ID:   "metrics_test_rule",
		Type: "test_rule",
		Name: "Metrics Test Rule",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "battery.level",
				Operator: "lt",
				Value:    50.0,
				Required: true,
			},
		},
		Logic:   "and",
		Enabled: true,
	}
	config.InlineRules = []rule.Definition{ruleDef}
	config.EnableGraphIntegration = false

	processor, err := rule.NewProcessorWithMetrics(natsClient, &config, metricsRegistry)
	require.NoError(t, err)
	require.NotNil(t, processor)

	err = processor.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = processor.Start(ctx)
	require.NoError(t, err)
	defer processor.Stop(5 * time.Second)

	time.Sleep(100 * time.Millisecond)

	// Publish a test message that should trigger the rule
	testMsg, err := createSemanticMessage(map[string]any{
		"battery": map[string]any{
			"level": 25.0,
		},
	})
	require.NoError(t, err)

	err = natsClient.Publish(ctx, "process.test.metrics", testMsg)
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(500 * time.Millisecond)

	// Verify metrics were recorded
	// Note: We can't directly access processor.metrics from test package
	// Instead, check via GetRuleMetrics or Health
	metrics := processor.GetRuleMetrics()
	assert.Greater(t, metrics["total_evaluated"].(int64), int64(0), "Should have evaluated messages")

	// Health check should show activity
	health := processor.Health()
	assert.True(t, health.Healthy)
	assert.Greater(t, health.Uptime.Seconds(), 0.0)
}

// TestIntegration_DynamicWatchPatterns tests runtime updates to entity watch patterns
func TestIntegration_DynamicWatchPatterns(t *testing.T) {
	natsClient := getTestNATSClient(t)
	ctx := context.Background()

	// Create ENTITY_STATES KV bucket
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	kv, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: "ENTITY_STATES",
		})
		require.NoError(t, err)
	}

	// Create processor with initial watch patterns
	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{Name: "entity_events", Type: "nats", Subject: "events.graph.entity.>", Required: true},
		},
		Outputs: []component.PortDefinition{
			{Name: "rule_events", Type: "nats", Subject: "events.rule.triggered", Required: true},
		},
	}
	config.EntityWatchPatterns = []string{"c360.platform1.test.>"}
	config.EnableGraphIntegration = false

	// Add a rule that triggers on battery level
	ruleDef := rule.Definition{
		ID:   "dynamic_watch_test",
		Type: "test_rule",
		Name: "Dynamic Watch Test",
		Conditions: []expression.ConditionExpression{
			{Field: "robotics.battery.level", Operator: "lt", Value: 50.0, Required: true},
		},
		Logic:   "and",
		Enabled: true,
		Entity: rule.EntityConfig{
			Pattern: ">", // Match all entities
		},
	}
	config.InlineRules = []rule.Definition{ruleDef}

	processor, err := rule.NewProcessor(natsClient, &config)
	require.NoError(t, err)

	err = processor.Initialize()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = processor.Start(testCtx)
	require.NoError(t, err)
	defer processor.Stop(5 * time.Second)

	// Wait for watchers to start
	time.Sleep(300 * time.Millisecond)

	// Subscribe to rule events
	receivedEvents := make([]map[string]any, 0)
	var receiveMu sync.Mutex

	_, err = natsClient.Subscribe(testCtx, "events.rule.triggered", func(_ context.Context, msg *nats.Msg) {
		var event map[string]any
		if err := json.Unmarshal(msg.Data, &event); err == nil {
			receiveMu.Lock()
			receivedEvents = append(receivedEvents, event)
			receiveMu.Unlock()
		}
	})
	require.NoError(t, err)

	// Create entity that matches initial pattern
	entity1 := gtypes.EntityState{
		ID: "c360.platform1.test.drone.d001",
		Triples: []message.Triple{
			{Subject: "c360.platform1.test.drone.d001", Predicate: "robotics.battery.level", Object: 25.0},
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}
	entity1JSON, _ := json.Marshal(entity1)
	_, err = kv.Put(ctx, entity1.ID, entity1JSON)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// Verify rule triggered for entity1
	receiveMu.Lock()
	initialEventCount := len(receivedEvents)
	receiveMu.Unlock()
	assert.Greater(t, initialEventCount, 0, "Rule should have triggered for entity1")

	// Now dynamically add a new watch pattern
	changes := map[string]any{
		"entity_watch_patterns": []string{
			"c360.platform1.test.>",
			"c360.platform2.test.>", // New pattern
		},
	}
	err = processor.ApplyConfigUpdate(changes)
	require.NoError(t, err)

	// Wait for new watcher to start
	time.Sleep(300 * time.Millisecond)

	// Create entity that matches the new pattern
	entity2 := gtypes.EntityState{
		ID: "c360.platform2.test.drone.d002",
		Triples: []message.Triple{
			{Subject: "c360.platform2.test.drone.d002", Predicate: "robotics.battery.level", Object: 30.0},
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}
	entity2JSON, _ := json.Marshal(entity2)
	_, err = kv.Put(ctx, entity2.ID, entity2JSON)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// Verify rule triggered for entity2 (matching new pattern)
	receiveMu.Lock()
	finalEventCount := len(receivedEvents)
	receiveMu.Unlock()
	assert.Greater(t, finalEventCount, initialEventCount, "Rule should have triggered for entity2 after adding new pattern")

	// Verify runtime config shows updated patterns
	runtimeConfig := processor.GetRuntimeConfig()
	patterns := runtimeConfig["entity_watch_patterns"].([]string)
	assert.Contains(t, patterns, "c360.platform2.test.>")
}

// TestIntegration_GraphIntegration tests event publishing to graph processor
func TestIntegration_GraphIntegration(t *testing.T) {
	natsClient := getTestNATSClient(t)

	config := rule.DefaultConfig()
	config.Ports = &component.PortConfig{
		Inputs: []component.PortDefinition{
			{
				Name:      "semantic_input",
				Type:      "nats",
				Subject:   "process.test.graph",
				Interface: "core.semantic.v1",
				Required:  true,
			},
		},
		Outputs: []component.PortDefinition{
			{
				Name:      "rule_events",
				Type:      "nats",
				Subject:   "events.rule.graph",
				Interface: "core.rule.v1",
				Required:  true,
			},
			{
				Name:      "graph_mutations",
				Type:      "nats",
				Subject:   "graph.mutations",
				Interface: "core.graph.v1",
				Required:  false,
			},
		},
	}

	// Add a rule that generates graph events
	ruleDef := rule.Definition{
		ID:   "graph_test_rule",
		Type: "test_rule",
		Name: "Graph Integration Rule",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "battery.level",
				Operator: "lt",
				Value:    15.0,
				Required: true,
			},
		},
		Logic:   "and",
		Enabled: true,
	}
	config.InlineRules = []rule.Definition{ruleDef}
	config.EnableGraphIntegration = true // Enable graph integration

	processor, err := rule.NewProcessor(natsClient, &config)
	require.NoError(t, err)
	require.NotNil(t, processor)

	err = processor.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = processor.Start(ctx)
	require.NoError(t, err)
	defer processor.Stop(5 * time.Second)

	time.Sleep(100 * time.Millisecond)

	// Subscribe to graph events (events are published to graph.events.*)
	receivedMutations := make([]map[string]any, 0)
	var receiveMu sync.Mutex

	_, err = natsClient.Subscribe(ctx, "graph.events.>", func(_ context.Context, msg *nats.Msg) {
		var mutation map[string]any
		if err := json.Unmarshal(msg.Data, &mutation); err == nil {
			receiveMu.Lock()
			receivedMutations = append(receivedMutations, mutation)
			receiveMu.Unlock()
		}
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Publish message that triggers rule
	testMsg, err := createSemanticMessage(map[string]any{
		"battery": map[string]any{
			"level": 10.0, // Below threshold
		},
	})
	require.NoError(t, err)

	err = natsClient.Publish(ctx, "process.test.graph", testMsg)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify graph mutation events were published
	receiveMu.Lock()
	if config.EnableGraphIntegration {
		// Only check if graph integration is enabled
		assert.Greater(t, len(receivedMutations), 0, "Should have published graph mutations")
	}
	receiveMu.Unlock()
}
