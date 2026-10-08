//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

const loopBoundaryFixtures = "../../test/fixtures/rule-family-consumer"

// TestConfiguredLoopFirstRequestBoundary joins the product's registered loop
// and graph owner over real NATS. It stops at the first AgentRequest: no rule
// executor, tool-name resolution, model provider, continuation, or E3 claim.
func TestConfiguredLoopFirstRequestBoundary(t *testing.T) {
	for _, bootstrap := range []string{"flow-bootstrap.json", "e2e-flow-bootstrap.json"} {
		t.Run(bootstrap, func(t *testing.T) {
			testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
			// Owner Stop runs before this cancel, including failed assertions.
			// t.Context alone would cancel Start authority before t.Cleanup.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 90*time.Second)
			t.Cleanup(cancel)
			cfg := loadLoopBoundaryConfig(t, filepath.Join("../../configs", bootstrap), testNATS.URL)
			declaredPlatform := cfg.Platform
			registry, _, err := setupRegistriesAndManager(cfg)
			require.NoError(t, err)
			payloads, err := buildPayloadRegistry()
			require.NoError(t, err)
			logger := quietLogger()
			require.NoError(t, config.NewStreamsManager(testNATS.Client, logger).EnsureStreams(ctx, cfg))
			configManager, err := config.NewConfigManager(cfg, testNATS.Client, logger)
			require.NoError(t, err)
			require.NoError(t, configManager.Start(ctx))
			t.Cleanup(func() { require.NoError(t, configManager.Stop(5*time.Second)) })
			platform := extractPlatformMeta(configManager.GetConfig().Get())
			require.Equal(t, declaredPlatform.Org, platform.Org)
			require.True(t, strings.HasPrefix(platform.Platform, declaredPlatform.ID+"-"), "public config manager must mint the shipped platform stem")

			managerService, err := service.NewComponentManager(json.RawMessage(`{}`), &service.Dependencies{
				NATSClient: testNATS.Client, Manager: configManager, ComponentRegistry: registry,
				PayloadRegistry: payloads, Logger: logger, Platform: platform,
			})
			require.NoError(t, err)
			manager, ok := managerService.(*service.ComponentManager)
			require.True(t, ok)
			stop := func() error {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer stopCancel()
				return manager.Stop(stopCtx)
			}
			stopped := false
			t.Cleanup(func() {
				if !stopped {
					require.NoError(t, stop())
				}
			})
			require.NoError(t, manager.Initialize())
			require.NoError(t, manager.Start(ctx))
			statuses := manager.GetComponentStatus()
			require.Len(t, statuses, 2, "only the approved graph owner and loop may run")
			for _, name := range []string{"graph-ingest", "teams-loop"} {
				status, exists := statuses[name]
				require.True(t, exists, "product registry did not admit %s", name)
				require.Equal(t, component.StateStarted, status.State, "%s: %v", name, status.LastError)
			}

			// Start has joined component setup. The durable capture is installed
			// before publication, so no sleeps or transient core subscriptions
			// stand in for request readiness.
			js, err := testNATS.Client.JetStream()
			require.NoError(t, err)
			stream, err := js.Stream(ctx, "AGENT")
			require.NoError(t, err)
			requests, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
				Durable: "loop-boundary-requests", FilterSubject: "agent.request.>", AckPolicy: jetstream.AckExplicitPolicy,
			})
			require.NoError(t, err)
			decoder := message.NewDecoder(payloads)
			reader := graph.NewExactEntityReader(testNATS.Client, 5*time.Second)
			var expected map[string]loopBoundaryExpected
			loopBoundaryReadJSON(t, filepath.Join(loopBoundaryFixtures, "expected.json"), &expected)
			require.Len(t, expected, 3)
			seenRequests := make(map[string]bool)
			seenLoops := make(map[string]bool)
			seenEntities := make(map[string]bool)

			// Fixed order leaves the omission case last in the SAME live loop
			// component, after both knobs have appeared on other fresh roots.
			for _, name := range []string{"required-live.json", "forced-function.json", "response-format.json", "omitted-knobs"} {
				t.Run(name, func(t *testing.T) {
					task, subject, want := loopBoundaryTask(t, name, expected)
					require.Empty(t, task.LoopID, "root identity must be minted by the loop")
					require.Empty(t, task.RunID)
					require.Empty(t, task.ParentLoopID)
					require.NoError(t, task.Validate())
					wire, err := json.Marshal(message.NewBaseMessage(task.Schema(), &task, "loop-boundary-test"))
					require.NoError(t, err)
					opCtx, opCancel := context.WithTimeout(ctx, 15*time.Second)
					defer opCancel()
					require.NoError(t, testNATS.Client.PublishToStream(opCtx, subject, wire))

					var request *agentic.AgentRequest
					for {
						received, err := requests.Next(jetstream.FetchContext(opCtx))
						require.NoError(t, err, "configured loop did not emit its first request")
						envelope, err := decoder.Decode(received.Data())
						require.NoError(t, err)
						var canonical bool
						request, canonical = envelope.Payload().(*agentic.AgentRequest)
						require.True(t, canonical, "canonical request payload = %T", envelope.Payload())
						require.NoError(t, request.Validate())
						require.Equal(t, "agent.request."+request.LoopID, received.Subject())
						require.NoError(t, received.DoubleAck(opCtx))
						if !seenRequests[request.RequestID] {
							break
						}
						// Publications are at least once. A replay of an earlier
						// captured request is not a new root's first request.
					}
					require.NotEmpty(t, request.RequestID)
					require.NotEmpty(t, request.LoopID)
					require.Equal(t, request.LoopID+":req:1:0", request.RequestID, "fresh root must publish its first request with matching loop identity")
					require.False(t, seenLoops[request.LoopID], "fresh task reused an earlier loop")
					seenRequests[request.RequestID] = true
					seenLoops[request.LoopID] = true
					require.Equal(t, want.Role, request.Role)
					require.Equal(t, want.Model, request.Model, "model capability is forwarded, not resolved here")
					require.Equal(t, want.ToolChoice, loopBoundaryActualChoice(request.ToolChoice))
					require.Equal(t, want.ResponseFormat, loopBoundaryActualFormat(request.ResponseFormat))
					require.Empty(t, request.Tools, "this fixture does not resolve or execute tool names")
					require.Contains(t, request.Messages, agentic.ChatMessage{Role: "user", Content: task.Prompt})

					// Frozen intake requires typed birth before publication. This
					// post-capture read proves the persisted birth and task binding;
					// it does not independently observe the two operations' ordering.
					entityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, request.LoopID)
					born, err := reader.ReadExactEntity(opCtx, entityID)
					require.NoError(t, err, "first request needs an actual typed loop birth")
					require.Positive(t, born.KVRevision)
					require.Equal(t, entityID, born.Entity.ID)
					require.False(t, seenEntities[born.Entity.ID], "fresh root reused an earlier persisted loop entity")
					seenEntities[born.Entity.ID] = true
					require.Equal(t, agentic.LoopExecutionMessageType(), born.Entity.MessageType)
					taskID, exists := born.Entity.GetPropertyValue(agvocab.LoopTask)
					require.True(t, exists)
					require.Equal(t, task.TaskID, taskID, "request loop must belong to this fixture task")
				})
			}
			require.NoError(t, stop())
			stopped = true
		})
	}
}

