import type { APIRequestContext, TestInfo } from "@playwright/test";
import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

type JsonRecord = Record<string, unknown>;

interface LoopSummary {
  loop_id?: string;
  role?: string | null;
  state?: string;
  outcome?: string;
  pending_approval?: unknown;
}

interface MessageEntry {
  sequence?: number;
  timestamp?: string;
  subject?: string;
  message_type?: string;
  summary?: string;
  raw_data?: unknown;
}

interface Triple {
  subject?: string;
  predicate?: string;
  object?: unknown;
}

interface ModelEndpoint {
  provider?: string;
  model?: string;
  url?: string;
}

interface ModelCapability {
  preferred?: string[];
  fallback?: string[];
}

interface ModelRegistry {
  endpoints?: Record<string, ModelEndpoint>;
  defaults?: {
    model?: string;
  };
  capabilities?: Record<string, ModelCapability>;
}

interface LoadedModelRegistry {
  configRef: string;
  configPath?: string;
  modelRegistry?: ModelRegistry;
  loadError?: string;
}

interface ResolvedModel {
  requested: string;
  resolution: "endpoint" | "capability" | "model_id" | "unresolved";
  endpoint?: string;
  capability?: string;
  provider?: string;
  model?: string;
  url?: string;
}

interface AttachmentSummary {
  name: string;
  file: string;
  contentType: string;
}

interface ArtifactAttachmentSummary extends AttachmentSummary {
  kind: string;
  bytes?: number;
  metadata?: JsonRecord;
}

export interface JourneyArtifactOutput {
  name: string;
  kind: string;
  contentType: string;
  path?: string | null;
  body?: string;
  metadata?: JsonRecord;
}

export interface JourneyReportOptions {
  journeyName: string;
  request: APIRequestContext;
  testInfo: TestInfo;
  fixture?: string;
  config?: string;
  runIds?: string[];
  runEntityIds?: string[];
  observations?: JsonRecord;
  artifactOutputs?: JourneyArtifactOutput[];
}

export interface JourneyReport {
  version: 1;
  captured_at: string;
  journey: {
    name: string;
    fixture: string;
    config: string;
  };
  run: {
    ids: string[];
    entity_ids: string[];
    phases: Array<{ subject?: string; object?: unknown }>;
  };
  models: {
    registry_config: string;
    registry_path?: string;
    registry_load_error?: string;
    defaults?: ModelRegistry["defaults"];
    requested: string[];
    resolved: ResolvedModel[];
  };
  evidence: {
    loops: {
      count: number;
      states: Record<string, number>;
      roles: Record<string, number>;
    };
    messages: {
      count: number;
      subjects: string[];
    };
    triples: {
      count: number;
      predicates: string[];
    };
    attachments: AttachmentSummary[];
  };
  artifacts: {
    explicit_outputs: ArtifactAttachmentSummary[];
    tool_calls: Array<{
      sequence?: number;
      subject?: string;
      tool: string;
      argument_keys: string[];
      arguments?: unknown;
    }>;
  };
  observations?: JsonRecord;
}

