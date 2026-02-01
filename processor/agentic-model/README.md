# agentic-model

Model integration component for the agentic processing system.

## Overview

The `agentic-model` component routes agent requests to OpenAI-compatible LLM endpoints. It receives `AgentRequest` messages from the loop orchestrator, calls the appropriate model endpoint, and publishes `AgentResponse` messages back. Supports multiple named endpoints, tool calling, retry with backoff, and token tracking.

## Architecture

```
┌───────────────┐     ┌────────────────┐     ┌──────────────────┐
│ agentic-loop  │────►│ agentic-model  │────►│ LLM Endpoint     │
│               │     │                │     │ (OpenAI, Ollama) │
│               │◄────│                │◄────│                  │
└───────────────┘     └────────────────┘     └──────────────────┘
  agent.request.*       HTTP/HTTPS          OpenAI-compatible
  agent.response.*                          /v1/chat/completions
```

## Features

- **Multiple Endpoints**: Configure different models/providers by name
- **OpenAI Compatible**: Works with OpenAI, Ollama, LiteLLM, vLLM, etc.
- **Tool Support**: Full tool calling (function calling) support
- **Retry Logic**: Configurable retry with exponential backoff
- **Token Tracking**: Tracks prompt and completion tokens

## Configuration

```json
{
  "type": "processor",
  "name": "agentic-model",
  "enabled": true,
  "config": {
    "stream_name": "AGENT",
    "timeout": "120s",
    "endpoints": {
      "gpt-4": {
        "url": "https://api.openai.com/v1/chat/completions",
        "model": "gpt-4",
        "api_key_env": "OPENAI_API_KEY"
      },
      "ollama": {
        "url": "http://localhost:11434/v1/chat/completions",
        "model": "llama2"
      }
    },
    "retry": {
      "max_attempts": 3,
      "backoff": "exponential"
    },
    "ports": {
      "inputs": [
        {"name": "requests", "type": "jetstream", "subject": "agent.request.>", "stream_name": "AGENT"}
      ],
      "outputs": [
        {"name": "responses", "type": "jetstream", "subject": "agent.response.*", "stream_name": "AGENT"}
      ]
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `endpoints` | object | required | Named endpoint configurations |
| `timeout` | string | "120s" | Request timeout |
| `stream_name` | string | "AGENT" | JetStream stream name |
| `consumer_name_suffix` | string | "" | Suffix for consumer names (for testing) |
| `retry.max_attempts` | int | 3 | Maximum retry attempts |
| `retry.backoff` | string | "exponential" | Backoff strategy |
| `ports` | object | (defaults) | Port configuration |

### Endpoint Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | Base URL for OpenAI-compatible API |
| `model` | string | yes | Model name for API requests |
| `api_key_env` | string | no | Environment variable for API key |

### Model Aliases

Model aliases provide semantic names for endpoints, allowing other components to reference models by purpose rather than specific endpoint:

```json
{
  "model_aliases": {
    "reasoning": "gpt-4",
    "coding": "gpt-4-turbo",
    "fast": "gpt-3.5-turbo",
    "summarization": "gpt-3.5-turbo"
  }
}
```

Alias rules:

- Target must exist in `endpoints`
- No alias chaining (alias cannot point to another alias)
- Empty target is not allowed

Requests can use either endpoint names or aliases:

```json
{
  "model": "fast"
}
```

Resolves to the `gpt-3.5-turbo` endpoint.

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| requests | jetstream | agent.request.> | Agent requests from agentic-loop |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| responses | jetstream | agent.response.* | Model responses to agentic-loop |

## Endpoint Resolution

Requests are routed to endpoints by model name:

1. Exact match: Request `model: "gpt-4"` routes to `endpoints.gpt-4`
2. Alias match: Request `model: "fast"` routes to `model_aliases.fast` target
3. Default fallback: If no match, routes to `endpoints.default` (if configured)
4. Error: If no match and no default, returns error response

## Compatible Providers

| Provider | URL Format | Notes |
|----------|------------|-------|
| OpenAI | `https://api.openai.com/v1/chat/completions` | Requires API key |
| Azure OpenAI | `https://{resource}.openai.azure.com/...` | Requires API key |
| Ollama | `http://localhost:11434/v1/chat/completions` | No auth required |
| LiteLLM | `http://localhost:8000/v1/chat/completions` | Proxy for multiple providers |
| vLLM | `http://localhost:8000/v1/chat/completions` | Self-hosted models |
| LocalAI | `http://localhost:8080/v1/chat/completions` | Local models |

## Response Status Mapping

| OpenAI Finish Reason | Agentic Status | Description |
|---------------------|----------------|-------------|
| `stop` | `complete` | Normal completion |
| `length` | `complete` | Max tokens reached |
| `tool_calls` | `tool_call` | Model wants to use tools |
| (error) | `error` | API or network error |

## Environment Variables

API keys are read from environment variables:

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="..."
```

Reference in config:

```json
{
  "api_key_env": "OPENAI_API_KEY"
}
```

## Retry Behavior

- **Max Attempts**: Default 3, configurable
- **Backoff**: Exponential (100ms, 200ms, 400ms)
- **Retryable**: Network errors, 5xx responses
- **Non-retryable**: Context cancellation, 4xx responses

## Troubleshooting

### Endpoint not found

- Verify model name in request matches an endpoint key
- Add a "default" endpoint as fallback
- Check for typos in model names

### Authentication errors

- Verify `api_key_env` points to correct environment variable
- Check that environment variable is set
- Ensure API key is valid and has quota

### Timeout errors

- Increase `timeout` for complex requests
- Check network connectivity to endpoint
- Verify endpoint URL is correct

### Tool calls not working

- Ensure tools are provided in AgentRequest
- Check tool parameter schemas are valid JSON Schema
- Verify model supports tool calling

## Related Components

- [agentic-loop](../agentic-loop/) - Loop orchestration
- [agentic-tools](../agentic-tools/) - Tool execution
- [agentic types](../../agentic/) - Shared type definitions