// Goldens use only test-local primitive types; production JSON behavior cannot
// transform the expected and actual sides together.
type loopBoundaryExpected struct {
	Subject        string              `json:"subject"`
	Role           string              `json:"role"`
	Model          string              `json:"model"`
	ToolChoice     *loopBoundaryChoice `json:"tool_choice"`
	ResponseFormat *loopBoundaryFormat `json:"response_format"`
}

type loopBoundaryChoice struct {
	Mode         string `json:"mode"`
	FunctionName string `json:"function_name"`
}

type loopBoundaryFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func loopBoundaryActualChoice(choice *agentic.ToolChoice) *loopBoundaryChoice {
	if choice == nil {
		return nil
	}
	return &loopBoundaryChoice{Mode: choice.Mode, FunctionName: choice.FunctionName}
}

func loopBoundaryActualFormat(format *agentic.ResponseFormat) *loopBoundaryFormat {
	if format == nil {
		return nil
	}
	return &loopBoundaryFormat{Type: format.Type, Name: format.Name, Strict: format.Strict, Schema: format.Schema}
}

func loopBoundaryReadJSON(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, dst))
}

func loopBoundaryTask(t *testing.T, name string, expected map[string]loopBoundaryExpected) (agentic.TaskMessage, string, loopBoundaryExpected) {
	t.Helper()
	if name == "omitted-knobs" {
		return agentic.TaskMessage{
			TaskID: "loop-boundary-omitted", Role: "fixture-omitted", Model: "fixture-model",
			Prompt: "A fresh root with no per-task tool choice or response format.", Tools: []agentic.ToolDefinition{},
		}, "agent.task.fixture-omitted", loopBoundaryExpected{Role: "fixture-omitted", Model: "fixture-model"}
	}
	var definition rule.Definition
	loopBoundaryReadJSON(t, filepath.Join(loopBoundaryFixtures, name), &definition)
	require.Len(t, definition.OnEnter, 1)
	action := definition.OnEnter[0]
	require.Equal(t, rule.ActionTypePublishAgent, action.Type)
	want, exists := expected[name]
	require.True(t, exists)
	require.Equal(t, want.Subject, action.Subject)
	// Construct a canonical task explicitly. This is not rule execution: tools
	// are deliberately empty, and action metadata/run anchors are not copied.
	return agentic.TaskMessage{
		TaskID: "loop-boundary-" + name, Role: action.Role, Model: action.Model, Prompt: action.Prompt,
		Tools: []agentic.ToolDefinition{}, ToolChoice: action.ToolChoice, ResponseFormat: action.ResponseFormat,
	}, action.Subject, want
}

