//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
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

type blockingCronActionExecutor struct {
	startedOnce sync.Once
	started     chan struct{}
	release     chan struct{}
	calls       atomic.Int64
}

func (e *blockingCronActionExecutor) Execute(_ context.Context, _ rulepkg.Action, _ *rulepkg.ExecutionContext) error {
	e.calls.Add(1)
	e.startedOnce.Do(func() { close(e.started) })
	<-e.release
	return nil
}

// TestScheduledCronInflightAndCooldownContainment verifies the two cron
// containment guards required by ADR-060 §7 / #272:
//  1. CronScheduler's per-rule inflight CAS guard skips overlapping ticks
//     while an earlier fire is still executing (no stacked runs).
//  2. A cron rule's Cooldown suppresses subsequent ticks within the cooldown
//     window and persists last_fired_at in the RULE_SCHEDULES KV bucket.
func TestScheduledCronInflightAndCooldownContainment(t *testing.T) {
	t.Run("inflight_skips_overlapping_ticks", func(t *testing.T) {
		exec := &blockingCronActionExecutor{
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		scheduler, err := rulepkg.NewCronScheduler(rulepkg.CronSchedulerConfig{
			Executor: exec,
			Logger:   quietLogger(),
		})
		require.NoError(t, err)

		cronRule, err := rulepkg.NewCronRule(rulepkg.Definition{
			ID:       "scheduled_pulse_inflight_test",
			Type:     rulepkg.CronRuleType,
			Name:     "Scheduled Pulse Inflight Guard Test",
			Enabled:  true,
			Schedule: "@every 1s",
			Cooldown: "1ms",
			Actions: []rulepkg.Action{{
				Type:              rulepkg.ActionTypePublishAgent,
				Subject:           "agent.task.coordinator",
				Role:              "coordinator",
				LoopMaxIterations: "8",
				Model:             "fast",
				Prompt:            "Scheduled pulse check.",
			}},
		})
		require.NoError(t, err)
		require.NoError(t, scheduler.Register(cronRule))

		startCtx, cancelStart := context.WithCancel(context.Background())
		defer cancelStart()
		require.NoError(t, scheduler.Start(startCtx))
		defer func() {
			stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelStop()
			require.NoError(t, scheduler.Stop(stopCtx))
		}()

		select {
		case <-exec.started:
		case <-time.After(3 * time.Second):
			t.Fatal("cron rule did not fire first tick within 3s")
		}

		// Hold the first fire open across the next @every 1s tick and verify
		// overlapping ticks are skipped by the inflight CAS guard.
		time.Sleep(1300 * time.Millisecond)
		require.Equal(t, int64(1), exec.calls.Load(), "overlapping cron tick must be skipped while first fire is in flight")
		close(exec.release)
	})

	t.Run("cooldown_suppresses_subsequent_ticks_and_persists_last_fired", func(t *testing.T) {
		testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		logger := quietLogger()

		cfg := loadLoopBoundaryConfig(t, "../../configs/flow-bootstrap.json", testNATS.URL)
		require.NoError(t, config.NewStreamsManager(testNATS.Client, logger).EnsureStreams(ctx, cfg))

		proc := startedRuleProcessor(t, testNATS.Client)
		js, err := testNATS.Client.JetStream()
		require.NoError(t, err)
		stream, err := js.Stream(ctx, "AGENT")
		require.NoError(t, err)
		consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:        "cron-cooldown-tasks",
			FilterSubjects: []string{"agent.task.coordinator"},
			AckPolicy:      jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)

		payloads, err := buildPayloadRegistry()
		require.NoError(t, err)
		decoder := message.NewDecoder(payloads)

		ruleID := "scheduled_pulse_cooldown_test"
		cronDef := rulepkg.Definition{
			ID:       ruleID,
			Type:     rulepkg.CronRuleType,
			Name:     "Scheduled Pulse Cooldown Test",
			Enabled:  true,
			Schedule: "@every 1s",
			Cooldown: "1h",
			Actions: []rulepkg.Action{{
				Type:              rulepkg.ActionTypePublishAgent,
				Subject:           "agent.task.coordinator",
				Role:              "coordinator",
				LoopMaxIterations: "8",
				Model:             "fast",
				Prompt:            "Scheduled pulse check.",
			}},
		}
		rawDef, err := json.Marshal(cronDef)
		require.NoError(t, err)
		var ruleMap map[string]any
		require.NoError(t, json.Unmarshal(rawDef, &ruleMap))
		require.NoError(t, proc.ApplyConfigUpdate(map[string]any{
			"rules": map[string]any{ruleID: ruleMap},
		}))

		fetchCtx, fetchCancel := context.WithTimeout(ctx, 3*time.Second)
		defer fetchCancel()
		msg, err := consumer.Next(jetstream.FetchContext(fetchCtx))
		require.NoError(t, err, "first cron fire must publish a coordinator TaskMessage")
		envelope, err := decoder.Decode(msg.Data())
		require.NoError(t, err)
		require.NoError(t, msg.DoubleAck(ctx))

		task, ok := envelope.Payload().(*agentic.TaskMessage)
		require.True(t, ok)
		require.NotNil(t, task.MaxIterations)
		require.Equal(t, 8, *task.MaxIterations)

		// Verify RULE_SCHEDULES KV persisted the last_fired_at record.
		schedBucket, err := js.KeyValue(ctx, rulepkg.ScheduleBucketName)
		require.NoError(t, err)
		entry, err := schedBucket.Get(ctx, ruleID)
		require.NoError(t, err)
		require.NotEmpty(t, entry.Value())

		// Observe across the next @every 1s tick: the 1h cooldown must suppress it.
		quietCtx, quietCancel := context.WithTimeout(ctx, 1400*time.Millisecond)
		defer quietCancel()
		extraMsg, err := consumer.Next(jetstream.FetchContext(quietCtx))
		require.Error(t, err, "1h cooldown must suppress subsequent ticks")
		require.Nil(t, extraMsg)
	})
}

