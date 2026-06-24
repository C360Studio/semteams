<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";

  interface Props {
    result: string | undefined;
  }

  let { result }: Props = $props();

  interface ProofFinding {
    id?: string;
    kind?: string;
    severity?: string;
    route?: string;
    reason?: string;
    claim?: string;
    dependency?: string;
    profile?: string;
    readiness?: string;
    waiver?: string;
  }

  type ProofRecord = Record<string, unknown>;

  interface ProofReport {
    run_entity_id?: string;
    status?: string;
    version?: string;
    finding_count?: number;
    findings?: ProofFinding[];
    proof_facts?: {
      dependencies?: ProofRecord[];
      readiness?: ProofRecord[];
      evidence?: ProofRecord[];
      waivers?: ProofRecord[];
    };
  }

  function isRecord(v: unknown): v is ProofRecord {
    return v !== null && typeof v === "object" && !Array.isArray(v);
  }

  function recordList(v: unknown): ProofRecord[] {
    return Array.isArray(v) ? v.filter(isRecord) : [];
  }

  function parseReport(raw: string | undefined): ProofReport | null {
    if (!raw) return null;
    try {
      const parsed: unknown = JSON.parse(raw);
      return isRecord(parsed) ? (parsed as ProofReport) : null;
    } catch {
      return null;
    }
  }

  function text(v: unknown, fallback = ""): string {
    if (typeof v === "string") return v;
    if (typeof v === "number" || typeof v === "boolean") return String(v);
    return fallback;
  }

  function textList(v: unknown): string[] {
    if (!Array.isArray(v)) return [];
    return v
      .map((item) => text(item))
      .filter((item) => item.trim().length > 0);
  }

  function statusTone(status: string | undefined): string {
    switch (status) {
      case "passed":
      case "ready":
      case "active":
        return "ok";
      case "failed":
      case "missing":
      case "expired":
      case "revoked":
        return "bad";
      case "stale":
      case "blocked":
      case "waived":
      case "ambiguous":
        return "warn";
      default:
        return "neutral";
    }
  }

  function isExpired(raw: unknown): boolean {
    const value = text(raw);
    if (!value) return false;
    const ts = Date.parse(value);
    return Number.isFinite(ts) && ts < Date.now();
  }

  function readinessFreshness(record: ProofRecord): string {
    const status = text(record.status, "unknown");
    if (status === "failed") return "failed";
    if (status === "blocked") return "blocked";
    if (status === "stale" || isExpired(record.expires_at)) return "stale";
    if (status === "passed") return "fresh";
    return "unknown";
  }

  function findingFallback(field: keyof ProofFinding): ProofRecord[] {
    const seen = new SvelteSet<string>();
    const out: ProofRecord[] = [];
    for (const finding of findings) {
      const value = finding[field];
      if (!value || seen.has(value)) continue;
      seen.add(value);
      out.push({
        id: value,
        status: finding.severity,
        route: finding.route,
        reason: finding.reason,
      });
    }
    return out;
  }

  const report = $derived(parseReport(result));
  const facts = $derived(report?.proof_facts ?? {});
  const findings = $derived(report?.findings ?? []);
  const dependencies = $derived.by(() => {
    const explicit = recordList(facts.dependencies);
    return explicit.length > 0 ? explicit : findingFallback("dependency");
  });
  const readinessRecords = $derived.by(() => {
    const explicit = recordList(facts.readiness);
    return explicit.length > 0 ? explicit : findingFallback("readiness");
  });
  const evidenceRecords = $derived(recordList(facts.evidence));
  const waiverRecords = $derived.by(() => {
    const explicit = recordList(facts.waivers);
    return explicit.length > 0 ? explicit : findingFallback("waiver");
  });
</script>

