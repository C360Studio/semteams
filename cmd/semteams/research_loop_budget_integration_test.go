//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestResearchLoopBudgetTerminalBoundary exercises the shipped loop + graph
// owner over real NATS, with deterministic model/tool responses. The small
// component ceiling narrows a pack task's larger budget. No real model, tool
// executor, rule matcher, run lifecycle, or user delivery runs in this fixture.
func TestResearchLoopBudgetTerminalBoundary(t *testing.T) {
	testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 90*time.Second)
	t.Cleanup(cancel)
	cfg := loadLoopBoundaryConfig(t, "../../configs/flow-bootstrap.json", testNATS.URL)
	instance := cfg.Components["teams-loop"]
	var loopConfig map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(instance.Config, &loopConfig))
	loopConfig["max_iterations"] = json.RawMessage(`2`)
	encoded, err := json.Marshal(loopConfig)
	require.NoError(t, err)
	instance.Config = encoded
	cfg.Components["teams-loop"] = instance
	registry, _, err := setupRegistriesAndManager(cfg)
	require.NoError(t, err)
	payloads, err := buildPayloadRegistry()
	require.NoError(t, err)
	logger := quietLogger()
	require.NoError(t, config.NewStreamsManager(testNATS.Client, logger).EnsureStreams(ctx, cfg))
	manager, err := config.NewConfigManager(cfg, testNATS.Client, logger)
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { require.NoError(t, manager.Stop(5*time.Second)) })
	serviceManager, err := service.NewComponentManager(json.RawMessage(`{}`), &service.Dependencies{
		NATSClient: testNATS.Client, Manager: manager, ComponentRegistry: registry,
		PayloadRegistry: payloads, Logger: logger, Platform: extractPlatformMeta(manager.GetConfig().Get()),
	})
	require.NoError(t, err)
	components := serviceManager.(*service.ComponentManager)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		require.NoError(t, components.Stop(stopCtx))
	})
	require.NoError(t, components.Initialize())
	require.NoError(t, components.Start(ctx))
	js, err := testNATS.Client.JetStream()
	require.NoError(t, err)
	decoder := message.NewDecoder(payloads)
	newConsumer := func(streamName, name string, subjects ...string) jetstream.Consumer {
		stream, err := js.Stream(ctx, streamName)
		require.NoError(t, err)
		consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable: name, FilterSubjects: subjects, AckPolicy: jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)
		return consumer
	}
	requests := newConsumer("AGENT", "budget-requests", "agent.request.>")
	created := newConsumer("AGENT", "budget-created", "agent.created.>")
	terminals := newConsumer("AGENT", "budget-terminals", "agent.complete.>", "agent.failed.>")
	calls := newConsumer("TOOL", "budget-calls", "tool.execute.>")
	receive := func(t *testing.T, consumer jetstream.Consumer) message.Payload {
		t.Helper()
		opCtx, opCancel := context.WithTimeout(ctx, 10*time.Second)
		defer opCancel()
		msg, err := consumer.Next(jetstream.FetchContext(opCtx))
		require.NoError(t, err)
		envelope, err := decoder.Decode(msg.Data())
		require.NoError(t, err)
		require.NoError(t, msg.DoubleAck(opCtx))
		return envelope.Payload()
	}
	publish := func(t *testing.T, subject string, payload message.Payload) {
		t.Helper()
		require.NoError(t, payload.Validate())
		wire, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "budget-fixture"))
		require.NoError(t, err)
		require.NoError(t, testNATS.Client.PublishToStream(ctx, subject, wire))
	}
	for _, exhaust := range []bool{false, true} {
		t.Run(fmt.Sprintf("exhaust=%t", exhaust), func(t *testing.T) {
			// Canonical task fixture, not claimed to be an actual pack dispatch. The
			// contract test separately connects all 17 live actions to loop intake.
			budget := 12
			task := &agentic.TaskMessage{TaskID: fmt.Sprintf("budget-%t", exhaust), Role: "researcher-research-plan", Model: "fixture",
				Prompt: "Exercise deterministic iteration containment.", MaxIterations: &budget, Tools: []agentic.ToolDefinition{}}
			publish(t, "agent.task.researcher-research-plan", task)
			birth, ok := receive(t, created).(*agentic.LoopCreatedEvent)
			require.True(t, ok)
			require.Equal(t, task.TaskID, birth.TaskID)
			require.Equal(t, 2, birth.MaxIterations, "component ceiling must narrow the task cap")
			toolRounds := 1
			if exhaust {
				toolRounds = 2
			}
			for turn := 0; turn < toolRounds; turn++ {
				request, ok := receive(t, requests).(*agentic.AgentRequest)
				require.True(t, ok)
				require.Equal(t, birth.LoopID, request.LoopID)
				require.Equal(t, fmt.Sprintf("%s:req:%d:0", birth.LoopID, turn+1), request.RequestID)
				name := "scratchpad"
				if !exhaust {
					name = "decide"
				}
				response := &agentic.AgentResponse{RequestID: request.RequestID, Status: agentic.StatusToolCall,
					Message: agentic.ChatMessage{Role: "assistant", ToolCalls: []agentic.ToolCall{{ID: fmt.Sprintf("call-%d", turn), Name: name}}}}
				publish(t, "agent.response."+request.LoopID, response)
				call, ok := receive(t, calls).(*agentic.ToolCall)
				require.True(t, ok)
				require.Equal(t, request.RequestID, call.RequestID)
				require.NotEmpty(t, call.ExecutionID)
				publish(t, "tool.result."+request.LoopID, &agentic.ToolResult{
					CallID: call.ID, Name: call.Name, LoopID: call.LoopID, RequestID: call.RequestID,
					ExecutionID: call.ExecutionID, CallOrdinal: call.CallOrdinal,
					Content: `{"action":"gather","reason":"fixture evidence"}`, StopLoop: !exhaust,
				})
			}
			if exhaust {
				// Frozen semantics count completed tool rounds. Once two have completed,
				// a further request exists, but its response must fail the budget guard
				// even if the model claims completion. Do not call that response success.
				request, ok := receive(t, requests).(*agentic.AgentRequest)
				require.True(t, ok)
				require.Equal(t, birth.LoopID+":req:3:0", request.RequestID)
				publish(t, "agent.response."+request.LoopID, &agentic.AgentResponse{RequestID: request.RequestID,
					Status: agentic.StatusComplete, Message: agentic.ChatMessage{Role: "assistant", Content: "Would otherwise complete"}})
				failure, ok := receive(t, terminals).(*agentic.LoopFailedEvent)
				require.True(t, ok, "the exhausted response must produce a failure terminal")
				require.Equal(t, birth.LoopID, failure.LoopID)
				require.Equal(t, "max_iterations", failure.Reason)
				require.Equal(t, 2, failure.Iterations)
			} else {
				completion, ok := receive(t, terminals).(*agentic.LoopCompletedEvent)
				require.True(t, ok)
				require.Equal(t, birth.LoopID, completion.LoopID)
				require.Equal(t, agentic.OutcomeSuccess, completion.Outcome)
				require.Equal(t, `{"action":"gather","reason":"fixture evidence"}`, completion.Result)
			}
		})
	}
}
