# Local Monitoring

Run Prometheus and Grafana locally to monitor SemStreams during development and e2e testing.

## Quick Start

```bash
# Start observability stack
task services:start:observability

# Open dashboards
open http://localhost:3000  # Grafana (admin/admin)
open http://localhost:9090  # Prometheus
```

Stop when done:

```bash
task services:stop
```

## Architecture

```text
SemStreams (:9090/metrics) --> Prometheus (:9090) --> Grafana (:3000)
     |                              |                      |
  Exposes metrics            Scrapes every 10s      Visualizes data
```

All SemStreams components expose metrics at a single `/metrics` endpoint on port 9090. Prometheus scrapes this endpoint and stores time-series data. Grafana queries Prometheus and renders dashboards.

## Dashboards

Four pre-configured dashboards auto-load on startup:

| Dashboard | Purpose | Key Panels |
|-----------|---------|------------|
| **SemStreams Overview** | System health at a glance | Service status, throughput, error rate, latency percentiles |
| **IndexManager Metrics** | Index operations | Backlog size, update rates, query latency by index type |
| **Graph Processor** | Graph processing | Entity processing rate, triple operations, community updates |
| **Cache Performance** | Cache efficiency | Hit/miss ratios, eviction rates, memory usage |

Access dashboards: Grafana sidebar > Dashboards > SemStreams folder.

## Key Metrics Reference

### Health Indicators

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `up{job="semstreams"}` | Service availability (1=up, 0=down) | < 1 |
| `semstreams_messages_failed_total` | Failed message count | Rate > 0 sustained |
| `indexmanager_backlog_size` | Pending index updates | > 1000 |

### Throughput

| Metric | Description | Typical Range |
|--------|-------------|---------------|
| `semstreams_messages_total` | Total messages received | Varies by load |
| `semstreams_messages_processed_total` | Successfully processed | Should track total |
| `indexmanager_updates_total` | Index update operations | 1-10x message rate |

### Latency

| Metric | Description | Target |
|--------|-------------|--------|
| `semstreams_processing_duration_seconds` | End-to-end processing time | p95 < 100ms |
| `indexmanager_query_duration_seconds` | Index query latency | p95 < 10ms |
| `indexmanager_update_duration_seconds` | Index update latency | p95 < 50ms |

### Index-Specific

| Metric | Description |
|--------|-------------|
| `indexmanager_size{index="..."}` | Entries per index type |
| `indexmanager_queries_total{index="..."}` | Queries per index |
| `indexmanager_updates_total{index="..."}` | Updates per index |

Index types: `predicate`, `incoming`, `outgoing`, `alias`, `spatial`, `temporal`, `structural`, `embedding`, `community`.

### Rules Engine

| Metric | Description |
|--------|-------------|
| `semstreams_rule_evaluations_total` | Total rule evaluations |
| `semstreams_rule_triggers_total{rule="..."}` | Triggers per rule |
| `semstreams_rule_state_transitions_total` | State machine transitions |

## Running with E2E Tests

Start observability before running e2e tests to watch metrics in real-time:

```bash
# Terminal 1: Start observability
task services:start:observability

# Terminal 2: Run e2e tests
task e2e:core:default
# or
task e2e:tiers
```

Watch the Grafana dashboards during test execution to see:
- Message throughput spikes as test data flows
- Index backlog rising then draining
- Processing latency under load

## Prometheus Direct Queries

For ad-hoc analysis, query Prometheus directly at `http://localhost:9090`:

```promql
# Messages per second (5m average)
rate(semstreams_messages_total[5m])

# Error rate percentage
rate(semstreams_messages_failed_total[5m]) / rate(semstreams_messages_total[5m]) * 100

# p99 processing latency
histogram_quantile(0.99, rate(semstreams_processing_duration_seconds_bucket[5m]))

# Index backlog by type
indexmanager_backlog_size
```

## Configuration

### Prometheus Scrape Config

Located at `configs/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'semstreams'
    static_configs:
      - targets: ['host.docker.internal:9090']
    scrape_interval: 10s
```

For Docker network setups (e.g., running SemStreams in a container), use the service name instead:

```yaml
- targets: ['semstreams:9090']
```

### Grafana Provisioning

Dashboards auto-load from `configs/grafana/dashboards/`. To add custom dashboards:

1. Create JSON dashboard in Grafana UI
2. Export via Share > Export > Save to file
3. Place in `configs/grafana/dashboards/`
4. Restart Grafana (or wait 10s for auto-reload)

## Troubleshooting

### "No data" in Grafana

1. Check SemStreams is running and exposing metrics:
   ```bash
   curl http://localhost:9090/metrics | head -20
   ```

2. Check Prometheus can reach the target:
   - Open `http://localhost:9090/targets`
   - Look for `semstreams` job status

3. Verify time range in Grafana (top-right) includes recent data

### Prometheus can't scrape metrics

If running SemStreams on host (not in Docker):
- Use `host.docker.internal:9090` in prometheus.yml (macOS/Windows)
- Use `172.17.0.1:9090` on Linux

If running SemStreams in Docker:
- Ensure both containers are on the same network
- Use container/service name instead of localhost

### Grafana login issues

Default credentials: `admin` / `admin`

To reset password:
```bash
docker exec -it semstreams-grafana grafana-cli admin reset-admin-password newpassword
```

## Next Steps

- [Configuration](../basics/06-configuration.md) - Capability tiers and feature flags
- [Performance](../advanced/03-performance.md) - Optimization strategies
