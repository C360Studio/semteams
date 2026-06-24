<script lang="ts">
  import type { MetricsEvidence, RunHealth } from "$lib/utils/runHealth";

  interface Props {
    health: RunHealth;
    compact?: boolean;
  }

  let { health, compact = false }: Props = $props();

  const metricSignals = $derived(health.metrics?.signals.slice(0, 4) ?? []);

  function formatAge(ms: number | null): string {
    if (ms === null) return "no sample";
    if (ms < 1000) return "now";
    if (ms < 60_000) return `${Math.round(ms / 1000)}s`;
    return `${Math.round(ms / 60_000)}m`;
  }

  function formatMetric(value: number | null, suffix = ""): string {
    if (value === null) return "-";
    if (value === 0) return `0${suffix}`;
    if (value < 0.01) return `<0.01${suffix}`;
    return `${value.toLocaleString("en-US", { maximumFractionDigits: 2 })}${suffix}`;
  }

  function metricsLabel(metrics: MetricsEvidence): string {
    if (metrics.freshness === "fresh") return `fresh (${metrics.metricCount} samples)`;
    if (metrics.freshness === "stale") return `stale (${formatAge(metrics.scrapeAgeMs)} old)`;
    return "unavailable";
  }
</script>

<article
  class="run-health"
  class:compact
  class:working={health.state === "working"}
  class:waiting={health.state === "waiting"}
  class:blocked={health.state === "blocked"}
  class:failing={health.state === "failing"}
  class:complete={health.state === "complete"}
  data-testid={compact ? "run-health-badge" : "run-health-panel"}
  aria-label="Run health: {health.label}"
>
  <div class="health-head">
    <span class="health-dot" aria-hidden="true"></span>
    <span class="health-label">{health.label}</span>
    <span class="health-gate">{health.currentGate}</span>
  </div>

  {#if !compact}
    <dl class="health-details">
      <div>
        <dt>Next</dt>
        <dd>{health.nextAction}</dd>
      </div>
      <div>
        <dt>Evidence</dt>
        <dd>{health.evidenceFreshness}</dd>
      </div>
      <div>
        <dt>Active loops</dt>
        <dd>{health.activeLoopCount}</dd>
      </div>
      {#if health.metrics}
        <div>
          <dt>Metrics</dt>
          <dd>{metricsLabel(health.metrics)}</dd>
        </div>
      {/if}
    </dl>
    <p class="health-detail">{health.detail}</p>

    {#if health.metrics}
      <section class="metrics-evidence" data-testid="run-metrics-evidence" aria-label="Prometheus evidence">
        <div class="metrics-head">
          <h4>Prometheus</h4>
          <span class="metrics-freshness">{health.metrics.freshness}</span>
        </div>
        <dl class="metrics-grid">
          <div>
            <dt>Scrape age</dt>
            <dd>{formatAge(health.metrics.scrapeAgeMs)}</dd>
          </div>
          <div>
            <dt>Components</dt>
            <dd>
              {health.metrics.errorComponentCount} error /
              {health.metrics.degradedComponentCount} degraded
            </dd>
          </div>
          <div>
            <dt>Queue</dt>
            <dd>{formatMetric(health.metrics.queueDepthMax)}</dd>
          </div>
          <div>
            <dt>Latency</dt>
            <dd>{formatMetric(health.metrics.latencyMsMax, "ms")}</dd>
          </div>
          <div>
            <dt>Error rate</dt>
            <dd>{formatMetric(health.metrics.errorRateMax, "/s")}</dd>
          </div>
        </dl>
        {#if metricSignals.length > 0}
          <ul class="metric-signals" data-testid="run-metric-signals">
            {#each metricSignals as signal (`${signal.kind}-${signal.component ?? ""}-${signal.metricName ?? signal.label}`)}
              <li class:critical={signal.severity === "critical"}>
                <span>
                  <strong class="severity-label">{signal.severity}</strong>
                  {signal.label}
                </span>
                <small>{signal.detail}</small>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="metrics-empty">No operational pressure signal in the latest metrics.</p>
        {/if}
      </section>
    {/if}
  {/if}
</article>

<style>
  .run-health {
    --health-color: var(--ui-interactive-primary, #2563eb);
    border: 1px solid color-mix(in srgb, var(--health-color) 32%, var(--ui-border-subtle, #e5e7eb));
    border-radius: 6px;
    background: color-mix(in srgb, var(--health-color) 8%, var(--ui-surface-primary, #fff));
    color: var(--ui-text-primary, #111827);
    padding: 0.625rem;
  }

  .run-health.working {
    --health-color: #2563eb;
  }

  .run-health.waiting {
    --health-color: #f97316;
  }

  .run-health.blocked {
    --health-color: #a16207;
  }

  .run-health.failing {
    --health-color: #dc2626;
  }

  .run-health.complete {
    --health-color: #16a34a;
  }

  .run-health.compact {
    padding: 0.1875rem 0.5rem;
    border-radius: 9999px;
    align-self: flex-start;
  }

  .health-head {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    min-width: 0;
  }

  .health-dot {
    width: 0.4375rem;
    height: 0.4375rem;
    border-radius: 50%;
    background: var(--health-color);
    flex: none;
  }

  .health-label {
    font-size: 0.75rem;
    font-weight: 700;
    color: var(--health-color);
    white-space: nowrap;
  }

  .health-gate {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .health-details {
    margin: 0.5rem 0 0;
    display: grid;
    gap: 0.375rem;
  }

  .health-details div {
    display: grid;
    grid-template-columns: 4.5rem 1fr;
    gap: 0.5rem;
    align-items: baseline;
  }

  dt {
    font-size: 0.6875rem;
    font-weight: 700;
    color: var(--ui-text-tertiary, #6b7280);
    text-transform: uppercase;
  }

  dd {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
    line-height: 1.35;
  }

  .health-detail {
    margin: 0.5rem 0 0;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #4b5563);
    line-height: 1.4;
  }

  .metrics-evidence {
    margin-top: 0.625rem;
    padding-top: 0.625rem;
    border-top: 1px solid color-mix(in srgb, var(--health-color) 20%, var(--ui-border-subtle, #e5e7eb));
  }

  .metrics-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  h4 {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
  }

  .metrics-freshness {
    margin-left: auto;
    font-size: 0.6875rem;
    font-weight: 700;
    color: var(--ui-text-secondary, #4b5563);
    text-transform: uppercase;
  }

  .metrics-grid {
    margin: 0.5rem 0 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.375rem 0.5rem;
  }

  .metrics-grid div {
    min-width: 0;
  }

  .metric-signals {
    margin: 0.5rem 0 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.25rem;
  }

  .metric-signals li {
    display: grid;
    gap: 0.0625rem;
    padding-left: 0.5rem;
    border-left: 2px solid #f59e0b;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
  }

  .metric-signals li.critical {
    border-left-color: #dc2626;
  }

  .severity-label {
    margin-right: 0.25rem;
    text-transform: uppercase;
    font-size: 0.625rem;
    color: var(--ui-text-secondary, #4b5563);
  }

  .metric-signals small,
  .metrics-empty {
    font-size: 0.6875rem;
    color: var(--ui-text-tertiary, #6b7280);
    line-height: 1.35;
  }

  .metrics-empty {
    margin: 0.5rem 0 0;
  }
</style>
