import type { AgentLoop } from "$lib/types/agent";
import { isActiveState } from "$lib/types/agent";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { RunPause } from "$lib/types/task";

export type RunHealthState = "working" | "waiting" | "blocked" | "failing" | "complete";
export type EvidenceFreshness = "fresh" | "stale" | "missing" | "unknown";
export type MetricsFreshness = "fresh" | "stale" | "unavailable";
export type MetricsSignalSeverity = "info" | "warning" | "critical";
export type MetricsSignalKind =
  | "freshness"
  | "correlation"
  | "component_pressure"
  | "queue_depth"
  | "latency"
  | "error_rate";

export type RunHealthSignalSource =
  | "lifecycle"
  | "active_loops"
  | "approvals"
  | "proof"
  | "evidence"
  | "metrics"
  | "cbg"
  | "tasks";

export interface RunHealthSignal {
  source: RunHealthSignalSource;
  label: string;
  detail?: string;
}

export interface RunMetricSample {
  component: string;
  metricName: string;
  rate: number | null;
  raw: {
    name: string;
    type: string;
    value: number;
    labels: Record<string, string>;
  };
}

export interface ComponentHealthSample {
  name: string;
  status: "healthy" | "degraded" | "error";
  message: string | null;
}

export interface MetricsEvidenceSignal {
  kind: MetricsSignalKind;
  severity: MetricsSignalSeverity;
  label: string;
  detail: string;
  component?: string;
  metricName?: string;
  value?: number;
  rate?: number;
}

export interface MetricsEvidence {
  freshness: MetricsFreshness;
  scrapeAgeMs: number | null;
  lastSampleAt: number | null;
  metricCount: number;
  componentCount: number;
  degradedComponentCount: number;
  errorComponentCount: number;
  queueDepthMax: number | null;
  latencyMsMax: number | null;
  errorRateMax: number | null;
  signals: MetricsEvidenceSignal[];
}

export interface GraphRunHealthFacts {
  runId: string;
  runEntityId: string;
  phase: string;
  outcome: string;
  formalClaimsStatus: string;
  formalClaimsFindingCount: number;
  proofReadinessRoute: string;
  proofImplementationReady: boolean;
  proofTestHarnessRequired: boolean;
  proofRequiresClarification: boolean;
  proofPauseRequired: boolean;
  devFromTaskRequested: boolean;
  cbgRetryPending: boolean;
  cbgRetryFinding: string;
  cbgRetryTargetTask: string;
  evidenceFreshness: EvidenceFreshness;
  readinessRecordCount: number;
  evidenceRecordCount: number;
  staleEvidenceCount: number;
  signals: RunHealthSignal[];
}

export interface RunHealth {
  state: RunHealthState;
  label: string;
  runEntityId?: string;
  metrics?: MetricsEvidence;
  currentGate: string;
  nextAction: string;
  detail: string;
  evidenceFreshness: EvidenceFreshness;
  activeLoopCount: number;
  signals: RunHealthSignal[];
}

interface DeriveRunHealthInput {
  runId: string;
  loops?: AgentLoop[];
  pause?: RunPause | null;
  graph?: GraphRunHealthFacts | null;
  metrics?: MetricsEvidence | null;
}

interface GraphDeriveOptions {
  now?: Date;
  evidenceStaleAfterMs?: number;
}

interface MetricsDeriveInput {
  metrics?: RunMetricSample[];
  healthComponents?: ComponentHealthSample[];
  connected?: boolean;
  error?: string | null;
  lastMetricsTimestamp?: number | null;
  now?: Date | number;
  staleAfterMs?: number;
}

const RUN_INFIX = ".agent.chain.execution.";
const DEFAULT_EVIDENCE_STALE_AFTER_MS = 24 * 60 * 60 * 1000;
const DEFAULT_METRICS_STALE_AFTER_MS = 60 * 1000;

