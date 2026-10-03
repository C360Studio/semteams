//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestConfiguredModelRetryBoundary covers the retry-path gap documented by the
// mock bootstrap. This exercises product registration and public composition,
// rather than constructing the upstream model directly. Response publication
// remains at least once; this test does not assert delivery deduplication.
func TestConfiguredModelRetryBoundary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		bootstrap    string
		wantAttempts int
		wantStatus   string
	}{
		{"production", "../../configs/flow-bootstrap.json", 3, agentic.StatusComplete},
		{"mock", "../../configs/e2e-flow-bootstrap.json", 1, agentic.StatusError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testNATS := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV(), natsclient.WithNATSVersion("2.14.4-alpine"))
			// Keep Start's authority alive until owner cleanup finishes, including
			// assertion failures: testing.T.Context is canceled before cleanups.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
			t.Cleanup(cancel)

			// The fixture records complete requests and returns 503, 503, then 200.
			// Its request reads are bounded; Shutdown joins handlers before
			// the final attempt count is inspected.
			var callsMu sync.Mutex
			var calls []modelBoundaryHTTPCall
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := modelBoundaryHTTPCall{Method: r.Method, Path: r.URL.Path}
				call.DecodeError = json.NewDecoder(r.Body).Decode(&call.Request)
				callsMu.Lock()
				calls = append(calls, call)
				attempt := len(calls)
				callsMu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if attempt <= 2 {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":{"message":"qualification transient failure","type":"server_error","code":"service_unavailable"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"fixture-completion","object":"chat.completion","model":"qualification-model","choices":[{"index":0,"message":{"role":"assistant","content":"qualified model response"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))
			}))
			server.Config.ReadTimeout = 5 * time.Second
			server.Config.WriteTimeout = 5 * time.Second
			server.Start()
			t.Cleanup(server.Close)

			cfg := loadModelBoundaryConfig(t, tc.bootstrap, testNATS.URL, server.URL)
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

			managerService, err := service.NewComponentManager(json.RawMessage(`{}`), &service.Dependencies{
				NATSClient: testNATS.Client, Manager: configManager, ComponentRegistry: registry,
				PayloadRegistry: payloads, Logger: logger,
				Platform: extractPlatformMeta(configManager.GetConfig().Get()),
			})
			require.NoError(t, err)
			manager, ok := managerService.(*service.ComponentManager)
			require.True(t, ok, "public component-manager constructor returned %T", managerService)
			stopped := false
			stop := func() error {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				return manager.Stop(stopCtx)
			}
			t.Cleanup(func() {
				if !stopped {
					require.NoError(t, stop())
				}
			})
			require.NoError(t, manager.Initialize())
			require.NoError(t, manager.Start(ctx))
			status, exists := manager.GetComponentStatus()["agentic-model"]
			require.True(t, exists, "configured model was not admitted by the product registry")
			require.Equal(t, component.StateStarted, status.State, "configured model must be running: %v", status.LastError)

			// Creating the durable pull consumer establishes response readiness;
			// the manager's completed Start establishes request-lane readiness.
			js, err := testNATS.Client.JetStream()
			require.NoError(t, err)
			stream, err := js.Stream(ctx, "AGENT")
			require.NoError(t, err)
			responses, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
				Durable: "model-boundary-responses", FilterSubject: "agent.response.>",
				AckPolicy: jetstream.AckExplicitPolicy,
			})
			require.NoError(t, err)
			request := &agentic.AgentRequest{
				RequestID: "model-boundary-request", LoopID: "c4b7d8e0-17c1-49f4-a170-ef0dbeac7b2f",
				Model: "qualification", Messages: []agentic.ChatMessage{{Role: "user", Content: "qualify the model boundary"}},
			}
			require.NoError(t, request.Validate())
			wire, err := json.Marshal(message.NewBaseMessage(request.Schema(), request, "qualification-test"))
			require.NoError(t, err)
			require.NoError(t, testNATS.Client.PublishToStream(ctx, "agent.request."+request.RequestID, wire))
			fetchCtx, fetchCancel := context.WithTimeout(ctx, 10*time.Second)
			defer fetchCancel()
			responseMsg, err := responses.Next(jetstream.FetchContext(fetchCtx))
			require.NoError(t, err, "configured model did not publish its terminal response")
			require.Equal(t, "agent.response."+request.RequestID, responseMsg.Subject())
			decoded, err := message.NewDecoder(payloads).Decode(responseMsg.Data())
			require.NoError(t, err)
			response, ok := decoded.Payload().(*agentic.AgentResponse)
			require.True(t, ok, "response must retain canonical concrete type, got %T", decoded.Payload())
			require.Equal(t, request.RequestID, response.RequestID)
			require.NoError(t, responseMsg.DoubleAck(ctx))

			// Stop joins the component's request handler before closing the
			// provider. Shutdown then joins HTTP handlers, so counts are final.
			require.NoError(t, stop())
			stopped = true
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			require.NoError(t, server.Config.Shutdown(shutdownCtx))
			callsMu.Lock()
			completedCalls := append([]modelBoundaryHTTPCall(nil), calls...)
			callsMu.Unlock()
			require.Len(t, completedCalls, tc.wantAttempts, "shipped retry setting must govern total provider attempts")
			for _, call := range completedCalls {
				require.NoError(t, call.DecodeError)
				require.Equal(t, http.MethodPost, call.Method)
				require.Equal(t, "/v1/chat/completions", call.Path)
				require.Equal(t, "qualification-model", call.Request.Model)
				require.False(t, call.Request.Stream, "fixture must exercise non-streaming completion")
				require.Equal(t, request.Messages, call.Request.Messages)
			}
			require.Equal(t, tc.wantStatus, response.Status, "terminal response: %+v", response)
			if tc.wantStatus == agentic.StatusComplete {
				require.Equal(t, "qualified model response", response.Message.Content)
				require.Equal(t, 2, response.RetryCount)
				require.Empty(t, response.Error)
			} else {
				require.Contains(t, response.Error, "qualification transient failure")
			}
		})
	}
}

