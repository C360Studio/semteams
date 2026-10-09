// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { Portfolio } from "$lib/types/work";
import { findRepository, validatePortfolio } from "./portfolio";

const valid = (): Portfolio => ({
  programs: [
    {
      id: "c360",
      name: "c360",
      projects: [
        {
          id: "semteams",
          name: "SemTeams",
          repositories: [
            {
              owner: "C360Studio",
              name: "semteams",
              project_board: { owner: "C360Studio", number: 3, status_field: "Status", priority_field: "Priority" },
            },
            { owner: "C360Studio", name: "semsource" },
          ],
        },
      ],
    },
  ],
});

function issuesOf(doc: unknown): string[] {
  const result = validatePortfolio(doc);
  if (result.ok) throw new Error("expected the document to be invalid");
  return result.issues.map((i) => `${i.path}: ${i.message}`);
}

describe("validatePortfolio", () => {
  it("accepts a valid document and returns it typed", () => {
    const result = validatePortfolio(valid());
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.portfolio.programs[0].projects[0].repositories).toHaveLength(2);
    expect(result.portfolio.programs[0].projects[0].repositories[0].project_board?.number).toBe(3);
    expect(result.portfolio.programs[0].projects[0].repositories[1].project_board).toBeUndefined();
  });

  it("accepts the shipped example document", () => {
    const example = JSON.parse(
      readFileSync(new URL("../../../../../configs/portfolio/portfolio.example.json", import.meta.url), "utf8"),
    );
    expect(validatePortfolio(example).ok).toBe(true);
  });

  it("names a missing required field by path", () => {
    const doc = valid();
    delete (doc.programs[0].projects[0].repositories[0] as { name?: string }).name;
    expect(issuesOf(doc)).toContain(
      "$.programs[0].projects[0].repositories[0].name: required, must be a non-empty string",
    );
  });

  it("rejects a non-string repository owner", () => {
    const doc = valid();
    (doc.programs[0].projects[0].repositories[1] as { owner: unknown }).owner = 7;
    expect(issuesOf(doc)).toContain(
      "$.programs[0].projects[0].repositories[1].owner: required, must be a non-empty string",
    );
  });

  it("rejects duplicate program ids and duplicate project ids across programs", () => {
    const doc = valid();
    doc.programs.push({
      id: "c360",
      name: "again",
      projects: [{ id: "semteams", name: "dup", repositories: [] }],
    });
    const issues = issuesOf(doc);
    expect(issues).toContain('$.programs[1].id: duplicate program id "c360"');
    expect(issues).toContain('$.programs[1].projects[0].id: duplicate project id "semteams"');
  });

  it("rejects a repository listed twice in one project, ignoring case", () => {
    const doc = valid();
    doc.programs[0].projects[0].repositories.push({ owner: "c360studio", name: "SemSource" });
    expect(issuesOf(doc).join("\n")).toMatch(/listed twice in this project/);
  });

  it("allows a repository under several projects when the board agrees, and rejects it when not", () => {
    const shared = valid();
    shared.programs[0].projects.push({
      id: "intake",
      name: "Intake",
      repositories: [{ owner: "C360Studio", name: "semsource" }],
    });
    expect(validatePortfolio(shared).ok).toBe(true);

    const conflicting = valid();
    conflicting.programs[0].projects.push({
      id: "intake",
      name: "Intake",
      repositories: [
        { owner: "C360Studio", name: "semsource", project_board: { owner: "C360Studio", number: 9, status_field: "Status" } },
      ],
    });
    expect(issuesOf(conflicting).join("\n")).toMatch(/conflicting project_board settings/);
  });

  it("validates project_board fields and rejects unknown properties", () => {
    const doc = valid();
    doc.programs[0].projects[0].repositories[0].project_board = {
      owner: "C360Studio",
      number: 0,
      status_field: "",
      extra: true,
    } as never;
    const issues = issuesOf(doc);
    expect(issues.some((i) => i.endsWith("project_board.number: required, must be a positive integer"))).toBe(true);
    expect(issues.some((i) => i.endsWith("project_board.status_field: required, must be a non-empty string"))).toBe(true);
    expect(issues.some((i) => i.endsWith("project_board.extra: unknown property"))).toBe(true);
  });

  it.each([null, [], "text", 4])("rejects a non-object document (%j)", (doc) => {
    expect(issuesOf(doc)).toEqual(["$: document must be a JSON object"]);
  });

  it("requires programs to be an array", () => {
    expect(issuesOf({})).toEqual(["$.programs: required, must be an array"]);
  });
});

describe("findRepository", () => {
  it("matches owner and name case-insensitively and returns the configured spelling", () => {
    const result = validatePortfolio(valid());
    if (!result.ok) throw new Error("fixture invalid");
    expect(findRepository(result.portfolio, "c360studio", "SEMTEAMS")?.owner).toBe("C360Studio");
    expect(findRepository(result.portfolio, "c360studio", "nope")).toBeUndefined();
  });
});
