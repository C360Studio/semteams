// Live GitHub work source (OpenSpec work-live-github-source, design D1/D2; issue #319).
//
// Reads repository issues, pull requests, and PR review comments server-side via
// the GitHub REST API (GET only, bounded by READ_TIMEOUT_MS) and derives
// structured baton, review, and decision/hold gate overlays through
// deriveBatonOverlays() without altering GitHub-owned columns.

import type {
  LookupStatus,
  Overlay,
  PullRequestRef,
  PullRequestReviewRecord,
  PullRequestReviewRequest,
  Repository,
  WorkDependency,
  WorkItem,
  WorkItemState,
} from "$lib/types/work";
import { deriveBatonOverlays, shaMatchesHead } from "./baton";
import { columnFor, columnsFor } from "./columns";
import { DEFAULT_GITHUB_API_URL } from "./config";
import type { ItemRef, WorkSource } from "./source";

export const READ_TIMEOUT_MS = 5000;
export const GITHUB_PAGE_LIMIT = 100;
export const MAX_OPEN_PR_COMMENT_FETCHES = 10;

export interface GithubSourceOptions {
  token?: string;
  apiUrl?: string;
}

type Read<T> =
  | { ok: true; value: T }
  | { ok: false; status?: number; error: string };

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

const EXCERPT_LENGTH = 240;

function excerpt(body: string | undefined): string {
  const flat = (body ?? "").replace(/\s+/g, " ").trim();
  return flat.length > EXCERPT_LENGTH ? `${flat.slice(0, EXCERPT_LENGTH).trimEnd()}…` : flat;
}

async function getGithubJson(
  apiBase: string,
  path: string,
  label: string,
  token?: string,
): Promise<Read<unknown>> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
  try {
    const headers: Record<string, string> = {
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
    };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    const response = await fetch(`${apiBase}${path}`, {
      signal: controller.signal,
      headers,
    });
    if (!response.ok) {
      return {
        ok: false,
        status: response.status,
        error: `${label} answered ${response.status}`,
      };
    }
    return { ok: true, value: await response.json() };
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") {
      return { ok: false, error: `${label} timed out after ${READ_TIMEOUT_MS} ms` };
    }
    return {
      ok: false,
      error: `${label} failed: ${err instanceof Error ? err.message : String(err)}`,
    };
  } finally {
    clearTimeout(timer);
  }
}

function graphqlEndpoint(apiBase: string): string {
  return apiBase.endsWith("/api/v3")
    ? `${apiBase.slice(0, -"/api/v3".length)}/api/graphql`
    : `${apiBase}/graphql`;
}

const PROJECT_BOARD_QUERY = `
query SemTeamsProjectBoard($owner: String!, $number: Int!, $itemLimit: Int!) {
  organization(login: $owner) {
    projectV2(number: $number) {
      ...ProjectFields
    }
  }
  user(login: $owner) {
    projectV2(number: $number) {
      ...ProjectFields
    }
  }
}
fragment ProjectFields on ProjectV2 {
  fields(first: 50) {
    nodes {
      __typename
      ... on ProjectV2SingleSelectField {
        name
        options {
          name
        }
      }
    }
  }
  items(first: $itemLimit) {
    pageInfo {
      hasNextPage
    }
    nodes {
      content {
        __typename
        ... on Issue {
          number
          repository {
            nameWithOwner
          }
        }
      }
      fieldValues(first: 20) {
        nodes {
          __typename
          ... on ProjectV2ItemFieldSingleSelectValue {
            name
            field {
              ... on ProjectV2FieldCommon {
                name
              }
            }
          }
          ... on ProjectV2ItemFieldTextValue {
            text
            field {
              ... on ProjectV2FieldCommon {
                name
              }
            }
          }
        }
      }
    }
  }
}
`.trim();

async function postGithubGraphql(
  apiBase: string,
  query: string,
  variables: Record<string, unknown>,
  label: string,
  token?: string,
): Promise<Read<unknown>> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
  try {
    const headers: Record<string, string> = {
      Accept: "application/vnd.github+json",
      "Content-Type": "application/json",
      "X-GitHub-Api-Version": "2022-11-28",
    };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    const response = await fetch(graphqlEndpoint(apiBase), {
      method: "POST",
      signal: controller.signal,
      headers,
      body: JSON.stringify({ query, variables }),
    });
    if (!response.ok) {
      return {
        ok: false,
        status: response.status,
        error: `${label} answered ${response.status}`,
      };
    }
    return { ok: true, value: await response.json() };
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") {
      return { ok: false, error: `${label} timed out after ${READ_TIMEOUT_MS} ms` };
    }
    return {
      ok: false,
      error: `${label} failed: ${err instanceof Error ? err.message : String(err)}`,
    };
  } finally {
    clearTimeout(timer);
  }
}

interface ProjectItemFields {
  pm_status?: string;
  priority?: string;
}

type ProjectBoardResolution =
  | { configured: false }
  | { configured: true; ok: false; error: string }
  | {
      configured: true;
      ok: true;
      statusOptions: string[];
      itemsByIssueNumber: Map<number, ProjectItemFields>;
      truncationReason?: string;
    };