<article class="proof-readiness" data-testid="proof-readiness-card">
  <header class="proof-header">
    <div>
      <p class="proof-eyebrow">Readiness Gate</p>
      <h4 class="proof-title">Claim Analysis</h4>
    </div>
    <span
      class="proof-status"
      data-testid="proof-status"
      data-status={report?.status ?? "unknown"}
      data-tone={statusTone(report?.status)}
    >
      {report?.status ?? "unknown"}
    </span>
  </header>

  {#if report}
    <dl class="proof-summary">
      {#if report.run_entity_id}
        <div>
          <dt>Run</dt>
          <dd>{report.run_entity_id}</dd>
        </div>
      {/if}
      <div>
        <dt>Findings</dt>
        <dd>{report.finding_count ?? findings.length}</dd>
      </div>
      {#if report.version}
        <div>
          <dt>Analyzer</dt>
          <dd>{report.version}</dd>
        </div>
      {/if}
    </dl>

    <div class="proof-sections">
      <section class="proof-section" data-testid="proof-dependencies-card">
        <h5>Dependencies</h5>
        {#if dependencies.length === 0}
          <p class="empty">No proof dependencies recorded.</p>
        {:else}
          <ul>
            {#each dependencies as dependency, idx (text(dependency.id, String(idx)))}
              <li>
                <div class="row-head">
                  <span class="item-id">{text(dependency.id, "dependency")}</span>
                  <span class="status-pill" data-tone={statusTone(text(dependency.status))}>
                    {text(dependency.status, "unknown")}
                  </span>
                </div>
                {#if text(dependency.description)}
                  <p>{text(dependency.description)}</p>
                {/if}
                <dl class="mini-facts">
                  {#if text(dependency.kind)}
                    <div><dt>Kind</dt><dd>{text(dependency.kind)}</dd></div>
                  {/if}
                  {#if text(dependency.profile_ref)}
                    <div><dt>Profile</dt><dd>{text(dependency.profile_ref)}</dd></div>
                  {/if}
                  {#if text(dependency.next_route) || text(dependency.route)}
                    <div><dt>Route</dt><dd>{text(dependency.next_route) || text(dependency.route)}</dd></div>
                  {/if}
                </dl>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="proof-section" data-testid="proof-readiness-records-card">
        <h5>Readiness</h5>
        {#if readinessRecords.length === 0}
          <p class="empty">No readiness records recorded.</p>
        {:else}
          <ul>
            {#each readinessRecords as readiness, idx (text(readiness.id, String(idx)))}
              {@const freshness = readinessFreshness(readiness)}
              <li>
                <div class="row-head">
                  <span class="item-id">{text(readiness.id, "readiness")}</span>
                  <span class="status-pill" data-tone={statusTone(freshness)}>
                    {freshness}
                  </span>
                </div>
                <dl class="mini-facts">
                  {#if text(readiness.status)}
                    <div><dt>Status</dt><dd>{text(readiness.status)}</dd></div>
                  {/if}
                  {#if text(readiness.smoke_status)}
                    <div><dt>Smoke</dt><dd>{text(readiness.smoke_status)}</dd></div>
                  {/if}
                  {#if text(readiness.expires_at)}
                    <div><dt>Expires</dt><dd>{text(readiness.expires_at)}</dd></div>
                  {/if}
                  {#if text(readiness.attestation_ref)}
                    <div><dt>Attestation</dt><dd>{text(readiness.attestation_ref)}</dd></div>
                  {/if}
                </dl>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="proof-section" data-testid="proof-evidence-card">
        <h5>Evidence</h5>
        {#if evidenceRecords.length === 0}
          <p class="empty">No evidence records recorded.</p>
        {:else}
          <ul>
            {#each evidenceRecords as evidence, idx (text(evidence.id, String(idx)))}
              <li>
                <div class="row-head">
                  <span class="item-id">{text(evidence.id, "evidence")}</span>
                  <span class="status-pill" data-tone="neutral">
                    {text(evidence.kind, "record")}
                  </span>
                </div>
                <dl class="mini-facts">
                  {#if text(evidence.created_at)}
                    <div><dt>Created</dt><dd>{text(evidence.created_at)}</dd></div>
                  {/if}
                  {#if text(evidence.producer)}
                    <div><dt>Producer</dt><dd>{text(evidence.producer)}</dd></div>
                  {/if}
                  {#if text(evidence.exit_code)}
                    <div><dt>Exit</dt><dd>{text(evidence.exit_code)}</dd></div>
                  {/if}
                  {#if text(evidence.uri)}
                    <div><dt>URI</dt><dd>{text(evidence.uri)}</dd></div>
                  {/if}
                </dl>
                {#if textList(evidence.covers).length > 0}
                  <p class="covers">Covers {textList(evidence.covers).join(", ")}</p>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="proof-section" data-testid="proof-waivers-card">
        <h5>Waivers</h5>
        {#if waiverRecords.length === 0}
          <p class="empty">No waivers recorded.</p>
        {:else}
          <ul>
            {#each waiverRecords as waiver, idx (text(waiver.id, String(idx)))}
              <li>
                <div class="row-head">
                  <span class="item-id">{text(waiver.id, "waiver")}</span>
                  <span class="status-pill" data-tone={statusTone(text(waiver.status))}>
                    {text(waiver.status, "unknown")}
                  </span>
                </div>
                {#if text(waiver.reason)}
                  <p>{text(waiver.reason)}</p>
                {/if}
                <dl class="mini-facts">
                  {#if text(waiver.expires_at)}
                    <div><dt>Expires</dt><dd>{text(waiver.expires_at)}</dd></div>
                  {/if}
                  {#if text(waiver.approved_by)}
                    <div><dt>Approved by</dt><dd>{text(waiver.approved_by)}</dd></div>
                  {/if}
                  {#if text(waiver.residual_risk)}
                    <div><dt>Risk</dt><dd>{text(waiver.residual_risk)}</dd></div>
                  {/if}
                </dl>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>

    {#if findings.length > 0}
      <section class="proof-findings" data-testid="proof-findings-card">
        <h5>Findings</h5>
        <ul>
          {#each findings as finding, idx (finding.id ?? String(idx))}
            <li>
              <span class="item-id">{finding.kind ?? "finding"}</span>
              {#if finding.reason}
                <span>{finding.reason}</span>
              {/if}
              {#if finding.route}
                <span class="route">route: {finding.route}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {:else}
    <p class="empty">Readiness Gate result was not valid JSON.</p>
  {/if}
</article>

<style>
  .proof-readiness {
    display: flex;
    flex-direction: column;
    gap: 0.625rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    padding: 0.625rem 0.75rem;
    font-size: 0.8125rem;
  }

  .proof-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .proof-eyebrow {
    margin: 0 0 0.125rem;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
    letter-spacing: 0;
  }

  .proof-title {
    margin: 0;
    font-size: 0.875rem;
    line-height: 1.3;
    color: var(--ui-text-primary, #111827);
  }

  .proof-status,
  .status-pill {
    display: inline-flex;
    align-items: center;
    min-height: 1.25rem;
    padding: 0.125rem 0.375rem;
    border-radius: 4px;
    font-size: 0.6875rem;
    font-weight: 650;
    text-transform: uppercase;
    letter-spacing: 0;
    white-space: nowrap;
  }

  [data-tone="ok"] {
    background: color-mix(in srgb, var(--status-success, #22c55e) 14%, transparent);
    color: var(--status-success, #15803d);
  }

  [data-tone="warn"] {
    background: color-mix(in srgb, var(--status-warning, #f59e0b) 16%, transparent);
    color: var(--status-warning, #92400e);
  }

  [data-tone="bad"] {
    background: color-mix(in srgb, var(--status-error, #ef4444) 13%, transparent);
    color: var(--status-error, #b91c1c);
  }

  [data-tone="neutral"] {
    background: var(--ui-surface-tertiary, #e5e7eb);
    color: var(--ui-text-secondary, #6b7280);
  }

  .proof-summary,
  .mini-facts {
    display: grid;
    gap: 0.25rem 0.75rem;
    margin: 0;
  }

  .proof-summary {
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
  }

  .proof-summary div,
  .mini-facts div {
    min-width: 0;
  }

  dt {
    margin: 0;
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.6875rem;
  }

  dd {
    margin: 0;
    color: var(--ui-text-secondary, #4b5563);
    overflow-wrap: anywhere;
  }

  .proof-sections {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
    gap: 0.5rem;
  }

  .proof-section,
  .proof-findings {
    padding-top: 0.5rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
    min-width: 0;
  }

  h5 {
    margin: 0 0 0.375rem;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  li {
    min-width: 0;
  }

  .row-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    min-width: 0;
  }

  .item-id {
    color: var(--ui-text-primary, #111827);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    overflow-wrap: anywhere;
  }

  p {
    margin: 0.25rem 0 0;
    color: var(--ui-text-secondary, #4b5563);
  }

  .empty {
    margin: 0;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
  }

  .covers,
  .route {
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.75rem;
  }

  .proof-findings li {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }
</style>
