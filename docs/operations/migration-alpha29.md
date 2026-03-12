# Migration Guide: alpha.28 → alpha.29

## Summary

Graph components now resolve LLM and embedding endpoints through the centralized model registry instead of per-component config fields. Three config fields have been removed. The `reactive-workflow` component has been removed from statistical and semantic tier configs.

## Breaking Changes

### Removed Config Fields

| Component | Removed Field | Replacement |
|-----------|--------------|-------------|
| `graph-clustering` | `llm_endpoint` | `model_registry.capabilities.community_summary` |
| `graph-clustering` | `llm_model` | `model_registry.endpoints.<name>.model` |
| `graph-embedding` | `embedder_url` | `model_registry.capabilities.embedding` |

Configs containing these fields will fail schema validation on startup.

### Removed Component from Tier Configs

`reactive-workflow` has been removed from `configs/statistical.json` and `configs/semantic.json`. It remains in `configs/e2e-structural.json` where it is actively used.

## Migration Steps

### 1. Remove deprecated fields

Delete from your `graph-clustering` config:
```diff
 "graph-clustering": {
   "config": {
     "enable_llm": true,
-    "llm_endpoint": "http://localhost:11434/v1",
-    "llm_model": "mistral-7b-instruct",
     ...
   }
 }
```

Delete from your `graph-embedding` config:
```diff
 "graph-embedding": {
   "config": {
     "embedder_type": "http",
-    "embedder_url": "http://localhost:8082",
     ...
   }
 }
```

### 2. Add model registry entries

If not already present, add endpoints and capabilities to your `model_registry` section:

```json
{
  "model_registry": {
    "endpoints": {
      "my-llm": {
        "provider": "openai",
        "url": "http://localhost:11434/v1",
        "model": "mistral-7b-instruct",
        "max_tokens": 4096
      },
      "my-embedder": {
        "provider": "openai",
        "url": "http://localhost:8082/v1",
        "model": "all-MiniLM-L6-v2",
        "max_tokens": 0
      }
    },
    "capabilities": {
      "community_summary": { "preferred": ["my-llm"] },
      "embedding": { "preferred": ["my-embedder"] }
    },
    "defaults": { "model": "my-llm" }
  }
}
```

**Notes:**
- `max_tokens: 0` is valid for embedding endpoints (means "not applicable")
- `embedder_type: "bm25"` requires no registry entry — BM25 is pure Go, no external service
- `enable_llm: true` still controls whether graph-clustering attempts LLM enhancement; the registry provides the endpoint when it does
- `query_classification` capability is optional — graph-query falls back to keyword-only classification when absent

### 3. Remove reactive-workflow (if not using workflows)

If your config includes `reactive-workflow` but you don't define workflows against it, remove the component block:

```diff
-    "reactive-workflow": {
-      "type": "processor",
-      "name": "reactive-workflow",
-      "enabled": true,
-      "config": {
-        "enable_metrics": true,
-        "state_bucket": "REACTIVE_WORKFLOW_STATE",
-        "callback_stream_name": "WORKFLOW_CALLBACKS",
-        "event_stream_name": "WORKFLOW_EVENTS",
-        "default_timeout": "10m",
-        "default_max_iterations": 10,
-        "consumer_name_prefix": "statistical-"
-      }
-    },
```

## Capability Reference

| Capability | Used By | Purpose |
|-----------|---------|---------|
| `community_summary` | graph-clustering | LLM-generated community summaries |
| `embedding` | graph-embedding | HTTP embedding service (e.g. sentence-transformers) |
| `query_classification` | graph-query | LLM-based query classification (optional) |
| `summarization` | agentic-loop | Context compaction (existing, unchanged) |