func loadLoopBoundaryConfig(t *testing.T, path, natsURL string) *config.Config {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg, err := config.NewLoader().LoadFromBytes(data)
	require.NoError(t, err)
	subset := make(config.ComponentConfigs)
	for _, name := range []string{"graph-ingest", "teams-loop"} {
		instance, exists := cfg.Components[name]
		require.True(t, exists)
		require.True(t, instance.Enabled)
		if name == "teams-loop" {
			instance.Config = loopBoundaryExternalInputs(t, instance.Config)
		}
		subset[name] = instance
	}
	cfg.Components = subset
	cfg.Services = make(types.ServiceConfigs)
	cfg.NATS.URLs = []string{natsURL}
	return cfg
}

func loopBoundaryExternalInputs(t *testing.T, original json.RawMessage) json.RawMessage {
	t.Helper()
	var fields, ports map[string]json.RawMessage
	var inputs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(original, &fields))
	require.NoError(t, json.Unmarshal(fields["ports"], &ports))
	require.NoError(t, json.Unmarshal(ports["inputs"], &inputs))
	previous := make(map[int]json.RawMessage)
	for i, input := range inputs {
		var name string
		require.NoError(t, json.Unmarshal(input["name"], &name))
		if name != "agent.task" && name != "agent.response" && name != "tool.result" {
			continue
		}
		var required, external bool
		require.NoError(t, json.Unmarshal(input["required"], &required))
		require.True(t, required, "%s must remain a required shipped input", name)
		prior := input["external"]
		if prior != nil {
			require.NoError(t, json.Unmarshal(prior, &external))
		}
		require.False(t, external, "%s originally has a product publisher", name)
		previous[i] = prior
		input["external"] = json.RawMessage(`true`)
	}
	require.Len(t, previous, 3)
	encode := func() json.RawMessage {
		var err error
		ports["inputs"], err = json.Marshal(inputs)
		require.NoError(t, err)
		fields["ports"], err = json.Marshal(ports)
		require.NoError(t, err)
		encoded, err := json.Marshal(fields)
		require.NoError(t, err)
		return encoded
	}
	derived := encode()
	for i, prior := range previous {
		delete(inputs[i], "external")
		if prior != nil {
			inputs[i]["external"] = prior
		}
	}
	require.JSONEq(t, string(original), string(encode()), "only the three External input markers may differ")
	return derived
}