export const RUN_HEALTH_PREDICATES = [
  "agent.run.phase",
  "agent.run.outcome",
  "formal_claims.status",
  "formal_claims.finding_count",
  "formal_claims.route.test_harness",
  "formal_claims.route.coordinator",
  "formal_claims.route.implementation",
  "proof_readiness.route",
  "proof_readiness.implementation_ready",
  "proof_readiness.test_harness_required",
  "proof_readiness.requires_clarification",
  "proof_readiness.pause_required",
  "dev_from_task.requested",
  "dev_via_test.run.status",
  "dev_via_test.cbg.retry.pending",
  "dev_via_test.cbg.retry.finding",
  "dev_via_test.cbg.retry.target_task",
] as const;

export function runIdFromEntity(subject: string): string {
  const idx = subject.indexOf(RUN_INFIX);
  return idx === -1 ? "" : subject.slice(idx + RUN_INFIX.length);
}

export function deriveGraphRunHealthFacts(
  triples: RawTriple[],
  options: GraphDeriveOptions = {},
): Map<string, GraphRunHealthFacts> {
  const now = options.now ?? new Date();
  const evidenceStaleAfterMs = options.evidenceStaleAfterMs ?? DEFAULT_EVIDENCE_STALE_AFTER_MS;
  const grouped = new Map<string, RawTriple[]>();

  for (const triple of triples) {
    const runId = runIdFromEntity(triple.subject);
    if (!runId) continue;
    const current = grouped.get(triple.subject) ?? [];
    current.push(triple);
    grouped.set(triple.subject, current);
  }

  const facts = new Map<string, GraphRunHealthFacts>();
  for (const [runEntityId, group] of grouped) {
    const runId = runIdFromEntity(runEntityId);
    const freshness = evidenceFreshness(group, now, evidenceStaleAfterMs);
    const formalClaimsStatus = valueOf(group, "formal_claims.status");
    const proofReadinessRoute = valueOf(group, "proof_readiness.route");
    const formalClaimsFindingCount = numberValue(valueOf(group, "formal_claims.finding_count"));
    const proofImplementationReady = truthy(valueOf(group, "proof_readiness.implementation_ready"));
    const proofTestHarnessRequired = truthy(valueOf(group, "proof_readiness.test_harness_required"));
    const proofRequiresClarification = truthy(valueOf(group, "proof_readiness.requires_clarification"));
    const proofPauseRequired = truthy(valueOf(group, "proof_readiness.pause_required"));
    const cbgRetryPending = valueOf(group, "dev_via_test.cbg.retry.pending") !== "";
    const cbgRetryFinding = valueOf(group, "dev_via_test.cbg.retry.finding");
    const cbgRetryTargetTask = valueOf(group, "dev_via_test.cbg.retry.target_task");
    const signals: RunHealthSignal[] = [];

    const phase = valueOf(group, "agent.run.phase");
    if (phase) signals.push({ source: "lifecycle", label: `phase ${phase}` });
    const outcome = valueOf(group, "agent.run.outcome");
    if (outcome) signals.push({ source: "lifecycle", label: `outcome ${outcome}` });
    if (formalClaimsStatus) {
      signals.push({
        source: "proof",
        label: `formal claims ${formalClaimsStatus}`,
        detail: formalClaimsFindingCount > 0 ? `${formalClaimsFindingCount} finding(s)` : undefined,
      });
    }
    if (proofReadinessRoute) {
      signals.push({ source: "proof", label: `proof route ${proofReadinessRoute}` });
    }
    if (freshness !== "unknown") {
      signals.push({ source: "evidence", label: `evidence ${freshness}` });
    }
    if (cbgRetryPending || cbgRetryFinding) {
      signals.push({
        source: "cbg",
        label: cbgRetryPending ? "CBG retry pending" : "CBG retry finding",
        detail: cbgRetryFinding || undefined,
      });
    }

    facts.set(runId, {
      runId,
      runEntityId,
      phase,
      outcome,
      formalClaimsStatus,
      formalClaimsFindingCount,
      proofReadinessRoute,
      proofImplementationReady,
      proofTestHarnessRequired,
      proofRequiresClarification,
      proofPauseRequired,
      devFromTaskRequested: truthy(valueOf(group, "dev_from_task.requested")),
      cbgRetryPending,
      cbgRetryFinding,
      cbgRetryTargetTask,
      evidenceFreshness: freshness,
      readinessRecordCount: countPredicateSuffix(group, /^proof\.readiness\.[^.]+\.status$/),
      evidenceRecordCount: countPredicateSuffix(group, /^proof\.evidence\.[^.]+\.created_at$/),
      staleEvidenceCount: countStaleEvidence(group, now, evidenceStaleAfterMs),
      signals,
    });
  }

  return facts;
}

