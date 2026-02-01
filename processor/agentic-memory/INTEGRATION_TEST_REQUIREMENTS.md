# Integration Test Requirements for agentic-memory

Builder must create integration tests (`//go:build integration`) covering:

## Extraction Integration Tests

- [ ] Extract facts from real agent response using LLM model
- [ ] Verify extracted triples are stored in graph
- [ ] Test extraction triggered by iteration interval
- [ ] Test extraction triggered by context threshold
- [ ] Verify model alias resolution (fast, accurate, default)

## Hydration Integration Tests

- [ ] Hydrate context from real graph storage (NATS KV)
- [ ] Pre-task hydration includes decisions and files
- [ ] Post-compaction hydration reconstructs from checkpoint
- [ ] Verify token limits are respected in hydration
- [ ] Test hydration with empty graph (no prior data)

## Checkpoint Integration Tests

- [ ] Create checkpoint in NATS KV bucket
- [ ] Restore from checkpoint after compaction
- [ ] Verify checkpoint retention policy
- [ ] Test concurrent checkpoint operations

## Port Integration Tests

- [ ] Subscribe to compaction_events on JetStream
- [ ] Publish injected_context to JetStream
- [ ] Publish graph_mutations via NATS
- [ ] Publish checkpoint_events via NATS
- [ ] Watch entity_states KV bucket
- [ ] Write to checkpoints KV bucket
- [ ] Request/response with model via nats-request port

## End-to-End Flow Tests

- [ ] Complete flow: compaction → extraction → graph mutation
- [ ] Complete flow: pre-task request → hydration → context injection
- [ ] Complete flow: checkpoint creation → compaction → restoration
- [ ] Verify health status reflects component state
- [ ] Verify metrics are updated (messages/sec, errors, etc.)

## Error Handling Tests

- [ ] Handle NATS connection failures
- [ ] Handle LLM model unavailable
- [ ] Handle graph query failures
- [ ] Handle KV bucket unavailable
- [ ] Verify graceful degradation when extraction disabled
- [ ] Verify graceful degradation when hydration disabled

## Notes

- Use testcontainers for NATS JetStream
- Mock LLM responses for predictable test scenarios
- Test both enabled and disabled feature flags
- Verify context cancellation throughout
- Run with `-race` flag to detect concurrency issues
