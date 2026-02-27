// Package agenticmodel provides the model integration processor for the SemStreams agentic system.
//
// # Overview
//
// The agentic-model processor routes agent requests to OpenAI-compatible LLM endpoints.
// It receives AgentRequest messages from the loop orchestrator, calls the appropriate
// model endpoint, and publishes AgentResponse messages back. The processor supports
// tool calling, retry with backoff, and token tracking. Model endpoints are
// resolved from the unified model registry (component.Dependencies.ModelRegistry).
//
// This processor acts as the bridge between the agentic orchestration layer and
// external LLM services (OpenAI, Ollama, LiteLLM, vLLM, or any OpenAI-compatible API).
//
// # Architecture
//
// The model processor sits between the loop orchestrator and external LLM services:
//
//	┌───────────────┐     ┌────────────────┐     ┌──────────────────┐
//	│ agentic-loop  │────▶│ agentic-model  │────▶│ LLM Endpoint     │
//	│               │     │ (this pkg)     │     │ (OpenAI, Ollama) │
//	│               │◀────│                │◀────│                  │
//	└───────────────┘     └────────────────┘     └──────────────────┘
//	  agent.request.*       HTTP/HTTPS          OpenAI-compatible
//	  agent.response.*                          /v1/chat/completions
//
// # Quick Start
//
// Configure the model registry in the top-level config and start the processor:
//
//	config := agenticmodel.Config{
//	    StreamName: "AGENT",
//	    Timeout:    "120s",
//	}
//
//	// Model endpoints are resolved from deps.ModelRegistry (set in config.model_registry)
//	rawConfig, _ := json.Marshal(config)
//	comp, err := agenticmodel.NewComponent(rawConfig, deps)
//
//	lc := comp.(component.LifecycleComponent)
//	lc.Initialize()
//	lc.Start(ctx)
//
// # Endpoint Resolution
//
// When processing an AgentRequest, the processor resolves the endpoint from
// the unified model registry by looking up the request's Model field.
// Clients are created dynamically and cached for reuse.
//
// If the resolved endpoint has SupportsTools=false, any tools in the request
// are stripped and a warning is logged.
//
// # OpenAI Compatibility
//
// The processor uses the sashabaranov/go-openai SDK and is compatible with any
// API that implements the OpenAI chat completions interface:
//
//   - OpenAI API (api.openai.com)
//   - Azure OpenAI Service
//   - Ollama (with OpenAI compatibility layer)
//   - LiteLLM proxy
//   - vLLM with OpenAI server
//   - LocalAI
//   - Any OpenAI-compatible proxy
//
// # Tool Support
//
// The processor fully supports tool calling (function calling):
//
// Incoming request with tools:
//
//	request := agentic.AgentRequest{
//	    Model: "gpt-4",
//	    Messages: []agentic.ChatMessage{
//	        {Role: "user", Content: "Read the config file"},
//	    },
//	    Tools: []agentic.ToolDefinition{
//	        {
//	            Name:        "read_file",
//	            Description: "Read file contents",
//	            Parameters: map[string]any{
//	                "type": "object",
//	                "properties": map[string]any{
//	                    "path": map[string]any{"type": "string"},
//	                },
//	            },
//	        },
//	    },
//	}
//
// Response with tool calls:
//
//	response := agentic.AgentResponse{
//	    Status: "tool_call",
//	    Message: agentic.ChatMessage{
//	        Role: "assistant",
//	        ToolCalls: []agentic.ToolCall{
//	            {ID: "call_001", Name: "read_file", Arguments: map[string]any{"path": "config.yaml"}},
//	        },
//	    },
//	}
//
// The processor converts between agentic.ToolDefinition and OpenAI's function schema
// format automatically.
//
// # Response Status Mapping
//
// The processor maps OpenAI finish reasons to agentic status:
//
//   - "stop" → "complete" (normal completion)
//   - "length" → "complete" (max tokens reached)
//   - "tool_calls" → "tool_call" (model wants to use tools)
//   - Any error → "error" with error message
//
// # Retry Logic
//
// The processor implements retry with exponential backoff:
//
//   - Default: 3 attempts
//   - Backoff: 100ms, 200ms, 400ms (exponential)
//   - Retryable: Network errors, 5xx responses
//   - Non-retryable: Context cancellation, 4xx responses
//
// Configuration:
//
//	"retry": {
//	    "max_attempts": 3,
//	    "backoff": "exponential"
//	}
//
// # Token Tracking
//
// Every response includes token usage for cost monitoring and rate limiting:
//
//	response.TokenUsage.PromptTokens     // Input tokens
//	response.TokenUsage.CompletionTokens // Output tokens
//	response.TokenUsage.Total()          // Sum of both
//
// Token counts come directly from the LLM provider's response.
//
// # Configuration Reference
//
// Full configuration schema (endpoints are in the top-level model_registry):
//
//	{
//	    "timeout": "string (default: 120s)",
//	    "stream_name": "string (default: AGENT)",
//	    "consumer_name_suffix": "string (optional)",
//	    "retry": {
//	        "max_attempts": "int (default: 3)",
//	        "backoff": "string (default: exponential)"
//	    },
//	    "ports": {
//	        "inputs": [...],
//	        "outputs": [...]
//	    }
//	}
//
// # Ports
//
// Input ports (JetStream consumers):
//
//   - agent.request: Agent requests from agentic-loop (subject: agent.request.>)
//
// Output ports (JetStream publishers):
//
//   - agent.response: Model responses to agentic-loop (subject: agent.response.*)
//
// # Message Flow
//
// The processor handles each request through:
//
//  1. Receive AgentRequest from agent.request.>
//  2. Resolve endpoint by model name
//  3. Convert AgentRequest to OpenAI format
//  4. Call LLM endpoint with retry logic
//  5. Convert OpenAI response to AgentResponse
//  6. Publish to agent.response.{request_id}
//  7. Acknowledge JetStream message
//
// # Client Architecture
//
// The processor dynamically creates and caches Client instances per endpoint:
//
//	client, err := NewClient(endpointConfig)
//	response, err := client.ChatCompletion(ctx, request)
//
// Clients are cached by URL|Model key with mutex protection for concurrent access.
// Clients wrap the go-openai SDK and handle:
//
//   - API key injection from environment variables
//   - Request/response type conversion
//   - Retry with exponential backoff
//   - Context cancellation propagation
//
// # Error Handling
//
// Errors are returned as AgentResponse with status="error":
//
//	response := agentic.AgentResponse{
//	    RequestID: "req_123",
//	    Status:    "error",
//	    Error:     "endpoint not found: unknown-model",
//	}
//
// Error categories:
//
//   - Endpoint resolution errors: Model not found in registry
//   - Request validation errors: Invalid request format
//   - Network errors: Connection failures (may retry)
//   - API errors: 4xx/5xx from LLM provider
//   - Timeout errors: Request exceeded timeout
//
// # Environment Variables
//
// API keys are read from environment variables specified in endpoint config:
//
//	export OPENAI_API_KEY="sk-..."
//	export ANTHROPIC_API_KEY="..."
//
// Endpoint config:
//
//	{
//	    "url": "https://api.openai.com/v1/chat/completions",
//	    "model": "gpt-4",
//	    "api_key_env": "OPENAI_API_KEY"
//	}
//
// If api_key_env is not specified, requests are made without authentication
// (suitable for local models like Ollama).
//
// # Thread Safety
//
// The Component is safe for concurrent use after Start() is called.
// Multiple goroutines can process requests concurrently. Each request
// creates its own Client instance, avoiding shared state issues.
//
// # Testing
//
// For testing, use the ConsumerNameSuffix config option and provide a model registry:
//
//	config := agenticmodel.Config{
//	    StreamName:         "AGENT",
//	    ConsumerNameSuffix: "test-" + t.Name(),
//	}
//
//	// Provide endpoints via model registry in deps.ModelRegistry
//	deps.ModelRegistry = &model.Registry{
//	    Endpoints: map[string]*model.EndpointConfig{
//	        "test-model": {URL: mockServer.URL, Model: "test-model", MaxTokens: 128000},
//	    },
//	    Defaults: model.DefaultsConfig{Model: "test-model"},
//	}
//
// Use httptest.Server to mock the LLM endpoint in tests.
//
// # Limitations
//
// Current limitations:
//
//   - No streaming support (responses are complete documents)
//   - Retry configuration is global, not per-endpoint
//   - No request queuing or rate limiting
//
// # See Also
//
// Related packages:
//
//   - agentic: Shared types (AgentRequest, AgentResponse, etc.)
//   - processor/agentic-loop: Loop orchestration
//   - processor/agentic-tools: Tool execution
//   - github.com/sashabaranov/go-openai: OpenAI SDK
package agenticmodel