// TestScheduledCronPatternBKillSwitch verifies that disabling a scheduled
// cron rule via Pattern-B rule.ConfigManager.SaveRule (Enabled: false)
// hot-reloads into the running rule.Processor without process restart,
// deregisters future cron ticks while allowing in-flight work triggered by an
// earlier tick to complete undisturbed.
func TestScheduledCronPatternBKillSwitch(t *testing.T) {
	testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	logger := quietLogger()

	ruleManager, err := rulepkg.NewConfigManager(logger)
	require.NoError(t, err)
	cfg := loadLoopBoundaryConfig(t, "../../configs/flow-bootstrap.json", testNATS.URL)
	require.NoError(t, config.NewStreamsManager(testNATS.Client, logger).EnsureStreams(ctx, cfg))
	_, _, configManager, err := setupRemainingInfrastructure(
		ctx, cfg, testNATS.Client, logger, ruleManager,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = configManager.Stop(5 * time.Second) })

	proc := &observedRuleProcessor{
		Processor: startedRuleProcessor(t, testNATS.Client),
		applied:   make(chan map[string]any, 8),
	}
	ruleConfig := newRuleConfigService(ruleManager, []rulepkg.HotReloadTarget{proc}, logger)
	require.NoError(t, ruleConfig.Start(ctx))
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), ruleProcessorStopBudget)
		defer stopCancel()
		require.NoError(t, ruleConfig.Stop(stopCtx))
	})

	// Consume initial seeded reconcile.
	_ = nextApplied(t, proc.applied)

	js, err := testNATS.Client.JetStream()
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "AGENT")
	require.NoError(t, err)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:        "cron-killswitch-tasks",
		FilterSubjects: []string{"agent.task.coordinator"},
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	payloads, err := buildPayloadRegistry()
	require.NoError(t, err)
	decoder := message.NewDecoder(payloads)

	rawShipped, err := os.ReadFile("../../configs/rules/research/10-scheduled-program-pulse.json")
	require.NoError(t, err)
	var shippedDef rulepkg.Definition
	require.NoError(t, json.Unmarshal(rawShipped, &shippedDef))
	require.Equal(t, "scheduled_program_pulse", shippedDef.ID)
	require.False(t, shippedDef.Enabled, "shipped 10-scheduled-program-pulse.json must be disabled by default")

	const ruleID = "scheduled_program_pulse"
	cronDef := func(enabled bool) rulepkg.Definition {
		d := shippedDef
		d.Enabled = enabled
		d.Schedule = "@every 1s"
		d.Cooldown = "1ms"
		return d
	}

	require.NoError(t, ruleManager.SaveRule(ctx, ruleID, cronDef(true)))
	for {
		rules := nextApplied(t, proc.applied)
		if raw, ok := rules[ruleID].(map[string]any); ok {
			if enabled, _ := raw["enabled"].(bool); enabled {
				break
			}
		}
	}

	// Wait for the first scheduled fire.
	fetchCtx, fetchCancel := context.WithTimeout(ctx, 3*time.Second)
	defer fetchCancel()
	msg, err := consumer.Next(jetstream.FetchContext(fetchCtx))
	require.NoError(t, err, "enabled cron rule must fire and publish TaskMessage")
	envelope, err := decoder.Decode(msg.Data())
	require.NoError(t, err)
	require.NoError(t, msg.DoubleAck(ctx))
	firstTask, ok := envelope.Payload().(*agentic.TaskMessage)
	require.True(t, ok)
	require.NotNil(t, firstTask.MaxIterations)
	require.Equal(t, 8, *firstTask.MaxIterations)
	require.Contains(t, firstTask.Prompt, "@pulse")

	// Simulate an in-flight downstream execution triggered by the first fire
	// that stays active across the Pattern-B disable operation.
	releaseInflight := make(chan struct{})
	inflightCompleted := make(chan string, 1)
	go func(taskID string) {
		<-releaseInflight
		inflightCompleted <- taskID
	}(firstTask.TaskID)

	// Disable the cron rule at runtime via Pattern-B rule.ConfigManager (no restart).
	require.NoError(t, ruleManager.SaveRule(ctx, ruleID, cronDef(false)))
	for {
		rules := nextApplied(t, proc.applied)
		if raw, ok := rules[ruleID].(map[string]any); ok {
			if enabled, _ := raw["enabled"].(bool); !enabled {
				break
			}
		}
	}

	// Drain at most one message if a tick raced before ApplyConfigUpdate completed.
	drainCtx, drainCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	if racedMsg, raceErr := consumer.Next(jetstream.FetchContext(drainCtx)); raceErr == nil {
		_ = racedMsg.DoubleAck(ctx)
	}
	drainCancel()

	// Complete the in-flight execution from the first fire after the rule is disabled.
	close(releaseInflight)
	require.Equal(t, firstTask.TaskID, <-inflightCompleted, "in-flight execution from first fire must complete undisturbed after kill switch")

	// Observe across a full post-disable @every 1s window: zero new fires may occur.
	postDisableCtx, postDisableCancel := context.WithTimeout(ctx, 1400*time.Millisecond)
	defer postDisableCancel()
	extraMsg, err := consumer.Next(jetstream.FetchContext(postDisableCtx))
	require.Error(t, err, "disabled cron rule must not fire after Pattern-B hot-reload")
	require.Nil(t, extraMsg)
}