export function deriveMetricsEvidence(input: MetricsDeriveInput = {}): MetricsEvidence {
  const metrics = input.metrics ?? [];
  const components = input.healthComponents ?? [];
  const nowMs = input.now instanceof Date ? input.now.getTime() : (input.now ?? Date.now());
  const staleAfterMs = input.staleAfterMs ?? DEFAULT_METRICS_STALE_AFTER_MS;
  const scrapeAgeMs = input.lastMetricsTimestamp === null || input.lastMetricsTimestamp === undefined
    ? null
    : Math.max(0, nowMs - input.lastMetricsTimestamp);
  const signals: MetricsEvidenceSignal[] = [];
  const correlatedSampleCount = metrics.filter(hasCorrelationLabel).length;

  let freshness: MetricsFreshness = "fresh";
  if (input.error) {
    freshness = "unavailable";
    signals.push({
      kind: "freshness",
      severity: "critical",
      label: "Metrics unavailable",
      detail: input.error,
    });
  } else if (input.connected === false || metrics.length === 0 || scrapeAgeMs === null) {
    freshness = "unavailable";
    signals.push({
      kind: "freshness",
      severity: "warning",
      label: "Metrics unavailable",
      detail: input.connected === false
        ? "Runtime stream is disconnected."
        : "No Prometheus metric samples have arrived.",
    });
  } else if (scrapeAgeMs > staleAfterMs) {
    freshness = "stale";
    signals.push({
      kind: "freshness",
      severity: "warning",
      label: "Metrics stale",
      detail: `Last metric sample is ${Math.round(scrapeAgeMs / 1000)}s old.`,
      value: scrapeAgeMs,
    });
  }
  if (metrics.length > 0 && correlatedSampleCount === 0) {
    signals.push({
      kind: "correlation",
      severity: "warning",
      label: "Metric correlation unavailable",
      detail: "No run_id, loop_id, flow_id, or component labels were present; treating metrics as global operational evidence.",
    });
  }

  let queueDepthMax: number | null = null;
  let latencyMsMax: number | null = null;
  let errorRateMax: number | null = null;
  const componentNames = new Set<string>();

  for (const component of components) {
    componentNames.add(component.name);
    if (component.status === "healthy") continue;
    signals.push({
      kind: "component_pressure",
      severity: component.status === "error" ? "critical" : "warning",
      label: `${component.name} ${component.status}`,
      detail: component.message ?? `Component health is ${component.status}.`,
      component: component.name,
    });
  }

  for (const metric of metrics) {
    componentNames.add(metric.component);
    const name = metric.metricName.toLowerCase();
    const value = finiteNumber(metric.raw.value);

    if (isQueueMetric(name)) {
      queueDepthMax = maxNullable(queueDepthMax, value);
      if (value > 0) {
        signals.push({
          kind: "queue_depth",
          severity: value >= 100 ? "critical" : "warning",
          label: `${metric.component} queue depth ${formatMetricNumber(value)}`,
          detail: metric.metricName,
          component: metric.component,
          metricName: metric.metricName,
          value,
        });
      }
    }

    if (isLatencyMetric(name)) {
      const latencyMs = metricValueAsMs(name, value);
      latencyMsMax = maxNullable(latencyMsMax, latencyMs);
      if (latencyMs >= 1000) {
        signals.push({
          kind: "latency",
          severity: latencyMs >= 5000 ? "critical" : "warning",
          label: `${metric.component} latency ${formatMetricNumber(latencyMs)}ms`,
          detail: metric.metricName,
          component: metric.component,
          metricName: metric.metricName,
          value: latencyMs,
        });
      }
    }

    if (isErrorMetric(name)) {
      const rate = finiteNumber(metric.rate ?? (metric.raw.type === "gauge" ? metric.raw.value : 0));
      errorRateMax = maxNullable(errorRateMax, rate);
      if (rate > 0) {
        signals.push({
          kind: "error_rate",
          severity: rate >= 1 ? "critical" : "warning",
          label: `${metric.component} error rate ${formatMetricNumber(rate)}/s`,
          detail: metric.metricName,
          component: metric.component,
          metricName: metric.metricName,
          rate,
        });
      }
    }
  }

  return {
    freshness,
    scrapeAgeMs,
    lastSampleAt: input.lastMetricsTimestamp ?? null,
    metricCount: metrics.length,
    componentCount: componentNames.size,
    degradedComponentCount: components.filter((c) => c.status === "degraded").length,
    errorComponentCount: components.filter((c) => c.status === "error").length,
    queueDepthMax,
    latencyMsMax,
    errorRateMax,
    signals,
  };
}

