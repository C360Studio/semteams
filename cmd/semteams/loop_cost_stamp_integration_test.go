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
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestLoopCostStampingAcrossBootstraps verifies #248 / #301 Slice 1 against
// the composed teams-loop + graph-ingest runtime over real NATS for both
// configs/flow-bootstrap.json and configs/e2e-flow-bootstrap.json:
//   - Direct-endpoint loops (coordinator default model) and capability-spawned
//     loops (coordinator, research, reviewing) resolve their endpoint via
//     model.ResolveEndpointName (semstreams#584 fix at frozen 8b99efe).
//   - Completed loop entities in ENTITY_STATES carry agent.loop.tokens-in,
//     agent.loop.tokens-out, agent.loop.model-used, and agent.loop.cost-usd
//     matching (tokensIn*inPrice + tokensOut*outPrice)/1_000_000.
func TestLoopCostStampingAcrossBootstraps(t *testing.T) {
	type loopCase struct {
		name         string
		role         string
		modelOrCap   string
		wantEndpoint string
		tokensIn     int
		tokensOut    int
		wantCostUSD  float64
	}

	bootstraps := []struct {
		file  string
		cases []loopCase
	}{
		{
			file: "flow-bootstrap.json",
			cases: []loopCase{
				{
					name:         "direct-gemini-flash",
					role:         "coordinator",
					modelOrCap:   "gemini-flash",
					wantEndpoint: "gemini-flash",
					tokensIn:     200_000,
					tokensOut:    100_000,
					wantCostUSD:  0.40, // (200k * 0.50 + 100k * 3.00) / 1M
				},
				{
					name:         "capability-coordinator-gemini-pro",
					role:         "coordinator",
					modelOrCap:   "coordinator",
					wantEndpoint: "gemini-pro",
					tokensIn:     200_000,
					tokensOut:    100_000,
					wantCostUSD:  1.60, // (200k * 2.00 + 100k * 12.00) / 1M
				},
				{
					name:         "capability-research-gemini-flash",
					role:         "researcher-research-gather",
					modelOrCap:   "research",
					wantEndpoint: "gemini-flash",
					tokensIn:     100_000,
					tokensOut:    50_000,
					wantCostUSD:  0.20, // (100k * 0.50 + 50k * 3.00) / 1M
				},
				{
					name:         "capability-reviewing-gemini-flash",
					role:         "reviewer-research",
					modelOrCap:   "reviewing",
					wantEndpoint: "gemini-flash",
					tokensIn:     100_000,
					tokensOut:    50_000,
					wantCostUSD:  0.20, // (100k * 0.50 + 50k * 3.00) / 1M
				},
			},
		},
		{
			file: "e2e-flow-bootstrap.json",
			cases: []loopCase{
				{
					name:         "direct-general",
					role:         "coordinator",
					modelOrCap:   "general",
					wantEndpoint: "general",
					tokensIn:     200_000,
					tokensOut:    100_000,
					wantCostUSD:  0.40, // (200k * 0.50 + 100k * 3.00) / 1M
				},
				{
					name:         "capability-research-general",
					role:         "researcher-research-plan",
					modelOrCap:   "research",
					wantEndpoint: "general",
					tokensIn:     100_000,
					tokensOut:    50_000,
					wantCostUSD:  0.20, // (100k * 0.50 + 50k * 3.00) / 1M
				},
				{
					name:         "capability-reviewing-general",
					role:         "reviewer-research",
					modelOrCap:   "reviewing",
					wantEndpoint: "general",
					tokensIn:     100_000,
					tokensOut:    50_000,
					wantCostUSD:  0.20, // (100k * 0.50 + 50k * 3.00) / 1M
				},
			},
		},
	}

	for _, b := range bootstraps {
		t.Run(b.file, func(t *testing.T) {
			testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 90*time.Second)
			t.Cleanup(cancel)

			cfg := loadLoopBoundaryConfig(t, "../../configs/"+b.file, testNATS.URL)
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

			platform := extractPlatformMeta(manager.GetConfig().Get())
			serviceManager, err := service.NewComponentManager(json.RawMessage(`{}`), &service.Dependencies{
				NATSClient: testNATS.Client, Manager: manager, ComponentRegistry: registry,
				PayloadRegistry: payloads, Logger: logger, Platform: platform,
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
			agentStream, err := js.Stream(ctx, "AGENT")
			require.NoError(t, err)
			decoder := message.NewDecoder(payloads)
			reader := graph.NewExactEntityReader(testNATS.Client, 5*time.Second)

			publish := func(t *testing.T, subject string, payload message.Payload) {
				t.Helper()
				require.NoError(t, payload.Validate())
				wire, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "cost-fixture"))
				require.NoError(t, err)
				require.NoError(t, testNATS.Client.PublishToStream(ctx, subject, wire))
			}

			for idx, lc := range b.cases {
				t.Run(lc.name, func(t *testing.T) {
					opCtx, opCancel := context.WithTimeout(ctx, 15*time.Second)
					defer opCancel()

					requests, err := agentStream.CreateOrUpdateConsumer(opCtx, jetstream.ConsumerConfig{
						Durable:        fmt.Sprintf("cost-requests-%d", idx),
						FilterSubjects: []string{"agent.request.>"},
						AckPolicy:      jetstream.AckExplicitPolicy,
						DeliverPolicy:  jetstream.DeliverNewPolicy,
					})
					require.NoError(t, err)
					terminals, err := agentStream.CreateOrUpdateConsumer(opCtx, jetstream.ConsumerConfig{
						Durable:        fmt.Sprintf("cost-terminals-%d", idx),
						FilterSubjects: []string{"agent.complete.>"},
						AckPolicy:      jetstream.AckExplicitPolicy,
						DeliverPolicy:  jetstream.DeliverNewPolicy,
					})
					require.NoError(t, err)

					task := &agentic.TaskMessage{
						TaskID: fmt.Sprintf("cost-task-%d", idx),
						Role:   lc.role,
						Model:  lc.modelOrCap,
						Prompt: "Verify loop cost-usd stamping.",
						Tools:  []agentic.ToolDefinition{},
					}
					publish(t, "agent.task."+lc.role, task)

					reqMsg, err := requests.Next(jetstream.FetchContext(opCtx))
					require.NoError(t, err)
					reqEnv, err := decoder.Decode(reqMsg.Data())
					require.NoError(t, err)
					require.NoError(t, reqMsg.DoubleAck(opCtx))
					request, ok := reqEnv.Payload().(*agentic.AgentRequest)
					require.True(t, ok)

					publish(t, "agent.response."+request.LoopID, &agentic.AgentResponse{
						RequestID: request.RequestID,
						Status:    agentic.StatusComplete,
						Message:   agentic.ChatMessage{Role: "assistant", Content: "completed"},
						TokenUsage: agentic.TokenUsage{
							PromptTokens:     lc.tokensIn,
							CompletionTokens: lc.tokensOut,
						},
					})

					termMsg, err := terminals.Next(jetstream.FetchContext(opCtx))
					require.NoError(t, err)
					termEnv, err := decoder.Decode(termMsg.Data())
					require.NoError(t, err)
					require.NoError(t, termMsg.DoubleAck(opCtx))
					completed, ok := termEnv.Payload().(*agentic.LoopCompletedEvent)
					require.True(t, ok)
					require.Equal(t, request.LoopID, completed.LoopID)
					require.Equal(t, lc.tokensIn, completed.TokensIn)
					require.Equal(t, lc.tokensOut, completed.TokensOut)

					entityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, request.LoopID)
					exact, err := reader.ReadExactEntity(opCtx, entityID)
					require.NoError(t, err)
					require.NotNil(t, exact.Entity)

					modelUsed, ok := exact.Entity.GetPropertyValue(agvocab.LoopModelUsed)
					require.True(t, ok, "agent.loop.model-used must be stamped on completed loop")
					wantModelEntity := agentic.ModelEndpointEntityID(platform.Org, platform.Platform, lc.wantEndpoint)
					require.Equal(t, wantModelEntity, modelUsed, "agent.loop.model-used must resolve to the concrete endpoint entity ID")

					tokensInVal, ok := exact.Entity.GetPropertyValue(agvocab.LoopTokensIn)
					require.True(t, ok)
					require.InDelta(t, float64(lc.tokensIn), tokensInVal, 1e-6)

					tokensOutVal, ok := exact.Entity.GetPropertyValue(agvocab.LoopTokensOut)
					require.True(t, ok)
					require.InDelta(t, float64(lc.tokensOut), tokensOutVal, 1e-6)

					costVal, ok := exact.Entity.GetPropertyValue(agvocab.LoopCostUSD)
					require.True(t, ok, "agent.loop.cost-usd must be stamped on completed loop")
					require.InDelta(t, lc.wantCostUSD, costVal, 1e-9, "agent.loop.cost-usd must equal (tokensIn*inPrice + tokensOut*outPrice)/1e6")
				})
			}
		})
	}
}
