# Known Limitations

This document tracks known limitations and planned improvements.

## NATS Clustering

**Status**: Not supported in MVP

**Issue**: The config structure accepts `nats.urls` as an array, but only the first URL is used.

**Code path**:

- `config/config.go`: `NATSConfig.URLs []string` - accepts array
- `cmd/semstreams/main.go:431`: `natsURL = cfg.NATS.URLs[0]` - only first used
- `natsclient/client.go:133`: `NewClient(url string)` - single URL parameter

**Workaround**: Use a NATS gateway or load balancer in front of your cluster.

**Rationale**: MVP focuses on edge/offline-first deployments where single-node NATS is typical. Clustering support is planned for future releases.

## ObjectStore Position in E2E Configs

**Status**: Bug in e2e configs

**Issue**: In `configs/semantic-kitchen-sink.json`, ObjectStore subscribes to `events.graph.entity.>` which is AFTER Graph processing.

**Intended**: ObjectStore should subscribe to raw document subjects (e.g., `raw.document.corpus`) to store content BEFORE Graph extracts triples.

**Impact**: Raw document content may not be persisted for later retrieval when needed.

**Code path**:

- `configs/semantic-kitchen-sink.json`: ObjectStore `inputs.subject` = `events.graph.entity.>`
- Should be: `raw.document.corpus` or similar pre-Graph subject

**Fix**: Update ObjectStore config to subscribe to raw document subjects before Graph processing.

---

## Future Improvements

To properly support NATS clustering:

1. Modify `NewClient()` to accept multiple URLs
2. Join URLs into comma-separated string for `nats.Connect()`
3. Update `main.go` to pass all URLs, not just the first
4. Add clustering documentation and test cases
