// Package agenticmodel provides the model integration processor for the SemStreams agentic system.
//
// # Overview
//
// The agentic-model processor routes agent requests to OpenAI-compatible LLM endpoints.
// It receives AgentRequest messages from the loop orchestrator, calls the appropriate
// model endpoint, and publishes AgentResponse messages back. The processor supports
// multiple named endpoints, tool calling, retry with backoff, and token tracking.
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
// Configure and start the processor:
//
//	config := agenticmodel.Config{
//	    StreamName: "AGENT",
//	    Endpoints: map[string]agenticmodel.Endpoint{
//	        "gpt-4": {
//	            URL:       "https://api.openai.com/v1/chat/completions",
//	            Model:     "gpt-4",
//	            APIKeyEnv: "OPENAI_API_KEY",
//	        },
//	        "ollama": {
//	            URL:   "http://localhost:11434/v1/chat/completions",
//	            Model: "llama2",
//	        },
//	    },
//	    Timeout: "120s",
//	}
//
//	rawConfig, _ := json.Marshal(config)
//	comp, err := agenticmodel.NewComponent(rawConfig, deps)
//
//	lc := comp.(component.LifecycleComponent)
//	lc.Initialize()
//	lc.Start(ctx)
//
// # Endpoint Configuration
//
// Endpoints are named configurations for different LLM services:
//
//	"endpoints": {
//	    "gpt-4": {
//	        "url": "https://api.openai.com/v1/chat/completions",
//	        "model": "gpt-4",
//	        "api_key_env": "OPENAI_API_KEY"
//	    },
//	    "gpt-3.5": {
//	        "url": "https://api.openai.com/v1/chat/completions",
//	        "model": "gpt-3.5-turbo",
//	        "api_key_env": "OPENAI_API_KEY"
//	    },
//	    "local-llama": {
//	        "url": "http://localhost:11434/v1/chat/completions",
//	        "model": "llama2"
//	    }
//	}
//
// Endpoint fields:
//
//   - url: Base URL for the OpenAI-compatible API (required)
//   - model: Model name to use in API requests (required)
//   - api_key_env: Environment variable name containing the API key (optional)
//
// # Endpoint Resolution
//
// When processing an AgentRequest, the processor resolves the endpoint by:
//
//  1. Looking for an endpoint matching the request's Model field exactly
//  2. Looking for a model alias matching the Model field, then resolving to target
//  3. Falling back to a "default" endpoint if configured
//  4. Returning an error if no matching endpoint is found
//
// # Model Aliases
//
// Model aliases provide semantic names for endpoints, allowing other components
// to reference models by purpose rather than specific endpoint name:
//
//	config := agenticmodel.Config{
//	    Endpoints: map[string]agenticmodel.Endpoint{
//	        "gpt-4": {URL: "...", Model: "gpt-4"},
//	        "gpt-3.5-turbo": {URL: "...", Model: "gpt-3.5-turbo"},
//	    },
//	    ModelAliases: map[string]string{
//	        "reasoning": "gpt-4",
//	        "coding":    "gpt-4",
//	        "fast":      "gpt-3.5-turbo",
//	    },
//	}
//
// Alias validation rules:
//
//   - Target must exist in Endpoints
//   - No alias chaining (alias cannot point to another alias)
//   - Empty target is not allowed
//
// Usage in requests:
//
//	// Using endpoint name directly
//	request := agentic.AgentRequest{Model: "gpt-4", ...}
//
//	// Using alias
//	request := agentic.AgentRequest{Model: "fast", ...}  // Resolves to gpt-3.5-turbo
//
// This allows agentic-loop, agentic-memory, and workflow components to reference
// models semantically (e.g., "fast" for summarization) without hardcoding
// specific model names.
//
// This allows routing different requests to different models/providers:
//
//	// Request using GPT-4
//	request := agentic.AgentRequest{
//	    Model: "gpt-4",  // Routes to "gpt-4" endpoint
//	    // ...
//	}
//
//	// Request using local Ollama
//	request := agentic.AgentRequest{
//	    Model: "local-llama",  // Routes to "local-llama" endpoint
//	    // ...
//	}
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
// Full configuration schema:
//
//	{
//	    "endpoints": {
//	        "<name>": {
//	            "url": "string (required)",
//	            "model": "string (required)",
//	            "api_key_env": "string (optional)"
//	        }
//	    },
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
// The processor creates Client instances per endpoint:
//
//	client, err := NewClient(endpoint)
//	response, err := client.ChatCompletion(ctx, request)
//	client.Close()
//
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
//   - Endpoint resolution errors: Unknown model name
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
// For testing, use the ConsumerNameSuffix config option:
//
//	config := agenticmodel.Config{
//	    StreamName:         "AGENT",
//	    ConsumerNameSuffix: "test-" + t.Name(),
//	    Endpoints: map[string]agenticmodel.Endpoint{
//	        "test-model": {
//	            URL:   mockServer.URL,
//	            Model: "test-model",
//	        },
//	    },
//	}
//
// Use httptest.Server to mock the LLM endpoint in tests.
//
// # Limitations
//
// Current limitations:
//
//   - No streaming support (responses are complete documents)
//   - No connection pooling per endpoint (new client per request)
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