async function readProjectBoard(
  apiBase: string,
  repository: Repository,
  token?: string,
): Promise<ProjectBoardResolution> {
  const board = repository.project_board;
  if (!board) return { configured: false };

  const label = `POST /graphql (ProjectV2 ${board.owner}#${board.number})`;
  const read = await postGithubGraphql(
    apiBase,
    PROJECT_BOARD_QUERY,
    {
      owner: board.owner,
      number: board.number,
      itemLimit: GITHUB_PAGE_LIMIT,
    },
    label,
    token,
  );

  if (!read.ok) {
    return { configured: true, ok: false, error: read.error };
  }
  if (!isObject(read.value)) {
    return { configured: true, ok: false, error: `${label} returned a non-object body` };
  }

  const data = isObject(read.value.data) ? read.value.data : undefined;
  const orgProject =
    data && isObject(data.organization) && isObject(data.organization.projectV2)
      ? data.organization.projectV2
      : undefined;
  const userProject =
    data && isObject(data.user) && isObject(data.user.projectV2)
      ? data.user.projectV2
      : undefined;
  const projectNode = orgProject ?? userProject;

  if (!projectNode) {
    const rawErrors = Array.isArray(read.value.errors)
      ? read.value.errors.filter(isObject)
      : [];
    const nonNotFound = rawErrors.find(
      (e) => e.type !== "NOT_FOUND" && typeof e.message === "string" && e.message.trim(),
    );
    if (nonNotFound && typeof nonNotFound.message === "string") {
      return {
        configured: true,
        ok: false,
        error: `${label}: ${nonNotFound.message.trim()}`,
      };
    }
    return {
      configured: true,
      ok: false,
      error: `${label}: project ${board.owner}#${board.number} not found`,
    };
  }

  const statusFieldLower = board.status_field.trim().toLowerCase();
  const priorityFieldLower = board.priority_field?.trim().toLowerCase();

  const fieldNodes =
    isObject(projectNode.fields) && Array.isArray(projectNode.fields.nodes)
      ? projectNode.fields.nodes.filter(isObject)
      : [];
  const statusFieldNode = fieldNodes.find(
    (f) =>
      typeof f.name === "string" &&
      f.name.trim().toLowerCase() === statusFieldLower &&
      Array.isArray(f.options),
  );
  const statusOptions =
    statusFieldNode && Array.isArray(statusFieldNode.options)
      ? statusFieldNode.options
          .filter(isObject)
          .map((opt) => (typeof opt.name === "string" ? opt.name.trim() : ""))
          .filter(Boolean)
      : [];

  if (statusOptions.length === 0) {
    return {
      configured: true,
      ok: false,
      error: `${label}: single-select status field "${board.status_field}" not found`,
    };
  }

  if (!isObject(projectNode.items) || !Array.isArray(projectNode.items.nodes)) {
    return {
      configured: true,
      ok: false,
      error: `${label}: project items missing`,
    };
  }

  const itemNodes = projectNode.items.nodes;
  const hasNextPage =
    isObject(projectNode.items.pageInfo) && projectNode.items.pageInfo.hasNextPage === true;
  const truncated = hasNextPage || itemNodes.length >= GITHUB_PAGE_LIMIT;

  const targetRepo = `${repository.owner}/${repository.name}`.toLowerCase();
  const itemsByIssueNumber = new Map<number, ProjectItemFields>();

  for (const rawItem of itemNodes) {
    if (!isObject(rawItem) || !isObject(rawItem.content)) continue;
    const content = rawItem.content;
    if (typeof content.__typename === "string" && content.__typename !== "Issue") continue;
    if (typeof content.number !== "number" || !Number.isInteger(content.number) || content.number < 1) {
      continue;
    }
    if (
      !isObject(content.repository) ||
      typeof content.repository.nameWithOwner !== "string" ||
      content.repository.nameWithOwner.trim().toLowerCase() !== targetRepo
    ) {
      continue;
    }

    let pm_status: string | undefined;
    let priority: string | undefined;

    const fvNodes =
      isObject(rawItem.fieldValues) && Array.isArray(rawItem.fieldValues.nodes)
        ? rawItem.fieldValues.nodes.filter(isObject)
        : [];
    for (const fv of fvNodes) {
      if (!isObject(fv.field) || typeof fv.field.name !== "string") continue;
      const fieldNameLower = fv.field.name.trim().toLowerCase();
      const val =
        typeof fv.name === "string"
          ? fv.name.trim()
          : typeof fv.text === "string"
            ? fv.text.trim()
            : "";
      if (!val) continue;
      if (fieldNameLower === statusFieldLower) {
        pm_status = val;
      } else if (priorityFieldLower && fieldNameLower === priorityFieldLower) {
        priority = val;
      }
    }

    itemsByIssueNumber.set(content.number, {
      ...(pm_status && { pm_status }),
      ...(priority && { priority }),
    });
  }

  return {
    configured: true,
    ok: true,
    statusOptions,
    itemsByIssueNumber,
    ...(truncated && {
      truncationReason: `${label} items truncated at ${GITHUB_PAGE_LIMIT} items`,
    }),
  };
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

interface ParsedIssue {
  number: number;
  title: string;
  body: string;
  state: WorkItemState;
  labels: string[];
  assignees: string[];
  milestone?: string;
  dependencies: WorkDependency[];
}

interface ParsedPullRequest {
  number: number;
  ref: PullRequestRef;
  claimedIssueNumbers: number[];
}

function normalizeIssueRef(rawRef: string, defaultRepo: string): string {
  const trimmed = rawRef.trim();
  return trimmed.startsWith("#") ? `${defaultRepo}${trimmed}` : trimmed;
}

export function extractIssueDependencies(body: string, defaultRepo: string): WorkDependency[] {
  const seen = new Set<string>();
  const out: WorkDependency[] = [];
  const add = (rawRef: string, source: WorkDependency["source"]) => {
    const ref = normalizeIssueRef(rawRef, defaultRepo);
    const key = `${source}:${ref.toLowerCase()}`;
    if (seen.has(key)) return;
    seen.add(key);
    out.push({ ref, source });
  };

  const blockedRegex =
    /(?:blocked[\s-]+by|depends[\s-]+on)\s*:?\s*((?:[a-z0-9_.-]+\/[a-z0-9_.-]+)?#\d+(?:\s*,\s*(?:[a-z0-9_.-]+\/[a-z0-9_.-]+)?#\d+)*)/gi;
  for (const match of body.matchAll(blockedRegex)) {
    const list = match[1] ?? "";
    for (const refMatch of list.matchAll(/((?:[a-z0-9_.-]+\/[a-z0-9_.-]+)?#\d+)/gi)) {
      if (refMatch[1]) add(refMatch[1], "blocked-by");
    }
  }

  const taskLineRegex = /^[ \t]*[-*][ \t]+\[[ xX]\][ \t]+([^\r\n]+)/gm;
  for (const lineMatch of body.matchAll(taskLineRegex)) {
    const line = lineMatch[1] ?? "";
    for (const refMatch of line.matchAll(/(?:^|[\s(])((?:[a-z0-9_.-]+\/[a-z0-9_.-]+)?#\d+)\b/gi)) {
      if (refMatch[1]) add(refMatch[1], "task-list");
    }
  }

  return out;
}

function parseIssue(raw: Record<string, unknown>, repository: Repository): ParsedIssue | null {
  // GitHub's /issues endpoint includes pull requests carrying a `pull_request` key.
  if (isObject(raw.pull_request)) return null;
  if (typeof raw.number !== "number" || !Number.isInteger(raw.number) || raw.number < 1) {
    return null;
  }
  if (typeof raw.title !== "string" || !raw.title.trim()) {
    return null;
  }
  const body = typeof raw.body === "string" ? raw.body : "";
  const state: WorkItemState = raw.state === "closed" ? "closed" : "open";

  const labels: string[] = [];
  if (Array.isArray(raw.labels)) {
    for (const entry of raw.labels) {
      if (typeof entry === "string" && entry.trim()) labels.push(entry.trim());
      else if (isObject(entry) && typeof entry.name === "string" && entry.name.trim()) {
        labels.push(entry.name.trim());
      }
    }
  }

  const assignees: string[] = [];
  if (Array.isArray(raw.assignees)) {
    for (const entry of raw.assignees) {
      if (typeof entry === "string" && entry.trim()) assignees.push(entry.trim());
      else if (isObject(entry) && typeof entry.login === "string" && entry.login.trim()) {
        assignees.push(entry.login.trim());
      }
    }
  } else if (isObject(raw.assignee) && typeof raw.assignee.login === "string" && raw.assignee.login.trim()) {
    assignees.push(raw.assignee.login.trim());
  }

  const milestone =
    isObject(raw.milestone) && typeof raw.milestone.title === "string" && raw.milestone.title.trim()
      ? raw.milestone.title.trim()
      : undefined;

  const repoFull = `${repository.owner}/${repository.name}`;
  return {
    number: raw.number,
    title: raw.title,
    body,
    state,
    labels,
    assignees,
    ...(milestone && { milestone }),
    dependencies: extractIssueDependencies(body, repoFull),
  };
}

export function extractClaimedIssueNumbers(
  title: string,
  body: string,
  repository: Repository,
): number[] {
  const targetRepo = `${repository.owner}/${repository.name}`.toLowerCase();
  const numbers = new Set<number>();

  const closingRegex =
    /\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?|claims?)\s*:?\s*(?:([a-z0-9_.-]+\/[a-z0-9_.-]+)?#(\d+))/gi;
  for (const match of body.matchAll(closingRegex)) {
    const repoPrefix = match[1]?.toLowerCase();
    const num = Number(match[2]);
    if ((!repoPrefix || repoPrefix === targetRepo) && Number.isInteger(num) && num > 0) {
      numbers.add(num);
    }
  }

  for (const match of title.matchAll(/(?:^|[\s(])#(\d+)\b/g)) {
    const num = Number(match[1]);
    if (Number.isInteger(num) && num > 0) {
      numbers.add(num);
    }
  }

  return [...numbers];
}

function matchMetadataField(text: string, fieldPattern: string): string | undefined {
  const regex = new RegExp(
    `^[ \\t]*(?:[-*][ \\t]*)?[*\`]*${fieldPattern}[*\`]*[ \\t]*:[*\`]*[ \\t]*[*\`]*([^\\r\\n\`*]+)[*\`]*`,
    "im",
  );
  const match = text.match(regex);
  const val = match?.[1]?.trim();
  return val ? val : undefined;
}

function parsePullRequest(raw: Record<string, unknown>, repository: Repository): ParsedPullRequest | null {
  if (typeof raw.number !== "number" || !Number.isInteger(raw.number) || raw.number < 1) {
    return null;
  }
  const title = typeof raw.title === "string" ? raw.title : "";
  const body = typeof raw.body === "string" ? raw.body : "";
  const claimedIssueNumbers = extractClaimedIssueNumbers(title, body, repository);

  const merged = Boolean(raw.merged_at);
  const state: PullRequestRef["state"] = merged
    ? "merged"
    : raw.state === "closed"
      ? "closed"
      : "open";
  const draft = typeof raw.draft === "boolean" ? raw.draft : undefined;
  const branch =
    isObject(raw.head) && typeof raw.head.ref === "string" && raw.head.ref.trim()
      ? raw.head.ref.trim()
      : undefined;
  const headSha =
    isObject(raw.head) && typeof raw.head.sha === "string" && raw.head.sha.trim()
      ? raw.head.sha.trim()
      : undefined;

  const implementedBy = matchMetadataField(body, "implemented[- \\t_]*by");
  const reviewedBy = matchMetadataField(body, "reviewed[- \\t_]*by");
  const stopPoint = matchMetadataField(body, "stop[- \\t_]*point");

  return {
    number: raw.number,
    claimedIssueNumbers,
    ref: {
      ref: `${repository.owner}/${repository.name}#${raw.number}`,
      ...(title && { title }),
      state,
      ...(draft !== undefined && { draft }),
      ...(branch && { branch }),
      ...(headSha && { head_sha: headSha }),
      ...(implementedBy && { implemented_by: implementedBy }),
      ...(reviewedBy && { reviewed_by: reviewedBy }),
      ...(stopPoint && { stop_point: stopPoint }),
    },
  };
}

function stripFencedCodeBlocks(text: string): string {
  const lines = text.split(/\r?\n/);
  const out: string[] = [];
  let fenceChar: "`" | "~" | null = null;
  let fenceLen = 0;

  for (const line of lines) {
    if (fenceChar === null) {
      const open = line.match(/^[ \t]{0,3}(`{3,}|~{3,})(.*)$/);
      if (open) {
        const marker = open[1];
        const rest = open[2];
        const ch = marker[0] as "`" | "~";
        if (ch === "~" || !rest.includes("`")) {
          fenceChar = ch;
          fenceLen = marker.length;
          continue;
        }
      }
      out.push(line);
    } else {
      const close = line.match(/^[ \t]{0,3}(`{3,}|~{3,})[ \t]*$/);
      if (close && close[1][0] === fenceChar && close[1].length >= fenceLen) {
        fenceChar = null;
        fenceLen = 0;
      }
    }
  }

  return out.join("\n");
}

function stripBlockquotes(text: string): string {
  return text
    .split(/\r?\n/)
    .filter((line) => !/^[ \t]*>/.test(line))
    .join("\n");
}

function stripKeywordInlineCode(text: string): string {
  return text.replace(/`+([^`\r\n]+)`+/g, (full, inner: string) =>
    /\b(?:reviewed[- \t_]*by|(?:re-)?reviewed[ \t]+at|verdict)\b/i.test(inner) ? "" : full,
  );
}

function resolveSingleCommitSha(candidates: string[]): string | undefined {
  if (candidates.length === 0) return undefined;
  const normalized = candidates.map((sha) => sha.toLowerCase());
  let longest = normalized[0];
  for (const sha of normalized) {
    if (sha.length > longest.length) {
      longest = sha;
    }
  }
  for (const sha of normalized) {
    if (!longest.startsWith(sha)) {
      return undefined;
    }
  }
  return longest;
}

const ATEAM_AGENT_TOKEN_RE = /(?<![a-z0-9])(claude|codex|gemini)(?![a-z0-9])/gi;
const LEADING_AGENT_RE = /^(claude|codex|gemini)(?![a-z0-9])(.*)$/i;
const MULTI_AGENT_CONTINUATION_RE =
  /^(?:\||\/|;|,|\b(?:and|or)\b)[ \t]*[`*]*(?:claude|codex|gemini)(?![a-z0-9])/i;
const REVIEWED_BY_LINE_RE =
  /^[ \t]*(?:[-*][ \t]*)?(?:\*{1,2}reviewed[- \t_]*by\*{0,2}|reviewed[- \t_]*by)[ \t]*:(?:\*{1,2})?[ \t]*([^\r\n]*)$/gim;
const CROSS_AGENT_HEADING_RE =
  /^[ \t]*#{1,6}[ \t]+independent[ \t]+cross-agent[ \t]+review\b([^\r\n]*)$/gim;
const VERDICT_LINE_RE =
  /^[ \t]*(?:(?:#{1,6}|[-*])[ \t]+)?(?:\*{1,2}verdict\*{0,2}|verdict)[ \t]*:(?:\*{1,2})?[ \t]*([^\r\n]*)$/gim;
const VERDICT_CLAUSE_SPLIT_RE =
  /[ \t]*(?:\(|—|–|,|:|\.|!|[ \t]-[ \t]|[ \t]+(?:with|see|on|at|citing|for|pending|after|until|once|because)\b)/i;
const REVIEWER_LANE_LINE_RE =
  /^[ \t]*(?:[-*][ \t]*)?(?:\*{1,2}(?:reviewer[- \t_]*lanes?|review[- \t_]*lanes?|lanes?)\*{0,2}|`(?:reviewer[- \t_]*lanes?|review[- \t_]*lanes?|lanes?)`|(?:reviewer[- \t_]*lanes?|review[- \t_]*lanes?|lanes?))[ \t]*:(?:\*{1,2})?[ \t]*([^\r\n]+)/im;
const REVIEW_REQUEST_HEADING_RE = /^[ \t]*#{1,6}[ \t]+review[ \t]+request\b/im;
const REVIEW_REQUEST_PLAIN_FIRST_LINE_RE =
  /^[ \t]*(?:\*{1,2})?review[ \t]+request(?:\*{1,2})?(?:[ \t]*[:—–-][^\r\n]*)?[ \t]*$/i;
const REVIEW_RECORD_HEADING_RE = /^[ \t]*#{1,6}[ \t]+review[ \t]+record\b/im;
const REVIEWED_AT_LINE_RE =
  /^[ \t]*(?:(?:#{1,6}|[-*])[ \t]+)?(?:\*{1,2}(?:re-)?reviewed[ \t]+at\b\*{0,2}|(?:re-)?reviewed[ \t]+at\b)[ \t]*:?[*` \t]*\b([0-9a-f]{7,40})\b(?!\.\.)/im;
const RECORD_SHA_RE =
  /\b(?:re-)?reviewed[ \t]+at\b[*` \t]*:?[*` \t]*\b([0-9a-f]{7,40})\b(?!\.\.)/gi;
const REQUEST_SHA_LINE_RE =
  /^[ \t]*(?:[-*][ \t]*)?(?:\*{1,2}(?:commit(?:[ \t]+sha)?(?:[ \t]+to[ \t]+read)?|head(?:[ \t_]+(?:sha|commit))?|sha)\*{0,2}|`(?:commit(?:[ \t]+sha)?(?:[ \t]+to[ \t]+read)?|head(?:[ \t_]+(?:sha|commit))?|sha)`|(?:commit(?:[ \t]+sha)?(?:[ \t]+to[ \t]+read)?|head(?:[ \t_]+(?:sha|commit))?|sha))[ \t]*:(?:\*{1,2})?[ \t]*[*`]*\b([0-9a-f]{7,40})\b(?!\.\.)/gim;

function extractAgentVendors(text: string): Set<string> {
  const agents = new Set<string>();
  for (const match of text.matchAll(ATEAM_AGENT_TOKEN_RE)) {
    agents.add(match[1].toLowerCase());
  }
  return agents;
}

function extractAteamReviewerIdentity(
  unquoted: string,
): { matched: boolean; reviewed_by?: string } {
  const reviewedByMatches = [...unquoted.matchAll(REVIEWED_BY_LINE_RE)];
  if (reviewedByMatches.length > 0) {
    const allAgents = new Set<string>();
    let display: string | undefined;
    for (const match of reviewedByMatches) {
      const cleaned = match[1].replace(/[`*]/g, "").trim();
      const leadMatch = cleaned.match(LEADING_AGENT_RE);
      if (!leadMatch) {
        return { matched: false };
      }
      const leadAgent = leadMatch[1].toLowerCase();
      const rest = leadMatch[2].trim();
      if (rest.startsWith("(")) {
        const parenMatch = rest.match(/^\(([^()]+)\)(.*)$/);
        if (!parenMatch) {
          return { matched: false };
        }
        for (const v of extractAgentVendors(parenMatch[1])) {
          if (v !== leadAgent) {
            return { matched: false };
          }
        }
        const afterParen = parenMatch[2].trim();
        if (afterParen && MULTI_AGENT_CONTINUATION_RE.test(afterParen)) {
          return { matched: false };
        }
      } else {
        const lineAgents = extractAgentVendors(cleaned);
        if (lineAgents.size !== 1 || /^[|/]/.test(rest)) {
          return { matched: false };
        }
      }
      allAgents.add(leadAgent);
      display = cleaned;
    }
    if (allAgents.size !== 1 || !display) {
      return { matched: false };
    }
    return { matched: true, reviewed_by: display };
  }

  const headingMatches = [...unquoted.matchAll(CROSS_AGENT_HEADING_RE)];
  if (headingMatches.length === 0) {
    return { matched: false };
  }

  const headingAgents = new Set<string>();
  let headingDisplay: string | undefined;
  for (const match of headingMatches) {
    const suffix = match[1] ?? "";
    const agents = extractAgentVendors(suffix);
    if (agents.size !== 1) {
      return { matched: false };
    }
    for (const agent of agents) {
      headingAgents.add(agent);
    }
    let cleaned = suffix
      .replace(/^[ \t:—–-]+/, "")
      .replace(/[`*]/g, "")
      .trim();
    const unwrapped = cleaned.match(/^\(([^()]+)\)$/);
    if (unwrapped) {
      cleaned = unwrapped[1].trim();
    }
    if (cleaned) headingDisplay = cleaned;
  }

  if (headingAgents.size !== 1 || !headingDisplay) {
    return { matched: false };
  }

  return {
    matched: true,
    reviewed_by: headingDisplay,
  };
}

function extractDesignatedRecordSha(unquoted: string): string | undefined {
  const candidates: string[] = [];
  for (const match of unquoted.matchAll(RECORD_SHA_RE)) {
    candidates.push(match[1]);
  }
  return resolveSingleCommitSha(candidates);
}

function extractAteamVerdict(unquoted: string): PullRequestReviewRecord["verdict"] | undefined {
  const matches = [...unquoted.matchAll(VERDICT_LINE_RE)];
  if (matches.length === 0) return undefined;

  const verdicts = new Set<PullRequestReviewRecord["verdict"]>();
  for (const match of matches) {
    const rawVal = match[1].replace(/[`*]/g, "").trim();
    const lead = rawVal.split(VERDICT_CLAUSE_SPLIT_RE)[0].trim().toLowerCase();

    let parsed: PullRequestReviewRecord["verdict"] | undefined;
    if (lead === "approve" || lead === "approved" || lead === "pass") {
      parsed = "pass";
    } else if (
      lead === "request changes" ||
      lead === "request_changes" ||
      lead === "request-changes" ||
      lead === "changes requested" ||
      lead === "changes_requested" ||
      lead === "changes-requested" ||
      lead === "blocking" ||
      lead === "fail"
    ) {
      parsed = "blocking";
    } else {
      return undefined;
    }

    verdicts.add(parsed);
  }

  if (verdicts.size !== 1) return undefined;
  return [...verdicts][0];
}

function extractReviewerLane(unquoted: string): string | undefined {
  const match = unquoted.match(REVIEWER_LANE_LINE_RE);
  const val = match?.[1]?.replace(/[`*]/g, "").trim();
  return val ? val : undefined;
}

function parseAteamReviewRecord(unquoted: string): PullRequestReviewRecord | undefined {
  const reviewer = extractAteamReviewerIdentity(unquoted);
  if (!reviewer.matched) return undefined;

  const sha = extractDesignatedRecordSha(unquoted);
  if (!sha) return undefined;

  const verdict = extractAteamVerdict(unquoted);
  if (!verdict) return undefined;

  const lane = extractReviewerLane(unquoted);
  return {
    sha,
    verdict,
    ...(lane && { reviewer_lane: lane }),
    ...(reviewer.reviewed_by && { reviewed_by: reviewer.reviewed_by }),
  };
}

function hasRecordSignals(unquoted: string): boolean {
  return (
    REVIEW_RECORD_HEADING_RE.test(unquoted) ||
    REVIEWED_AT_LINE_RE.test(unquoted) ||
    [...unquoted.matchAll(REVIEWED_BY_LINE_RE)].length > 0 ||
    [...unquoted.matchAll(CROSS_AGENT_HEADING_RE)].length > 0 ||
    [...unquoted.matchAll(VERDICT_LINE_RE)].length > 0
  );
}

function hasReviewRequestHeader(unquoted: string): boolean {
  if (REVIEW_REQUEST_HEADING_RE.test(unquoted)) return true;
  const firstNonEmpty = unquoted.split(/\r?\n/).find((line) => line.trim().length > 0);
  if (!firstNonEmpty) return false;
  return REVIEW_REQUEST_PLAIN_FIRST_LINE_RE.test(firstNonEmpty);
}

function parsePlainReviewRequest(unquoted: string): PullRequestReviewRequest | undefined {
  if (!hasReviewRequestHeader(unquoted) || hasRecordSignals(unquoted)) {
    return undefined;
  }

  const shas: string[] = [];
  for (const match of unquoted.matchAll(REQUEST_SHA_LINE_RE)) {
    shas.push(match[1]);
  }
  const sha = resolveSingleCommitSha(shas);
  if (!sha) return undefined;

  const lane = extractReviewerLane(unquoted);
  return {
    sha,
    ...(lane && { reviewer_lane: lane }),
  };
}

function parseLegacyReviewRequest(body: string): PullRequestReviewRequest | undefined {
  const sha = matchMetadataField(body, "(?:commit|head(?:_sha)?|sha)");
  const lane = matchMetadataField(body, "(?:reviewer[- \\t_]*lane|lane)");
  if (sha && /^[0-9a-f]{7,40}$/i.test(sha)) {
    return {
      sha,
      ...(lane && { reviewer_lane: lane }),
    };
  }
  return undefined;
}

function parseLegacyReviewRecord(body: string): PullRequestReviewRecord | undefined {
  const sha = matchMetadataField(body, "(?:commit|head(?:_sha)?|sha)");
  const verdictRaw = matchMetadataField(body, "verdict")?.toLowerCase();
  const lane = matchMetadataField(body, "(?:reviewer[- \\t_]*lane|lane)");
  const reviewedBy = matchMetadataField(body, "reviewed[- \\t_]*by");
  const verdict: PullRequestReviewRecord["verdict"] | undefined =
    verdictRaw === "pass" || verdictRaw === "approved"
      ? "pass"
      : verdictRaw === "blocking" || verdictRaw === "changes_requested" || verdictRaw === "fail"
        ? "blocking"
        : undefined;
  if (sha && /^[0-9a-f]{7,40}$/i.test(sha) && verdict) {
    return {
      sha,
      verdict,
      ...(lane && { reviewer_lane: lane }),
      ...(reviewedBy && { reviewed_by: reviewedBy }),
    };
  }
  return undefined;
}

export function parseReviewComments(comments: unknown[]): {
  review_request?: PullRequestReviewRequest;
  review_record?: PullRequestReviewRecord;
  review_unparsed?: boolean;
} {
  let review_request: PullRequestReviewRequest | undefined;
  let review_record: PullRequestReviewRecord | undefined;
  let review_unparsed = false;

  for (const entry of comments) {
    if (!isObject(entry) || typeof entry.body !== "string") continue;
    const body = entry.body;
    const unquoted = stripBlockquotes(stripFencedCodeBlocks(body));
    const unquotedWithoutInlineCode = unquoted.replace(/`[^`\r\n]*`/g, "");

    const hasLegacyRequestMarker = unquotedWithoutInlineCode.includes(
      "<!-- semteams-review-request -->",
    );
    const hasLegacyRecordMarker = unquotedWithoutInlineCode.includes(
      "<!-- semteams-review-record -->",
    );

    let matchedLegacy = false;
    if (hasLegacyRequestMarker) {
      const legacyRequest = parseLegacyReviewRequest(unquoted);
      if (legacyRequest) {
        review_request = legacyRequest;
        matchedLegacy = true;
      }
    }
    if (hasLegacyRecordMarker) {
      const legacyRecord = parseLegacyReviewRecord(unquoted);
      if (legacyRecord) {
        review_record = legacyRecord;
      } else {
        review_unparsed = true;
      }
      matchedLegacy = true;
    }
    if (matchedLegacy) continue;

    const ateamBody = stripKeywordInlineCode(unquoted);
    const ateamRecord = parseAteamReviewRecord(ateamBody);
    if (ateamRecord) {
      review_record = ateamRecord;
      continue;
    }
    if (hasRecordSignals(ateamBody)) {
      review_unparsed = true;
      continue;
    }

    const plainRequest = parsePlainReviewRequest(ateamBody);
    if (plainRequest) {
      review_request = plainRequest;
    }
  }

  return {
    ...(review_request && { review_request }),
    ...(review_record && { review_record }),
    ...(review_unparsed && { review_unparsed: true }),
  };
}

function buildWorkItem(
  repository: Repository,
  columnRepo: Repository,
  issue: ParsedIssue,
  delivery: Overlay<PullRequestRef[]>,
  flakeHolds: string[],
  boardFields?: ProjectItemFields,
): WorkItem {
  const baton = deriveBatonOverlays({
    labels: issue.labels,
    assignees: issue.assignees,
    dependencies: issue.dependencies,
    delivery,
    flake_holds: flakeHolds,
  });

  const pm_status = columnRepo.project_board ? boardFields?.pm_status : undefined;
  const priority = columnRepo.project_board?.priority_field ? boardFields?.priority : undefined;

  return {
    ref: `${repository.owner}/${repository.name}#${issue.number}`,
    repository: `${repository.owner}/${repository.name}`,
    number: issue.number,
    title: issue.title,
    body_excerpt: excerpt(issue.body),
    state: issue.state,
    labels: issue.labels,
    assignees: issue.assignees,
    ...(issue.milestone && { milestone: issue.milestone }),
    ...(pm_status && { pm_status }),
    ...(priority && { priority }),
    column: columnFor(
      { state: issue.state, assignees: issue.assignees, pm_status },
      columnRepo,
    ),
    dependencies: issue.dependencies,
    delivery,
    ...baton,
  };
}

// ---------------------------------------------------------------------------
// Source implementation
// ---------------------------------------------------------------------------

export function createGithubSource(options: GithubSourceOptions = {}): WorkSource {
  const apiBase = (options.apiUrl?.trim() || DEFAULT_GITHUB_API_URL).replace(/\/+$/, "");
  const token = options.token?.trim() || undefined;

  async function enrichOpenPullsWithComments(
    repository: Repository,
    pulls: ParsedPullRequest[],
    targetIssueNumbers: Set<number>,
  ): Promise<string[]> {
    const reasons: string[] = [];
    const relevantOpenPulls = pulls.filter(
      (pr) =>
        pr.ref.state === "open" &&
        pr.claimedIssueNumbers.some((n) => targetIssueNumbers.has(n)),
    );

    const toFetch = relevantOpenPulls.slice(0, MAX_OPEN_PR_COMMENT_FETCHES);
    const skipped = relevantOpenPulls.slice(MAX_OPEN_PR_COMMENT_FETCHES);
    if (skipped.length > 0) {
      reasons.push(
        `${repository.owner}/${repository.name} open PR comment reads capped at ${MAX_OPEN_PR_COMMENT_FETCHES} (${skipped.length} uninspected)`,
      );
      for (const pr of skipped) {
        pr.ref.review_inspected = false;
      }
    }

    await Promise.all(
      toFetch.map(async (pr) => {
        const path = `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.name)}/issues/${pr.number}/comments?per_page=${GITHUB_PAGE_LIMIT}`;
        const label = `GET /repos/${repository.owner}/${repository.name}/issues/${pr.number}/comments`;
        const read = await getGithubJson(apiBase, path, label, token);
        if (!read.ok || !Array.isArray(read.value)) {
          pr.ref.review_inspected = false;
          reasons.push(read.ok ? `${label} returned a non-array body` : read.error);
          return;
        }
        const parsed = parseReviewComments(read.value);
        if (parsed.review_request) pr.ref.review_request = parsed.review_request;
        if (parsed.review_record) pr.ref.review_record = parsed.review_record;
        if (read.value.length >= GITHUB_PAGE_LIMIT) {
          reasons.push(`${label} truncated at ${GITHUB_PAGE_LIMIT} comments`);
        }
        if (parsed.review_unparsed) {
          reasons.push(`${label} contains an unparseable review record`);
          pr.ref.review_inspected = false;
        } else if (read.value.length >= GITHUB_PAGE_LIMIT) {
          // A definitive review_record found in the page still holds if its SHA
          // matches HEAD; otherwise absence or freshness is unproven.
          pr.ref.review_inspected = Boolean(
            parsed.review_record &&
              pr.ref.head_sha &&
              shaMatchesHead(parsed.review_record.sha, pr.ref.head_sha),
          );
        } else {
          pr.ref.review_inspected = true;
        }
      }),
    );

    return reasons;
  }

  return {
    kind: "github",

    async listItems(repository: Repository) {
      const ownerEnc = encodeURIComponent(repository.owner);
      const nameEnc = encodeURIComponent(repository.name);
      const issuesPath = `/repos/${ownerEnc}/${nameEnc}/issues?state=open&per_page=${GITHUB_PAGE_LIMIT}`;
      const pullsPath = `/repos/${ownerEnc}/${nameEnc}/pulls?state=all&per_page=${GITHUB_PAGE_LIMIT}`;
      const issuesLabel = `GET /repos/${repository.owner}/${repository.name}/issues`;
      const pullsLabel = `GET /repos/${repository.owner}/${repository.name}/pulls`;

      const [issuesRead, pullsRead, projectBoard] = await Promise.all([
        getGithubJson(apiBase, issuesPath, issuesLabel, token),
        getGithubJson(apiBase, pullsPath, pullsLabel, token),
        readProjectBoard(apiBase, repository, token),
      ]);

      const columnRepo: Repository =
        projectBoard.configured && !projectBoard.ok
          ? { ...repository, project_board: undefined }
          : repository;
      const statusOptions =
        projectBoard.configured && projectBoard.ok ? projectBoard.statusOptions : [];

      if (!issuesRead.ok || !Array.isArray(issuesRead.value)) {
        const reason = issuesRead.ok ? `${issuesLabel} returned a non-array body` : issuesRead.error;
        return {
          lookup: "failed",
          reason,
          columns: columnsFor(columnRepo, statusOptions, []),
          items: [],
        };
      }

      const reasons: string[] = [];
      if (issuesRead.value.length >= GITHUB_PAGE_LIMIT) {
        reasons.push(`${issuesLabel} truncated at ${GITHUB_PAGE_LIMIT} items`);
      }
      if (projectBoard.configured) {
        if (!projectBoard.ok) {
          reasons.push(projectBoard.error);
        } else if (projectBoard.truncationReason) {
          reasons.push(projectBoard.truncationReason);
        }
      }

      const parsedIssues = issuesRead.value
        .filter(isObject)
        .map((raw) => parseIssue(raw, repository))
        .filter((i): i is ParsedIssue => i !== null);

      const flakeHolds = parsedIssues
        .filter((i) => i.labels.includes("class:flake"))
        .map((i) => `${repository.owner}/${repository.name}#${i.number}`);

      let pullsOk = false;
      let pullsTruncated = false;
      let pullsError = "";
      let parsedPulls: ParsedPullRequest[] = [];

      if (!pullsRead.ok || !Array.isArray(pullsRead.value)) {
        pullsError = pullsRead.ok ? `${pullsLabel} returned a non-array body` : pullsRead.error;
        reasons.push(pullsError);
      } else {
        pullsOk = true;
        pullsTruncated = pullsRead.value.length >= GITHUB_PAGE_LIMIT;
        if (pullsTruncated) {
          reasons.push(`${pullsLabel} truncated at ${GITHUB_PAGE_LIMIT} items`);
        }
        parsedPulls = pullsRead.value
          .filter(isObject)
          .map((raw) => parsePullRequest(raw, repository))
          .filter((pr): pr is ParsedPullRequest => pr !== null);

        const issueNumbers = new Set(parsedIssues.map((i) => i.number));
        const commentReasons = await enrichOpenPullsWithComments(repository, parsedPulls, issueNumbers);
        reasons.push(...commentReasons);
      }

      const items = parsedIssues.map((issue) => {
        let delivery: Overlay<PullRequestRef[]>;
        if (!pullsOk) {
          delivery = { state: "unknown", reason: pullsError };
        } else {
          const matched = parsedPulls
            .filter((pr) => pr.claimedIssueNumbers.includes(issue.number))
            .map((pr) => pr.ref);
          if (matched.length > 0) {
            delivery = { state: "known", value: matched };
          } else if (pullsTruncated) {
            delivery = {
              state: "unknown",
              reason: `${pullsLabel} truncated at ${GITHUB_PAGE_LIMIT} items`,
            };
          } else {
            delivery = { state: "none" };
          }
        }
        const boardFields =
          projectBoard.configured && projectBoard.ok
            ? projectBoard.itemsByIssueNumber.get(issue.number)
            : undefined;
        return buildWorkItem(repository, columnRepo, issue, delivery, flakeHolds, boardFields);
      });

      const lookup: LookupStatus = reasons.length === 0 ? "complete" : "partial";
      return {
        lookup,
        ...(reasons.length > 0 && { reason: [...new Set(reasons)].join("; ") }),
        columns: columnsFor(columnRepo, statusOptions, items.map((i) => i.column)),
        items,
      };
    },

    async getItem({ repository, number }: ItemRef) {
      const ownerEnc = encodeURIComponent(repository.owner);
      const nameEnc = encodeURIComponent(repository.name);
      const issuePath = `/repos/${ownerEnc}/${nameEnc}/issues/${number}`;
      const pullsPath = `/repos/${ownerEnc}/${nameEnc}/pulls?state=all&per_page=${GITHUB_PAGE_LIMIT}`;
      const flakesPath = `/repos/${ownerEnc}/${nameEnc}/issues?state=open&labels=class:flake&per_page=${GITHUB_PAGE_LIMIT}`;
      const issueLabel = `GET /repos/${repository.owner}/${repository.name}/issues/${number}`;
      const pullsLabel = `GET /repos/${repository.owner}/${repository.name}/pulls`;
      const flakesLabel = `GET /repos/${repository.owner}/${repository.name}/issues?labels=class:flake`;

      const [issueRead, pullsRead, flakesRead, projectBoard] = await Promise.all([
        getGithubJson(apiBase, issuePath, issueLabel, token),
        getGithubJson(apiBase, pullsPath, pullsLabel, token),
        getGithubJson(apiBase, flakesPath, flakesLabel, token),
        readProjectBoard(apiBase, repository, token),
      ]);

      if (!issueRead.ok) {
        if (issueRead.status === 404) {
          return { lookup: "complete" };
        }
        return { lookup: "failed", reason: issueRead.error };
      }
      if (!isObject(issueRead.value)) {
        return { lookup: "failed", reason: `${issueLabel} returned a non-object body` };
      }

      const parsedIssue = parseIssue(issueRead.value, repository);
      if (!parsedIssue) {
        // A pull request number hit on /issues/{number} is not a work item.
        return { lookup: "complete" };
      }

      const reasons: string[] = [];
      if (projectBoard.configured) {
        if (!projectBoard.ok) {
          reasons.push(projectBoard.error);
        } else if (projectBoard.truncationReason) {
          reasons.push(projectBoard.truncationReason);
        }
      }

      let flakeHolds: string[] = [];
      if (!flakesRead.ok || !Array.isArray(flakesRead.value)) {
        reasons.push(flakesRead.ok ? `${flakesLabel} returned a non-array body` : flakesRead.error);
      } else {
        flakeHolds = flakesRead.value
          .filter(isObject)
          .map((raw) => parseIssue(raw, repository))
          .filter((i): i is ParsedIssue => i !== null)
          .map((i) => `${repository.owner}/${repository.name}#${i.number}`);
      }

      let delivery: Overlay<PullRequestRef[]>;
      if (!pullsRead.ok || !Array.isArray(pullsRead.value)) {
        const err = pullsRead.ok ? `${pullsLabel} returned a non-array body` : pullsRead.error;
        reasons.push(err);
        delivery = { state: "unknown", reason: err };
      } else {
        const pullsTruncated = pullsRead.value.length >= GITHUB_PAGE_LIMIT;
        if (pullsTruncated) {
          reasons.push(`${pullsLabel} truncated at ${GITHUB_PAGE_LIMIT} items`);
        }
        const parsedPulls = pullsRead.value
          .filter(isObject)
          .map((raw) => parsePullRequest(raw, repository))
          .filter((pr): pr is ParsedPullRequest => pr !== null);

        const commentReasons = await enrichOpenPullsWithComments(
          repository,
          parsedPulls,
          new Set([number]),
        );
        reasons.push(...commentReasons);

        const matched = parsedPulls
          .filter((pr) => pr.claimedIssueNumbers.includes(number))
          .map((pr) => pr.ref);
        if (matched.length > 0) {
          delivery = { state: "known", value: matched };
        } else if (pullsTruncated) {
          delivery = {
            state: "unknown",
            reason: `${pullsLabel} truncated at ${GITHUB_PAGE_LIMIT} items`,
          };
        } else {
          delivery = { state: "none" };
        }
      }

      const columnRepo: Repository =
        projectBoard.configured && !projectBoard.ok
          ? { ...repository, project_board: undefined }
          : repository;
      const boardFields =
        projectBoard.configured && projectBoard.ok
          ? projectBoard.itemsByIssueNumber.get(number)
          : undefined;

      const lookup: LookupStatus = reasons.length === 0 ? "complete" : "partial";
      return {
        lookup,
        ...(reasons.length > 0 && { reason: [...new Set(reasons)].join("; ") }),
        item: buildWorkItem(repository, columnRepo, parsedIssue, delivery, flakeHolds, boardFields),
      };
    },

    async linkedRunBindings({ repository, number }: ItemRef) {
      return {
        lookup: "complete",
        bindings: [
          {
            by: "work_item_ref",
            ref: `${repository.owner}/${repository.name}#${number}`,
          },
        ],
      };
    },
  };
}

