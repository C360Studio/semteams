// Work API configuration (design D1). Read from the environment at request
// time, not module load, so the source can change without a rebuild and tests
// can set it per case.

import { env } from "$env/dynamic/private";
import type { PortfolioIssue } from "$lib/types/work";

export type WorkSourceMode = "fixture" | "github" | "unconfigured";

export interface WorkConfig {
  source: WorkSourceMode;
  /** Fixture set name; only meaningful when source is "fixture". */
  fixtureSet: string;
  /** Portfolio document path; only read when source is "github". */
  portfolioPath?: string;
  /** Backend `host:port` for server-side run reads. */
  backendHost: string;
}

/**
 * The work API cannot serve: the environment, the portfolio document or the
 * fixture set is wrong. Surfaces as 503 with `code` (and `issues` for a
 * document problem) so the board can show what to fix.
 */
export class WorkConfigError extends Error {
  readonly code: string;
  readonly issues?: PortfolioIssue[];

  constructor(message: string, code = "WORK_CONFIG_INVALID", issues?: PortfolioIssue[]) {
    super(message);
    this.name = "WorkConfigError";
    this.code = code;
    this.issues = issues;
  }
}

export const DEFAULT_FIXTURE_SET = "board-mvp";
export const DEFAULT_BACKEND_HOST = "localhost:8080";

function nonEmpty(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

/** Throws WorkConfigError for an unrecognised WORK_SOURCE rather than guessing. */
export function readWorkConfig(): WorkConfig {
  const rawSource = nonEmpty(env.WORK_SOURCE);
  if (
    rawSource !== undefined &&
    rawSource !== "fixture" &&
    rawSource !== "github"
  ) {
    throw new WorkConfigError(
      `WORK_SOURCE must be "fixture" or "github" (or unset), got "${rawSource}"`,
    );
  }
  return {
    source: rawSource ?? "unconfigured",
    fixtureSet: nonEmpty(env.WORK_FIXTURE_SET) ?? DEFAULT_FIXTURE_SET,
    portfolioPath: nonEmpty(env.SEMTEAMS_PORTFOLIO_PATH),
    backendHost: nonEmpty(env.BACKEND_HOST) ?? DEFAULT_BACKEND_HOST,
  };
}
