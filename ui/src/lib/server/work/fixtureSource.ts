// Fixture-backed work source (design D5). Fixture sets are bundled with the
// server (import.meta.glob) so the production build needs no source tree at
// runtime, and a set name can never reach the filesystem.

import type {
  PortfolioIssue,
  PullRequestRef,
  Portfolio,
  Repository,
  WorkDependency,
  WorkItem,
  WorkItemState,
} from "$lib/types/work";
import { columnFor, columnsFor } from "./columns";
import { WorkConfigError } from "./config";
import { findRepository, repositoryKey } from "./portfolio";
import type { ItemRef, LinkedRunBinding, WorkSource } from "./source";

const portfolioFiles = import.meta.glob("./fixtures/*/portfolio.json", {
  eager: true,
  import: "default",
}) as Record<string, unknown>;
const itemFiles = import.meta.glob("./fixtures/*/items.json", {
  eager: true,
  import: "default",
}) as Record<string, unknown>;

function setNameOf(path: string): string {
  return path.split("/").slice(-2, -1)[0] ?? "";
}

export function listFixtureSets(): string[] {
  return Object.keys(portfolioFiles).map(setNameOf).sort();
}

export function loadFixtureSet(
  name: string,
): { portfolio: unknown; items: unknown } | undefined {
  const portfolioPath = Object.keys(portfolioFiles).find((p) => setNameOf(p) === name);
  const itemsPath = Object.keys(itemFiles).find((p) => setNameOf(p) === name);
  if (!portfolioPath || !itemsPath) return undefined;
  return { portfolio: portfolioFiles[portfolioPath], items: itemFiles[itemsPath] };
}

// ---------------------------------------------------------------------------
// Fixture file shape: one object keyed by `owner/name`.
// ---------------------------------------------------------------------------

interface FixtureItem {
  number: number;
  title: string;
  body?: string;
  state: WorkItemState;
  labels?: string[];
  assignees?: string[];
  milestone?: string;
  pm_status?: string;
  priority?: string;
  dependencies?: WorkDependency[];
  /** Absent = the source does not supply PRs (unknown); [] = none. */
  pull_requests?: PullRequestRef[];
  /** Absent behaves like [] in fixture mode. */
  linked_runs?: LinkedRunBinding[];
}

interface FixtureRepository {
  /** The project board's status options, in column order. */
  status_options?: string[];
  items: FixtureItem[];
}

type FixtureItems = Record<string, FixtureRepository>;

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((v) => typeof v === "string");
}

function parseBinding(raw: unknown): LinkedRunBinding | null {
  if (!isObject(raw)) return null;
  if (raw.by === "run_entity_id" && typeof raw.value === "string" && raw.value) {
    return { by: "run_entity_id", value: raw.value };
  }
  if (raw.by === "coordinator_prompt" && typeof raw.equals === "string" && raw.equals) {
    return { by: "coordinator_prompt", equals: raw.equals };
  }
  return null;
}

