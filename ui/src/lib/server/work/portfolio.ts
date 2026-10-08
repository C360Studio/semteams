// Portfolio document validation (design D4). Dependency-free on purpose: the
// document is operator-authored and the board must name exactly what is wrong
// with it rather than render a half-understood portfolio. The hand-authored
// JSON Schema at configs/portfolio/portfolio.schema.json describes the same
// shape for the Go readers (#267, #273); uniqueness rules live here.

import type {
  Portfolio,
  PortfolioIssue,
  Program,
  Project,
  ProjectBoard,
  Repository,
} from "$lib/types/work";

export type PortfolioValidation =
  | { ok: true; portfolio: Portfolio }
  | { ok: false; issues: PortfolioIssue[] };

type Json = Record<string, unknown>;

function isObject(value: unknown): value is Json {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function nonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

/** `owner/name`, compared case-insensitively as GitHub does. */
export function repositoryKey(owner: string, name: string): string {
  return `${owner}/${name}`.toLowerCase();
}

export function validatePortfolio(input: unknown): PortfolioValidation {
  const issues: PortfolioIssue[] = [];
  const issue = (path: string, message: string) => issues.push({ path, message });

  function rejectUnknownKeys(obj: Json, allowed: string[], path: string) {
    for (const key of Object.keys(obj)) {
      if (!allowed.includes(key)) issue(`${path}.${key}`, "unknown property");
    }
  }

  function requireString(obj: Json, key: string, path: string): string | null {
    const value = obj[key];
    if (nonEmptyString(value)) return value;
    issue(`${path}.${key}`, "required, must be a non-empty string");
    return null;
  }

  function parseBoard(raw: unknown, path: string): ProjectBoard | undefined {
    if (!isObject(raw)) {
      issue(path, "must be an object");
      return undefined;
    }
    rejectUnknownKeys(
      raw,
      ["owner", "number", "status_field", "priority_field"],
      path,
    );
    const owner = requireString(raw, "owner", path);
    const statusField = requireString(raw, "status_field", path);
    const number = raw.number;
    if (typeof number !== "number" || !Number.isInteger(number) || number < 1) {
      issue(`${path}.number`, "required, must be a positive integer");
    }
    let priorityField: string | undefined;
    if (raw.priority_field !== undefined) {
      if (nonEmptyString(raw.priority_field)) priorityField = raw.priority_field;
      else issue(`${path}.priority_field`, "must be a non-empty string when present");
    }
    if (owner === null || statusField === null || typeof number !== "number") {
      return undefined;
    }
    return {
      owner,
      number,
      status_field: statusField,
      ...(priorityField !== undefined && { priority_field: priorityField }),
    };
  }

  function parseRepository(raw: unknown, path: string): Repository | null {
    if (!isObject(raw)) {
      issue(path, "must be an object");
      return null;
    }
    rejectUnknownKeys(raw, ["owner", "name", "project_board"], path);
    const owner = requireString(raw, "owner", path);
    const name = requireString(raw, "name", path);
    const board =
      raw.project_board === undefined
        ? undefined
        : parseBoard(raw.project_board, `${path}.project_board`);
    if (owner === null || name === null) return null;
    return { owner, name, ...(board !== undefined && { project_board: board }) };
  }

  function parseProject(raw: unknown, path: string, seenIds: Set<string>): Project | null {
    if (!isObject(raw)) {
      issue(path, "must be an object");
      return null;
    }
    rejectUnknownKeys(raw, ["id", "name", "repositories"], path);
    const id = requireString(raw, "id", path);
    const name = requireString(raw, "name", path);
    if (id !== null) {
      if (seenIds.has(id)) issue(`${path}.id`, `duplicate project id "${id}"`);
      seenIds.add(id);
    }
    const repositories: Repository[] = [];
    if (!Array.isArray(raw.repositories)) {
      issue(`${path}.repositories`, "required, must be an array");
    } else {
      const seenRepos = new Set<string>();
      raw.repositories.forEach((entry, i) => {
        const repoPath = `${path}.repositories[${i}]`;
        const repo = parseRepository(entry, repoPath);
        if (!repo) return;
        const key = repositoryKey(repo.owner, repo.name);
        if (seenRepos.has(key)) {
          issue(repoPath, `repository ${repo.owner}/${repo.name} listed twice in this project`);
        }
        seenRepos.add(key);
        repositories.push(repo);
      });
    }
    if (id === null || name === null) return null;
    return { id, name, repositories };
  }

  function parseProgram(
    raw: unknown,
    path: string,
    seenProgramIds: Set<string>,
    seenProjectIds: Set<string>,
  ): Program | null {
    if (!isObject(raw)) {
      issue(path, "must be an object");
      return null;
    }
    rejectUnknownKeys(raw, ["id", "name", "projects"], path);
    const id = requireString(raw, "id", path);
    const name = requireString(raw, "name", path);
    if (id !== null) {
      if (seenProgramIds.has(id)) issue(`${path}.id`, `duplicate program id "${id}"`);
      seenProgramIds.add(id);
    }
    const projects: Project[] = [];
    if (!Array.isArray(raw.projects)) {
      issue(`${path}.projects`, "required, must be an array");
    } else {
      raw.projects.forEach((entry, i) => {
        const project = parseProject(entry, `${path}.projects[${i}]`, seenProjectIds);
        if (project) projects.push(project);
      });
    }
    if (id === null || name === null) return null;
    return { id, name, projects };
  }

  if (!isObject(input)) {
    return { ok: false, issues: [{ path: "$", message: "document must be a JSON object" }] };
  }
  rejectUnknownKeys(input, ["programs"], "$");

  const programs: Program[] = [];
  if (!Array.isArray(input.programs)) {
    issue("$.programs", "required, must be an array");
  } else {
    const seenProgramIds = new Set<string>();
    const seenProjectIds = new Set<string>();
    input.programs.forEach((entry, i) => {
      const program = parseProgram(entry, `$.programs[${i}]`, seenProgramIds, seenProjectIds);
      if (program) programs.push(program);
    });
  }

  // A repository may sit under several projects, but the board decides its
  // columns, so every appearance must agree on it.
  const boardByRepo = new Map<string, string>();
  for (const program of programs) {
    for (const project of program.projects) {
      for (const repo of project.repositories) {
        const key = repositoryKey(repo.owner, repo.name);
        const board = JSON.stringify(repo.project_board ?? null);
        const seen = boardByRepo.get(key);
        if (seen !== undefined && seen !== board) {
          issue(
            `$.programs[*].projects[${project.id}].repositories`,
            `repository ${repo.owner}/${repo.name} appears with conflicting project_board settings`,
          );
        }
        boardByRepo.set(key, board);
      }
    }
  }

  return issues.length > 0 ? { ok: false, issues } : { ok: true, portfolio: { programs } };
}

/** First configured repository matching `owner/name` (case-insensitive), or undefined. */
export function findRepository(
  portfolio: Portfolio,
  owner: string,
  name: string,
): Repository | undefined {
  const wanted = repositoryKey(owner, name);
  for (const program of portfolio.programs) {
    for (const project of program.projects) {
      for (const repo of project.repositories) {
        if (repositoryKey(repo.owner, repo.name) === wanted) return repo;
      }
    }
  }
  return undefined;
}
