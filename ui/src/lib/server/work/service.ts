// Work API orchestration (design D1). Routes stay thin: they parse nothing and
// decide nothing, and call one function here that returns a status and a body.
// Every function is GET-only by construction: nothing here writes anywhere.

import { readFile } from "node:fs/promises";
import type {
  ItemResponse,
  ItemsResponse,
  Portfolio,
  PortfolioResponse,
  Repository,
  RunsResponse,
  WorkApiError,
} from "$lib/types/work";
import { readWorkConfig, WorkConfigError } from "./config";
import type { WorkConfig } from "./config";
import { createFixtureSource, listFixtureSets, loadFixtureSet } from "./fixtureSource";
import { resolveLinkedRuns } from "./overlays";
import { findRepository, validatePortfolio } from "./portfolio";
import { createGithubSource } from "./source";
import type { WorkSource } from "./source";

export interface WorkResult<T> {
  status: number;
  body: T | WorkApiError;
}

type Context =
  | { kind: "unconfigured" }
  | { kind: "ready"; config: WorkConfig; portfolio: Portfolio; source: WorkSource };

function failure(status: number, code: string, error: string): WorkResult<never> {
  return { status, body: { code, error } };
}

async function loadContext(): Promise<Context> {
  const config = readWorkConfig();
  if (config.source === "unconfigured") return { kind: "unconfigured" };

  if (config.source === "fixture") {
    const set = loadFixtureSet(config.fixtureSet);
    if (!set) {
      throw new WorkConfigError(
        `unknown WORK_FIXTURE_SET; available: ${listFixtureSets().join(", ") || "none"}`,
        "FIXTURE_SET_UNKNOWN",
        undefined,
        `requested "${config.fixtureSet}"`,
      );
    }
    const checked = validatePortfolio(set.portfolio);
    if (!checked.ok) {
      throw new WorkConfigError(
        "fixture set has an invalid portfolio",
        "PORTFOLIO_INVALID",
        checked.issues,
        `fixture set "${config.fixtureSet}"`,
      );
    }
    return {
      kind: "ready",
      config,
      portfolio: checked.portfolio,
      source: createFixtureSource(checked.portfolio, set.items),
    };
  }

  if (!config.portfolioPath) {
    throw new WorkConfigError(
      "SEMTEAMS_PORTFOLIO_PATH is required when WORK_SOURCE=github",
      "PORTFOLIO_PATH_REQUIRED",
    );
  }
  let document: unknown;
  try {
    document = JSON.parse(await readFile(config.portfolioPath, "utf8"));
  } catch (err) {
    // The path and the raw read/parse message stay in the server log: the
    // browser gets the code, not the operator's filesystem layout.
    throw new WorkConfigError(
      "portfolio document is unreadable",
      "PORTFOLIO_UNREADABLE",
      undefined,
      `${config.portfolioPath}: ${err instanceof Error ? err.message : String(err)}`,
    );
  }
  const checked = validatePortfolio(document);
  if (!checked.ok) {
    throw new WorkConfigError(
      "portfolio document is invalid",
      "PORTFOLIO_INVALID",
      checked.issues,
      config.portfolioPath,
    );
  }
  return { kind: "ready", config, portfolio: checked.portfolio, source: createGithubSource() };
}

/**
 * Runs a handler against the loaded context; a config problem becomes 503 with
 * its code and validation issues. The error's `detail` (paths, raw exception
 * text) is logged here and never sent.
 */
async function withContext<T>(
  handler: (context: Context) => Promise<WorkResult<T>>,
): Promise<WorkResult<T>> {
  try {
    return await handler(await loadContext());
  } catch (err) {
    if (err instanceof WorkConfigError) {
      console.error(`[work-api] ${err.code}: ${err.message}${err.detail ? ` (${err.detail})` : ""}`);
      return {
        status: 503,
        body: {
          code: err.code,
          error: err.message,
          ...(err.issues && { issues: err.issues }),
        },
      };
    }
    throw err;
  }
}

function repositoryId(repository: Repository): string {
  return `${repository.owner}/${repository.name}`;
}

const PART = /^[^/\s]+$/;

function parseItemNumber(raw: string): number | null {
  return /^[1-9]\d*$/.test(raw) ? Number(raw) : null;
}

type Scoped =
  | { ok: true; context: Extract<Context, { kind: "ready" }>; repository: Repository }
  | { ok: false; result: WorkResult<never> };