function parseFixtureItems(raw: unknown, portfolio: Portfolio): FixtureItems {
  const issues: PortfolioIssue[] = [];
  const issue = (path: string, message: string) => issues.push({ path, message });
  const out: FixtureItems = {};

  if (!isObject(raw)) {
    throw new WorkConfigError("fixture items must be an object keyed by owner/name", "FIXTURE_INVALID");
  }

  for (const [key, entry] of Object.entries(raw)) {
    const path = `$[${key}]`;
    const [owner, name, ...extra] = key.split("/");
    if (!owner || !name || extra.length > 0 || !findRepository(portfolio, owner, name)) {
      issue(path, "repository is not in the fixture portfolio");
      continue;
    }
    if (!isObject(entry) || !Array.isArray(entry.items)) {
      issue(path, "must be an object with an items array");
      continue;
    }
    if (entry.status_options !== undefined && !isStringArray(entry.status_options)) {
      issue(`${path}.status_options`, "must be an array of strings");
    }
    const seen = new Set<number>();
    entry.items.forEach((rawItem: unknown, i: number) => {
      const itemPath = `${path}.items[${i}]`;
      if (!isObject(rawItem)) return issue(itemPath, "must be an object");
      if (typeof rawItem.number !== "number" || !Number.isInteger(rawItem.number) || rawItem.number < 1) {
        issue(`${itemPath}.number`, "required, must be a positive integer");
      } else if (seen.has(rawItem.number)) {
        issue(`${itemPath}.number`, `duplicate item number ${rawItem.number}`);
      } else {
        seen.add(rawItem.number);
      }
      if (typeof rawItem.title !== "string" || !rawItem.title) {
        issue(`${itemPath}.title`, "required, must be a non-empty string");
      }
      if (rawItem.state !== "open" && rawItem.state !== "closed") {
        issue(`${itemPath}.state`, 'must be "open" or "closed"');
      }
      if (rawItem.linked_runs !== undefined) {
        const ok =
          Array.isArray(rawItem.linked_runs) &&
          rawItem.linked_runs.every((b: unknown) => parseBinding(b) !== null);
        if (!ok) issue(`${itemPath}.linked_runs`, "every binding needs by + value/equals");
      }
    });
    out[key.toLowerCase()] = entry as unknown as FixtureRepository;
  }

  if (issues.length > 0) {
    throw new WorkConfigError("fixture items are invalid", "FIXTURE_INVALID", issues);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Source
// ---------------------------------------------------------------------------

const EXCERPT_LENGTH = 240;

function excerpt(body: string | undefined): string {
  const flat = (body ?? "").replace(/\s+/g, " ").trim();
  return flat.length > EXCERPT_LENGTH ? `${flat.slice(0, EXCERPT_LENGTH).trimEnd()}…` : flat;
}

function toWorkItem(repository: Repository, raw: FixtureItem): WorkItem {
  const board = repository.project_board;
  // PM status and priority come from the project board; with no configured
  // source they stay unset even if a fixture carries a value.
  const pmStatus = board ? raw.pm_status?.trim() || undefined : undefined;
  const priority = board?.priority_field ? raw.priority?.trim() || undefined : undefined;
  const assignees = raw.assignees ?? [];

  let delivery: WorkItem["delivery"];
  if (raw.pull_requests === undefined) {
    delivery = { state: "unknown", reason: "source does not supply linked pull requests" };
  } else if (raw.pull_requests.length === 0) {
    delivery = { state: "none" };
  } else {
    delivery = { state: "known", value: raw.pull_requests };
  }

  return {
    ref: `${repository.owner}/${repository.name}#${raw.number}`,
    repository: `${repository.owner}/${repository.name}`,
    number: raw.number,
    title: raw.title,
    body_excerpt: excerpt(raw.body),
    state: raw.state,
    labels: raw.labels ?? [],
    assignees,
    ...(raw.milestone !== undefined && { milestone: raw.milestone }),
    ...(pmStatus !== undefined && { pm_status: pmStatus }),
    ...(priority !== undefined && { priority }),
    column: columnFor({ state: raw.state, assignees, pm_status: pmStatus }, repository),
    dependencies: raw.dependencies ?? [],
    delivery,
  };
}

/** Throws WorkConfigError (FIXTURE_INVALID) when the fixture items are malformed. */
export function createFixtureSource(portfolio: Portfolio, rawItems: unknown): WorkSource {
  const fixtures = parseFixtureItems(rawItems, portfolio);
  const forRepository = (repository: Repository): FixtureRepository | undefined =>
    fixtures[repositoryKey(repository.owner, repository.name)];

  return {
    kind: "fixture",
    async listItems(repository) {
      const fixture = forRepository(repository);
      const items = (fixture?.items ?? []).map((raw) => toWorkItem(repository, raw));
      return {
        lookup: "complete",
        columns: columnsFor(repository, fixture?.status_options ?? [], items.map((i) => i.column)),
        items,
      };
    },
    async getItem({ repository, number }: ItemRef) {
      const raw = forRepository(repository)?.items.find((i) => i.number === number);
      return raw ? { lookup: "complete", item: toWorkItem(repository, raw) } : { lookup: "complete" };
    },
    async linkedRunBindings({ repository, number }: ItemRef) {
      const raw = forRepository(repository)?.items.find((i) => i.number === number);
      const bindings = (raw?.linked_runs ?? [])
        .map(parseBinding)
        .filter((b): b is LinkedRunBinding => b !== null);
      return { lookup: "complete", bindings };
    },
  };
}