export function deriveRunHealth(input: DeriveRunHealthInput): RunHealth {
  const loops = input.loops ?? [];
  const activeLoops = loops.filter((loop) => isActiveState(loop.state));
  const failedLoop = loops.find((loop) =>
    ["failed", "error", "cancelled"].includes(loop.state) || terminalFailure(loop.outcome),
  );
  const graph = input.graph ?? null;
  const signals: RunHealthSignal[] = [...(graph?.signals ?? [])];
  const metrics = input.metrics ?? null;
  if (metrics) {
    signals.push({
      source: "metrics",
      label: `metrics ${metrics.freshness}`,
      detail: metrics.signals[0]?.label,
    });
  }

  const pause = input.pause ?? null;
  if (pause) {
    const isClarification = pause.cause === "clarification";
    signals.push({
      source: "approvals",
      label: isClarification ? "clarification pending" : "approval pending",
    });
    const detail = pause.cause === "clarification"
      ? pause.question || "A coordinator is waiting for an answer."
      : `Loop ${pause.gatedLoopId} is waiting on approval.`;
    return {
      state: "waiting",
      label: "Waiting",
      runEntityId: graph?.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: isClarification ? "Human clarification" : "Tool approval",
      nextAction: isClarification ? "Answer the coordinator question" : "Approve or reject the gated tool call",
      detail,
      evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
      activeLoopCount: activeLoops.length,
      signals,
    };
  }

  if (failedLoop || terminalFailure(graph?.outcome) || ["failed", "cancelled"].includes(graph?.phase ?? "")) {
    const loopDetail = failedLoop
      ? `${failedLoop.role || "loop"} ${failedLoop.loop_id} is ${failedLoop.state}`
      : `run outcome ${graph?.outcome || graph?.phase}`;
    signals.push({ source: "lifecycle", label: "run failure", detail: loopDetail });
    return {
      state: "failing",
      label: "Failing",
      runEntityId: graph?.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: "Failure recovery",
      nextAction: "Inspect the failing loop and decide whether to retry, amend, or stop",
      detail: loopDetail,
      evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
      activeLoopCount: activeLoops.length,
      signals,
    };
  }

  const proofBlocker = proofBlockerReason(graph);
  if (proofBlocker) {
    signals.push({ source: "proof", label: "proof blocker", detail: proofBlocker.detail });
    return {
      state: "blocked",
      label: "Blocked",
      runEntityId: graph?.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: proofBlocker.gate,
      nextAction: proofBlocker.nextAction,
      detail: proofBlocker.detail,
      evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
      activeLoopCount: activeLoops.length,
      signals,
    };
  }

  if (graph?.proofImplementationReady && !graph.devFromTaskRequested && activeLoops.length === 0) {
    signals.push({ source: "proof", label: "implementation ready" });
    return {
      state: "waiting",
      label: "Waiting",
      runEntityId: graph.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: "Implement Task request",
      nextAction: "Request implementation from the approved spec",
      detail: "Readiness Gate has released implementation, but no Implement Task request is active.",
      evidenceFreshness: graph.evidenceFreshness,
      activeLoopCount: 0,
      signals,
    };
  }

  if (activeLoops.length > 0 || ["dispatched", "executing"].includes(graph?.phase ?? "")) {
    const leadLoop = activeLoops[0];
    signals.push({
      source: "active_loops",
      label: activeLoops.length > 0 ? `${activeLoops.length} active loop(s)` : "run executing",
      detail: leadLoop ? `${leadLoop.role || "loop"} is ${leadLoop.state}` : undefined,
    });
    return {
      state: "working",
      label: "Working",
      runEntityId: graph?.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: leadLoop ? `${leadLoop.role || "Agent"} ${leadLoop.state}` : "Run executing",
      nextAction: "Wait for the current loop or gate to emit evidence",
      detail: leadLoop ? `${leadLoop.role || "Agent"} is ${leadLoop.state}.` : "The run lifecycle is executing.",
      evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
      activeLoopCount: activeLoops.length,
      signals,
    };
  }

  if (terminalSuccess(graph?.outcome) || graph?.phase === "completed" || loopsComplete(loops)) {
    signals.push({ source: "lifecycle", label: "run complete" });
    return {
      state: "complete",
      label: "Complete",
      runEntityId: graph?.runEntityId,
      metrics: metrics ?? undefined,
      currentGate: "Accepted",
      nextAction: "Review the final artifact or export evidence",
      detail: "No active blockers are visible and the run is terminal.",
      evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
      activeLoopCount: 0,
      signals,
    };
  }

  return {
    state: "working",
    label: "Working",
    runEntityId: graph?.runEntityId,
    metrics: metrics ?? undefined,
    currentGate: "Starting",
    nextAction: "Wait for the first lifecycle, proof, or loop signal",
    detail: `Run ${input.runId} has no blocking or terminal signal yet.`,
    evidenceFreshness: graph?.evidenceFreshness ?? "unknown",
    activeLoopCount: activeLoops.length,
    signals,
  };
}

