# Known Limitations

This document tracks known limitations and planned improvements.

## NATS Clustering

**Status**: Supported

Multiple NATS server URLs can be specified for clustering/failover:

```json
{
  "nats": {
    "urls": [
      "nats://server1:4222",
      "nats://server2:4222",
      "nats://server3:4222"
    ]
  }
}
```

Or via environment variable (comma-separated):

```bash
export SEMSTREAMS_NATS_URLS="nats://server1:4222,nats://server2:4222"
```

NATS handles automatic failover and reconnection across the cluster.

## ObjectStore Content Storage

**Status**: Supported

ObjectStore can be positioned before Graph to store raw content and emit StoredMessage with StorageRef:

```
Raw Doc → ObjectStore → StoredMessage (triples + StorageRef) → Graph
                                                              → LLM/Embedding Worker (fetches via StorageRef)
```

Configure ObjectStore with a `stored` output port to enable this flow. See `configs/semantic-kitchen-sink.json` for example.