export async function attachJourneyEvidenceReport(
  opts: JourneyReportOptions,
): Promise<JourneyReport> {
  const [loops, messages, triples] = await Promise.all([
    fetchJson<LoopSummary[]>(opts.request, "/teams-dispatch/loops"),
    fetchJson<MessageEntry[]>(
      opts.request,
      "/message-logger/entries?limit=10000",
    ),
    fetchJson<Triple[]>(opts.request, "/graph/triples?limit=10000"),
  ]);

  const loadedRegistry = await loadModelRegistry(opts.config);
  const requestedModels = extractModelRequests(
    messages,
    loadedRegistry.modelRegistry,
  );
  const resolvedModels = requestedModels.map((requested) =>
    resolveModel(requested, loadedRegistry.modelRegistry),
  );

  const evidenceAttachments = [
    await writeJsonAttachment(opts.testInfo, "journey-loops.json", loops),
    await writeJsonAttachment(opts.testInfo, "journey-messages.json", messages),
    await writeJsonAttachment(opts.testInfo, "journey-triples.json", triples),
  ];
  const artifactAttachments = await attachArtifactOutputs(
    opts.testInfo,
    opts.artifactOutputs ?? [],
  );

  const report: JourneyReport = {
    version: 1,
    captured_at: new Date().toISOString(),
    journey: {
      name: opts.journeyName,
      fixture: opts.fixture ?? process.env.FIXTURE ?? "unknown",
      config: loadedRegistry.configRef,
    },
    run: {
      ids: opts.runIds ?? [],
      entity_ids: opts.runEntityIds ?? [],
      phases: triples
        .filter((t) => t.predicate === "agent.run.phase")
        .map((t) => ({ subject: t.subject, object: t.object })),
    },
    models: {
      registry_config: loadedRegistry.configRef,
      registry_path: loadedRegistry.configPath,
      registry_load_error: loadedRegistry.loadError,
      defaults: loadedRegistry.modelRegistry?.defaults,
      requested: requestedModels,
      resolved: resolvedModels,
    },
    evidence: {
      loops: {
        count: loops.length,
        states: countBy(loops, (loop) => loop.state ?? "unknown"),
        roles: countBy(loops, (loop) => loop.role ?? "(dispatch)"),
      },
      messages: {
        count: messages.length,
        subjects: unique(
          messages.map((entry) => entry.subject).filter(isString),
        ).slice(0, 200),
      },
      triples: {
        count: triples.length,
        predicates: unique(
          triples.map((triple) => triple.predicate).filter(isString),
        ).slice(0, 200),
      },
      attachments: evidenceAttachments,
    },
    artifacts: {
      explicit_outputs: artifactAttachments,
      tool_calls: extractArtifactToolCalls(messages),
    },
    observations: opts.observations,
  };

  const reportAttachment = await writeJsonAttachment(
    opts.testInfo,
    "journey-report.json",
    report,
  );
  report.evidence.attachments.push(reportAttachment);
  await writeFile(
    opts.testInfo.outputPath("journey-report.json"),
    `${JSON.stringify(report, null, 2)}\n`,
    "utf8",
  );

  return report;
}