function finiteNumber(value: number): number {
  return Number.isFinite(value) ? value : 0;
}

function maxNullable(current: number | null, value: number): number {
  return current === null ? value : Math.max(current, value);
}

function isQueueMetric(name: string): boolean {
  return (
    name.includes("queue") ||
    name.includes("backlog") ||
    name.includes("pending")
  ) && (
    name.includes("depth") ||
    name.includes("size") ||
    name.includes("length") ||
    name.includes("pending") ||
    name.includes("backlog")
  );
}

function isLatencyMetric(name: string): boolean {
  return (
    name.includes("latency") ||
    name.includes("duration") ||
    name.includes("elapsed")
  ) && !name.endsWith("_bucket");
}

function isErrorMetric(name: string): boolean {
  return (
    name.includes("error") ||
    name.includes("fail") ||
    name.includes("reject")
  ) && !name.endsWith("_bucket");
}

function hasCorrelationLabel(metric: RunMetricSample): boolean {
  const labels = metric.raw.labels;
  return Boolean(
    labels.run_id ||
    labels.loop_id ||
    labels.flow_id ||
    labels.component ||
    labels.component_name,
  );
}

function metricValueAsMs(name: string, value: number): number {
  if (name.includes("seconds") || name.endsWith("_seconds") || name.endsWith("_sec")) {
    return value * 1000;
  }
  return value;
}

function formatMetricNumber(value: number): string {
  if (value === 0) return "0";
  if (value < 0.01) return "<0.01";
  return value.toLocaleString("en-US", { maximumFractionDigits: 2 });
}

function valueOf(triples: RawTriple[], predicate: string): string {
  let out = "";
  for (const triple of triples) {
    if (triple.predicate === predicate) out = String(triple.object ?? "");
  }
  return out.trim();
}

function truthy(value: string): boolean {
  return ["true", "yes", "1", "present"].includes(value.trim().toLowerCase());
}

function numberValue(value: string): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function terminalFailure(value: string | undefined): boolean {
  return ["failed", "error", "cancelled", "truncated"].includes(value ?? "");
}

function terminalSuccess(value: string | undefined): boolean {
  return ["success", "complete", "completed"].includes(value ?? "");
}

function loopsComplete(loops: AgentLoop[]): boolean {
  return loops.length > 0 && loops.every((loop) => ["complete", "success", "truncated"].includes(loop.state));
}