type modelBoundaryHTTPCall struct {
	Method      string
	Path        string
	DecodeError error
	Request     struct {
		Model    string                `json:"model"`
		Stream   bool                  `json:"stream"`
		Messages []agentic.ChatMessage `json:"messages"`
	}
}

func loadModelBoundaryConfig(t *testing.T, path, natsURL, providerURL string) *config.Config {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg, err := config.NewLoader().LoadFromBytes(data)
	require.NoError(t, err)
	modelComponent, exists := cfg.Components["agentic-model"]
	require.True(t, exists)
	require.True(t, modelComponent.Enabled)
	agentStream, exists := cfg.Streams["AGENT"]
	require.True(t, exists)
	// The fixture is outside the composition. Declare only that admission
	// fact; keep shipped retry, transport, required flags and stream bounds.
	modelComponent.Config = modelBoundaryExternalInput(t, modelComponent.Config)
	// Isolate this boundary from other components, services and real providers.
	cfg.Components = config.ComponentConfigs{"agentic-model": modelComponent}
	cfg.Streams = config.StreamConfigs{"AGENT": agentStream}
	cfg.Services = make(types.ServiceConfigs)
	cfg.NATS.URLs = []string{natsURL}
	cfg.ModelRegistry = &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"qualification": {Provider: "openai", URL: providerURL + "/v1", Model: "qualification-model", Stream: false},
		},
		Defaults: model.DefaultsConfig{Model: "qualification"},
	}
	return cfg
}

// modelBoundaryExternalInput makes the one admission change needed when the
// shipped loop publisher is replaced by this external test publisher. Full
// bootstrap admission is covered separately by TestShippedBootstrapCompositions.
func modelBoundaryExternalInput(t *testing.T, original json.RawMessage) json.RawMessage {
	t.Helper()
	var fields, ports map[string]json.RawMessage
	var inputs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(original, &fields))
	require.NoError(t, json.Unmarshal(fields["ports"], &ports))
	require.NoError(t, json.Unmarshal(ports["inputs"], &inputs))
	var requestInput map[string]json.RawMessage
	for _, input := range inputs {
		var name string
		require.NoError(t, json.Unmarshal(input["name"], &name))
		if name == "agent.request" {
			require.Nil(t, requestInput, "expected one request input")
			requestInput = input
		}
	}
	require.NotNil(t, requestInput, "shipped model must declare agent.request")
	var required, external bool
	require.NoError(t, json.Unmarshal(requestInput["required"], &required))
	require.True(t, required, "shipped request input must remain required")
	originalExternal, hadExternal := requestInput["external"]
	if hadExternal {
		require.NoError(t, json.Unmarshal(originalExternal, &external))
	}
	require.False(t, external, "shipped request input is fed by its loop component")
	encode := func() json.RawMessage {
		var err error
		ports["inputs"], err = json.Marshal(inputs)
		require.NoError(t, err)
		fields["ports"], err = json.Marshal(ports)
		require.NoError(t, err)
		result, err := json.Marshal(fields)
		require.NoError(t, err)
		return result
	}
	requestInput["external"] = json.RawMessage(`true`)
	derived := encode()
	// Reversing that single marker must reconstruct the complete shipped
	// config; this guards the fixture delta, including all unknown fields.
	delete(requestInput, "external")
	if hadExternal {
		requestInput["external"] = originalExternal
	}
	require.JSONEq(t, string(original), string(encode()), "only request External may differ")
	return derived
}