async function fetchJson<T>(
  request: APIRequestContext,
  endpoint: string,
): Promise<T> {
  const resp = await request.get(endpoint);
  if (!resp.ok()) {
    throw new Error(
      `${endpoint} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as T;
}

async function writeJsonAttachment(
  testInfo: TestInfo,
  fileName: string,
  data: unknown,
): Promise<AttachmentSummary> {
  const filePath = testInfo.outputPath(fileName);
  await mkdir(path.dirname(filePath), { recursive: true });
  await writeFile(filePath, `${JSON.stringify(data, null, 2)}\n`, "utf8");
  await testInfo.attach(fileName, {
    path: filePath,
    contentType: "application/json",
  });
  return {
    name: fileName,
    file: path.relative(process.cwd(), filePath),
    contentType: "application/json",
  };
}

async function attachArtifactOutputs(
  testInfo: TestInfo,
  artifacts: JourneyArtifactOutput[],
): Promise<ArtifactAttachmentSummary[]> {
  const attached: ArtifactAttachmentSummary[] = [];
  for (const artifact of artifacts) {
    const fileName = artifactFileName(artifact);
    const filePath = testInfo.outputPath(fileName);
    await mkdir(path.dirname(filePath), { recursive: true });

    if (artifact.path) {
      await copyFile(artifact.path, filePath);
    } else if (artifact.body != null) {
      await writeFile(filePath, artifact.body, "utf8");
    } else {
      continue;
    }

    await testInfo.attach(fileName, {
      path: filePath,
      contentType: artifact.contentType,
    });

    attached.push({
      name: artifact.name,
      kind: artifact.kind,
      file: path.relative(process.cwd(), filePath),
      contentType: artifact.contentType,
      bytes: await byteLength(filePath),
      metadata: artifact.metadata,
    });
  }
  return attached;
}

async function byteLength(filePath: string): Promise<number> {
  return (await readFile(filePath)).byteLength;
}

function artifactFileName(artifact: JourneyArtifactOutput): string {
  const ext = extensionForContentType(artifact.contentType);
  const name = safeFilePart(artifact.name).replace(
    new RegExp(`${escapeRegExp(ext)}$`),
    "",
  );
  return `artifact-${safeFilePart(artifact.kind)}-${name}${ext}`;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function extensionForContentType(contentType: string): string {
  if (contentType.includes("json")) return ".json";
  if (contentType.includes("markdown")) return ".md";
  if (contentType.startsWith("text/")) return ".txt";
  return ".bin";
}

function safeFilePart(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 120);
}

async function loadModelRegistry(
  configOverride?: string,
): Promise<LoadedModelRegistry> {
  const configRef =
    configOverride ?? process.env.AGENTIC_CONFIG ?? "e2e-flow-bootstrap.json";
  const candidates = configCandidates(configRef);
  for (const candidate of candidates) {
    try {
      const raw = await readFile(candidate, "utf8");
      const parsed = JSON.parse(raw) as { model_registry?: ModelRegistry };
      return {
        configRef,
        configPath: path.relative(process.cwd(), candidate),
        modelRegistry: parsed.model_registry,
      };
    } catch (err) {
      if (candidate === candidates[candidates.length - 1]) {
        return {
          configRef,
          loadError: err instanceof Error ? err.message : String(err),
        };
      }
    }
  }
  return { configRef, loadError: "no config candidate paths were generated" };
}

function configCandidates(configRef: string): string[] {
  if (path.isAbsolute(configRef)) return [configRef];

  const fromHelper = fileURLToPath(new URL("../../../configs/", import.meta.url));
  return unique([
    path.resolve(process.cwd(), "../configs", configRef),
    path.resolve(process.cwd(), "configs", configRef),
    path.resolve(fromHelper, configRef),
  ]);
}

function extractModelRequests(
  messages: MessageEntry[],
  registry?: ModelRegistry,
): string[] {
  const models = new Set<string>();
  for (const entry of messages) collectModelValues(entry.raw_data, models);
  if (models.size === 0 && registry?.defaults?.model) {
    models.add(registry.defaults.model);
  }
  return [...models].sort();
}

function collectModelValues(value: unknown, out: Set<string>): void {
  if (Array.isArray(value)) {
    for (const item of value) collectModelValues(item, out);
    return;
  }
  const record = asRecord(value);
  if (!record) return;
  for (const [key, child] of Object.entries(record)) {
    if (isModelKey(key) && isString(child) && child.trim() !== "") {
      out.add(child.trim());
    }
    collectModelValues(child, out);
  }
}

function isModelKey(key: string): boolean {
  return ["model", "model_id", "model_name", "model_alias"].includes(key);
}

function resolveModel(
  requested: string,
  registry?: ModelRegistry,
): ResolvedModel {
  const endpoints = registry?.endpoints ?? {};
  const direct = endpoints[requested];
  if (direct) {
    return {
      requested,
      resolution: "endpoint",
      endpoint: requested,
      provider: direct.provider,
      model: direct.model,
      url: direct.url,
    };
  }

  const capability = registry?.capabilities?.[requested];
  const endpointName = capability?.preferred?.[0] ?? capability?.fallback?.[0];
  if (endpointName) {
    const endpoint = endpoints[endpointName];
    return {
      requested,
      resolution: "capability",
      capability: requested,
      endpoint: endpointName,
      provider: endpoint?.provider,
      model: endpoint?.model,
      url: endpoint?.url,
    };
  }

  const modelMatch = Object.entries(endpoints).find(
    ([, endpoint]) => endpoint.model === requested,
  );
  if (modelMatch) {
    const [endpointNameForModel, endpoint] = modelMatch;
    return {
      requested,
      resolution: "model_id",
      endpoint: endpointNameForModel,
      provider: endpoint.provider,
      model: endpoint.model,
      url: endpoint.url,
    };
  }

  return { requested, resolution: "unresolved" };
}

function extractArtifactToolCalls(
  messages: MessageEntry[],
): JourneyReport["artifacts"]["tool_calls"] {
  return messages.flatMap((entry) => {
    const tool = toolName(entry);
    if (!tool || !isArtifactTool(tool)) return [];
    const args = toolArguments(entry);
    const argRecord = asRecord(args);
    return [
      {
        sequence: entry.sequence,
        subject: entry.subject,
        tool,
        argument_keys: argRecord ? Object.keys(argRecord).sort() : [],
        arguments: args,
      },
    ];
  });
}

function toolName(entry: MessageEntry): string | undefined {
  const payload = asRecord(asRecord(entry.raw_data)?.payload);
  const fromPayload = payload?.name;
  if (isString(fromPayload)) return fromPayload;
  if (entry.subject?.startsWith("tool.execute.")) {
    return entry.subject.slice("tool.execute.".length);
  }
  return undefined;
}

function toolArguments(entry: MessageEntry): unknown {
  const payload = asRecord(asRecord(entry.raw_data)?.payload);
  return payload?.arguments ?? payload?.tool_arguments;
}

function isArtifactTool(tool: string): boolean {
  return (
    tool.startsWith("emit_") ||
    tool === "render_openspec" ||
    tool === "project_spec_tasks"
  );
}

function countBy<T>(
  items: T[],
  keyFn: (item: T) => string,
): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const item of items) {
    const key = keyFn(item);
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}

function unique<T>(items: T[]): T[] {
  return [...new Set(items)];
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function asRecord(value: unknown): JsonRecord | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return undefined;
  }
  return value as JsonRecord;
}