function proofBlockerReason(
  graph: GraphRunHealthFacts | null,
): { gate: string; nextAction: string; detail: string } | null {
  if (!graph) return null;
  if (graph.proofPauseRequired || graph.proofReadinessRoute === "pause") {
    return {
      gate: "Readiness Gate",
      nextAction: "Inspect proof findings and ask the operator for a waiver or revised proof model",
      detail: "Readiness Gate failed without a safe route.",
    };
  }
  if (graph.proofTestHarnessRequired || graph.proofReadinessRoute === "test_harness") {
    return {
      gate: "Readiness Gate",
      nextAction: "Build or attach the required harness profile before implementation",
      detail: "Required proof dependencies are not ready.",
    };
  }
  if (graph.proofRequiresClarification || graph.proofReadinessRoute === "coordinator") {
    return {
      gate: "Spec clarification",
      nextAction: "Clarify the proof model before implementation",
      detail: "The proof model is ambiguous.",
    };
  }
  if (graph.formalClaimsStatus === "failed" && !graph.proofImplementationReady) {
    return {
      gate: "Claim Analysis",
      nextAction: "Resolve proof findings before implementation",
      detail: `${graph.formalClaimsFindingCount || "Some"} proof finding(s) block the run.`,
    };
  }
  if (graph.formalClaimsStatus === "ambiguous") {
    return {
      gate: "Claim Analysis",
      nextAction: "Model accepted claims and proof dependencies",
      detail: "The analyzer could not determine what must be proved.",
    };
  }
  if (graph.evidenceFreshness === "stale") {
    return {
      gate: "Evidence freshness",
      nextAction: "Refresh readiness evidence or record a human waiver",
      detail: "One or more readiness or evidence records are stale.",
    };
  }
  return null;
}

function evidenceFreshness(
  triples: RawTriple[],
  now: Date,
  staleAfterMs: number,
): EvidenceFreshness {
  let sawEvidence = false;
  let sawFresh = false;

  for (const triple of triples) {
    if (/^proof\.readiness\.[^.]+\.status$/.test(triple.predicate)) {
      sawEvidence = true;
      const status = String(triple.object ?? "").toLowerCase();
      if (["stale", "failed", "blocked"].includes(status)) return "stale";
      if (["passed", "ready"].includes(status)) sawFresh = true;
    }
    if (/^proof\.readiness\.[^.]+\.smoke_status$/.test(triple.predicate)) {
      sawEvidence = true;
      const status = String(triple.object ?? "").toLowerCase();
      if (status === "failed") return "stale";
      if (status === "passed") sawFresh = true;
    }
    if (/^proof\.readiness\.[^.]+\.expires_at$/.test(triple.predicate)) {
      sawEvidence = true;
      const expires = Date.parse(String(triple.object ?? ""));
      if (Number.isFinite(expires)) {
        if (expires <= now.getTime()) return "stale";
        sawFresh = true;
      }
    }
    if (/^proof\.evidence\.[^.]+\.created_at$/.test(triple.predicate)) {
      sawEvidence = true;
      const created = Date.parse(String(triple.object ?? ""));
      if (Number.isFinite(created)) {
        if (now.getTime() - created > staleAfterMs) return "stale";
        sawFresh = true;
      }
    }
  }

  if (!sawEvidence) return "unknown";
  return sawFresh ? "fresh" : "missing";
}

function countPredicateSuffix(triples: RawTriple[], pattern: RegExp): number {
  const ids = new Set<string>();
  for (const triple of triples) {
    if (!pattern.test(triple.predicate)) continue;
    ids.add(triple.predicate);
  }
  return ids.size;
}

function countStaleEvidence(triples: RawTriple[], now: Date, staleAfterMs: number): number {
  let stale = 0;
  for (const triple of triples) {
    if (/^proof\.readiness\.[^.]+\.status$/.test(triple.predicate)) {
      const status = String(triple.object ?? "").toLowerCase();
      if (["stale", "failed", "blocked"].includes(status)) stale++;
    }
    if (/^proof\.readiness\.[^.]+\.expires_at$/.test(triple.predicate)) {
      const expires = Date.parse(String(triple.object ?? ""));
      if (Number.isFinite(expires) && expires <= now.getTime()) stale++;
    }
    if (/^proof\.evidence\.[^.]+\.created_at$/.test(triple.predicate)) {
      const created = Date.parse(String(triple.object ?? ""));
      if (Number.isFinite(created) && now.getTime() - created > staleAfterMs) stale++;
    }
  }
  return stale;
}