function scope(context: Context, owner: string, name: string): Scoped {
  // Not a missing repository: the whole work source is absent, which is a
  // deployment state (503), not a property of this request (404).
  if (context.kind === "unconfigured") {
    return {
      ok: false,
      result: failure(503, "WORK_SOURCE_UNCONFIGURED", "work source is not configured; set WORK_SOURCE"),
    };
  }
  const repository = findRepository(context.portfolio, owner, name);
  if (!repository) {
    return {
      ok: false,
      result: failure(404, "REPOSITORY_NOT_CONFIGURED", `repository ${owner}/${name} is not in the portfolio`),
    };
  }
  return { ok: true, context, repository };
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

export function getPortfolio(): Promise<WorkResult<PortfolioResponse>> {
  return withContext<PortfolioResponse>(async (context) => {
    if (context.kind === "unconfigured") {
      return { status: 200, body: { source: "unconfigured", programs: [] } };
    }
    return {
      status: 200,
      body: { source: context.source.kind, programs: context.portfolio.programs },
    };
  });
}

export function getItems(repositoryParam: string | null): Promise<WorkResult<ItemsResponse>> {
  if (!repositoryParam) {
    return Promise.resolve(failure(400, "REPOSITORY_REQUIRED", "query parameter repository=owner/name is required"));
  }
  const [owner, name, ...extra] = repositoryParam.split("/");
  if (!owner || !name || extra.length > 0 || !PART.test(owner) || !PART.test(name)) {
    return Promise.resolve(failure(400, "REPOSITORY_INVALID", "repository must look like owner/name"));
  }
  return withContext<ItemsResponse>(async (context) => {
    const scoped = scope(context, owner, name);
    if (!scoped.ok) return scoped.result;
    const result = await scoped.context.source.listItems(scoped.repository);
    return {
      status: 200,
      body: {
        lookup: result.lookup,
        ...(result.reason && { reason: result.reason }),
        repository: repositoryId(scoped.repository),
        columns: result.columns,
        items: result.items,
        ...(result.next_cursor && { next_cursor: result.next_cursor }),
      },
    };
  });
}

export function getItem(owner: string, name: string, numberParam: string): Promise<WorkResult<ItemResponse>> {
  const number = parseItemNumber(numberParam);
  if (number === null) {
    return Promise.resolve(failure(400, "ITEM_NUMBER_INVALID", "item number must be a positive integer"));
  }
  return withContext<ItemResponse>(async (context) => {
    const scoped = scope(context, owner, name);
    if (!scoped.ok) return scoped.result;
    const result = await scoped.context.source.getItem({ repository: scoped.repository, number });
    if (result.lookup === "complete" && !result.item) {
      return failure(404, "ITEM_NOT_FOUND", `${repositoryId(scoped.repository)}#${number} does not exist`);
    }
    return {
      status: 200,
      body: {
        lookup: result.lookup,
        ...(result.reason && { reason: result.reason }),
        ...(result.item && { item: result.item }),
      },
    };
  });
}

export function getRuns(owner: string, name: string, numberParam: string): Promise<WorkResult<RunsResponse>> {
  const number = parseItemNumber(numberParam);
  if (number === null) {
    return Promise.resolve(failure(400, "ITEM_NUMBER_INVALID", "item number must be a positive integer"));
  }
  return withContext<RunsResponse>(async (context) => {
    const scoped = scope(context, owner, name);
    if (!scoped.ok) return scoped.result;
    const ref = { repository: scoped.repository, number };
    const { source, config } = scoped.context;

    const item = await source.getItem(ref);
    if (item.lookup === "complete" && !item.item) {
      return failure(404, "ITEM_NOT_FOUND", `${repositoryId(scoped.repository)}#${number} does not exist`);
    }

    const declared = await source.linkedRunBindings(ref);
    if (declared.lookup !== "complete") {
      return {
        status: 200,
        body: {
          lookup: declared.lookup,
          ...(declared.reason && { reason: declared.reason }),
          runs: [],
        },
      };
    }
    const resolution = await resolveLinkedRuns(declared.bindings, config.backendHost);
    return {
      status: 200,
      body: {
        lookup: resolution.lookup,
        ...(resolution.reason && { reason: resolution.reason }),
        runs: resolution.runs,
      },
    };
  });
}
