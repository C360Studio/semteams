import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";

/**
 * Journey: read-only work board over fixtures and one live ops control
 * (OpenSpec work-board-read-only, design D5; issue #296)
 *
 * Goal: An operator opens the Work lens and sees the configured portfolio as
 * a board whose columns come from GitHub fields only, with PM status,
 * execution stage and verification as three separate, honestly-unknown badges.
 * Opening an item and loading its linked runs fills the run facts in for that
 * item alone, and the run it is bound to drills into the existing runs lens,
 * where the rule-fired ops observer is explained as one control row.
 *
 * Validates:
 *   - `/work` renders both fixture repositories: the one with a Project
 *     status field gets its configured columns (Todo, In Progress, In Review,
 *     Done), the one without gets the documented fallback (Todo, In Progress,
 *     Done). Neither is an error or "not configured" state.
 *   - Every card carries `overlay-pm-status`, `overlay-execution-stage` and
 *     `overlay-verification` as separate badges, and before any item is
 *     opened the run-derived ones are `unknown` (never `none`): "linked runs
 *     are read when an item is opened".
 *   - Read-only is structural: nothing in the board is draggable and the work
 *     API answers 405 to a write.
 *   - Three linkage cases, each against the real backend:
 *       #311 declares `linked_runs: []`  -> lookup `complete`, "no linked run"
 *       #313 binds a run entity that does not exist -> lookup `partial`
 *            (a real failed resolution, never reported as "no linked run")
 *       #312 binds, by exact coordinator prompt, the run THIS spec creates
 *            -> lookup `complete`, stage `completed`, drill-in to the run
 *     and a linked run never moves a card (#312 stays in its PM column).
 *   - The runs lens folds the ops observer into the coordinator task as a
 *     control: one `control-row`, and a story row that shows the rule as
 *     unknown (#298), the run fact at spawn (`agent.run.phase` -> completed),
 *     the control identity and its applied outcome.
 *   - The table view renders the same items as the board.
 *
 * **Why a real run:** the runs lens reads live loops and trajectories, so a
 * fixture run could not be drilled into. The journey drives the same research
 * prompt as ops-run-terminal and waits for the ops-chain-observer, which can
 * only appear after the run entity reaches a terminal phase; that is also
 * what makes `agent.run.phase = completed` a settled fact before /work reads
 * it.
 *
 * **Why #313 shows a row, not "unknown, no runs":** a `run_entity_id` binding
 * always yields a run row for the run it names, even when that entity has no
 * facts; the row carries the `partial` lookup and the reason. The
 * `linked-runs-unknown` paragraph is only for a lookup that produced no rows
 * at all (for example a prompt binding that matched nothing).
 *
 * **Failure shapes:**
 *   - `work-source-state` in an error/unconfigured state: the ui container
 *     lacks `WORK_SOURCE=fixture` / `WORK_FIXTURE_SET=board-mvp` (see
 *     docker-compose.agentic-e2e.yml). A stack started before that env was
 *     added must be torn down, not reused.
 *   - #312 shows `partial` with "prompt binding matched no coordinator loop":
 *     the coordinator's `agent.loop.description` no longer equals the prompt
 *     exactly, or `/graph/triples` stopped answering the object filter.
 *   - No `control-row` on the coordinator: the observer is classified from
 *     its `task_id` (`rule-<run entity>-<nanos>`) and folds in only after a
 *     graph read confirms it is not a run member (no `agent.loop.run`).
 *   - #312 shows two `linked-run-row`s: the stack was reused. A prompt
 *     binding matches every coordinator carrying that exact prompt, so the
 *     journey needs a fresh stack (the Taskfile entry starts one).
 *
 * **Known gap (see the two expected-failure tests at the end):** the in-page
 * Table toggle and card selection do not work in a real browser because
 * `replaceState` does not update `page.url`. This journey therefore reaches
 * the table view and every item by URL, as chain-drill-in reaches its task.
 *
 * Required fixture: test/fixtures/journeys/ops-run-terminal.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (ops rule wired)
 * Required env:     WORK_SOURCE=fixture, WORK_FIXTURE_SET=board-mvp on the ui
 *                   container (docker-compose.agentic-e2e.yml)
 *
 * Run via: task ui:test:e2e:agentic:work-board
 */

const PROMPT =
  "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?";

const CHAIN_TIMEOUT_MS = 120_000;
const OPS_TIMEOUT_MS = 90_000;
const DISPATCH_READY_TIMEOUT_MS = 60_000;

const SEMTEAMS = "c360studio/semteams";
const SEMSOURCE = "c360studio/semsource";
const NO_RUNS_ITEM = `${SEMTEAMS}#311`;
const PROMPT_BOUND_ITEM = `${SEMTEAMS}#312`;
const DEAD_RUN_ITEM = `${SEMTEAMS}#313`;
// From fixtures/board-mvp/items.json: a run entity that cannot exist.
const DEAD_RUN_ID =
  "c360.semteams.chain.agent.execution.00000000-0000-4000-8000-00000000dead";

interface LoopSummary {
  loop_id: string;
  role: string;
  state: string;
  task_id: string;
}

async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 500;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}

/**
 * Post the research prompt and return the coordinator loop id.
 *
 * Right after the stack reports /health 200, the dispatch loop state can
 * still be unavailable: the first posts answer 503 "loop state is not
 * available right now; retry shortly" for a few seconds (measured
 * 2026-10-08). That specific 503 is a startup race, not a rejection of the
 * prompt, so it is retried; any other failure surfaces immediately.
 */
async function startResearchRun(request: APIRequestContext): Promise<string> {
  const deadline = Date.now() + DISPATCH_READY_TIMEOUT_MS;
  let lastStatus = 0;
  let lastBody = "";
  while (Date.now() < deadline) {
    const sent = await request.post("/teams-dispatch/message", {
      data: { content: PROMPT },
    });
    lastStatus = sent.status();
    lastBody = await sent.text();
    if (sent.ok()) {
      const body = JSON.parse(lastBody) as { in_reply_to?: string };
      expect(
        body.in_reply_to,
        `dispatch accepted the prompt but did not name the coordinator loop: ${lastBody}`,
      ).toBeTruthy();
      return body.in_reply_to!;
    }
    const startupRace =
      lastStatus === 503 && /loop state is not available/.test(lastBody);
    if (!startupRace) break;
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(
    `dispatch rejected the prompt: ${lastStatus} ${lastBody}` +
      (lastStatus === 503
        ? ` (still unavailable after ${DISPATCH_READY_TIMEOUT_MS / 1000}s)`
        : ""),
  );
}

/** Wait for the board: both repositories read, or say why the source did not answer. */
async function waitForBoard(page: Page): Promise<void> {
  const semteams = page.getByTestId(`work-repository-${slug(SEMTEAMS)}`);
  try {
    await expect(semteams.getByTestId("work-column-todo")).toBeVisible({
      timeout: 60_000,
    });
    await expect(
      page
        .getByTestId(`work-repository-${slug(SEMSOURCE)}`)
        .getByTestId("work-column-done"),
    ).toBeVisible({ timeout: 30_000 });
  } catch (err) {
    const source = page.getByTestId("work-source-state");
    const detail =
      (await source.count()) > 0
        ? `work-source-state data-state=${await source.getAttribute("data-state")} ` +
          `data-code=${await source.getAttribute("data-code")}: ${(await source.innerText()).replace(/\s+/g, " ")}`
        : "no work-source-state on the page (the board rendered but its columns did not)";
    throw new Error(
      `the work board did not render its fixture columns. ${detail}\n${String(err)}`,
    );
  }
  await expect(
    page.getByTestId("work-source-state"),
    "a ready board must not carry a source-state notice (loading, unconfigured or error)",
  ).toHaveCount(0);
}

/** `owner/repo` -> `owner-repo`, the test-id form `PortfolioSections` uses. */
function slug(repository: string): string {
  return repository.replace("/", "-");
}

function card(page: Page, ref: string) {
  return page
    .getByTestId("work-item-card")
    .and(page.locator(`[data-item="${ref}"]`));
}

/** The URL is the selection: a deep link opens the overview directly. */
async function openItem(page: Page, ref: string): Promise<void> {
  await page.goto(`/work?item=${encodeURIComponent(ref)}`);
  await expect(page.getByTestId("work-item-overview")).toHaveAttribute(
    "data-item",
    ref,
    { timeout: 30_000 },
  );
}

async function loadLinkedRuns(page: Page): Promise<void> {
  const runs = page.getByTestId("work-linked-runs");
  // Before the operator asks, the lookup has not happened and must not
  // claim an answer in either direction.
  await expect(runs).toHaveAttribute("data-lookup", "not-loaded");
  await page.getByTestId("load-linked-runs").click();
  await expect(runs).not.toHaveAttribute("data-lookup", "not-loaded", {
    timeout: 30_000,
  });
  await expect(runs).not.toHaveAttribute("data-lookup", "loading");
}

test.describe("Work board: fixtures + live ops control", () => {
  test.setTimeout(CHAIN_TIMEOUT_MS + OPS_TIMEOUT_MS + 240_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("board renders, linkage cases are honest, the bound run drills into its explained control", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — drive one research run to a terminal phase and capture the
    // coordinator loop id. The ops observer exists only after the RUN
    // entity is terminal, so its completion settles agent.run.phase.
    // -----------------------------------------------------------------
    const coordinatorId = await startResearchRun(request);

    let observer: LoopSummary | null = null;
    const settled = await pollUntil(
      async () => {
        const resp = await request.get("/teams-dispatch/loops");
        if (!resp.ok()) return null;
        const list = (await resp.json()) as LoopSummary[];
        observer =
          list.find(
            (l) =>
              l.role === "ops-chain-observer" &&
              l.task_id.includes(`.chain.agent.execution.${coordinatorId}-`),
          ) ?? null;
        return observer?.state === "complete" ? list : null;
      },
      { timeoutMs: CHAIN_TIMEOUT_MS + OPS_TIMEOUT_MS },
    );
    expect(
      settled,
      "no completed ops-chain-observer for this run. Either the run never reached a terminal phase (agent-run rules 03/04) or the ops rule did not fire on it (configs/rules/ops/01-run-terminal-observe.json).",
    ).toBeTruthy();
    const observerId = (observer as LoopSummary | null)!.loop_id;

    // The run instance equals the coordinator's loop id on this runtime;
    // the drill-in href, the controls index and the run binding all rely
    // on it, so pin it here where a violation is easiest to read.
    const coordinator = settled!.find((l) => l.loop_id === coordinatorId);
    expect(
      coordinator?.role,
      `loop ${coordinatorId} named by POST /teams-dispatch/message is not a coordinator`,
    ).toBe("coordinator");
    expect(coordinator?.task_id).toMatch(/^dispatch-/);

    // -----------------------------------------------------------------
    // Step 2 — the board. Both repositories read, configured columns for
    // the one with a Project status field, fallback columns for the other.
    // -----------------------------------------------------------------
    await page.goto("/work");
    await waitForBoard(page);

    const semteamsBoard = page.getByTestId(`work-repository-${slug(SEMTEAMS)}`);
    const semsourceBoard = page.getByTestId(
      `work-repository-${slug(SEMSOURCE)}`,
    );
    for (const column of ["todo", "in-progress", "in-review", "done"]) {
      await expect(
        semteamsBoard.getByTestId(`work-column-${column}`),
        `${SEMTEAMS} has a Project status field; its configured column ${column} must render even if empty`,
      ).toBeVisible();
    }
    for (const column of ["todo", "in-progress", "done"]) {
      await expect(
        semsourceBoard.getByTestId(`work-column-${column}`),
      ).toBeVisible();
    }
    await expect(
      semsourceBoard.getByTestId("work-column-in-review"),
      `${SEMSOURCE} has no Project status field, so it gets the fallback columns only`,
    ).toHaveCount(0);

    await expect(page.getByTestId("run-facts-note").first()).toContainText(
      "Run facts are read when an item is opened",
    );

    // PM status, execution stage and verification are three separate
    // values on every card. Run-derived ones are unknown until an item's
    // runs are loaded — never "none", which would claim a completed lookup.
    const cards = page.getByTestId("work-item-card");
    const cardCount = await cards.count();
    expect(cardCount, "fixture items rendered as cards").toBeGreaterThanOrEqual(
      7,
    );
    for (let i = 0; i < cardCount; i++) {
      const c = cards.nth(i);
      const label = (await c.getAttribute("data-item")) ?? `card ${i}`;
      for (const name of ["pm-status", "execution-stage", "verification"]) {
        await expect(
          c.getByTestId(`overlay-${name}`),
          `${label} must carry its own overlay-${name} badge`,
        ).toHaveCount(1);
      }
      await expect(c.getByTestId("overlay-pm-status")).toHaveAttribute(
        "data-state",
        "known",
      );
      await expect(c.getByTestId("overlay-execution-stage")).toHaveAttribute(
        "data-state",
        "unknown",
      );
      await expect(c.getByTestId("overlay-verification")).toHaveAttribute(
        "data-state",
        "unknown",
      );
    }

    // The PM column is a GitHub field, not run state.
    await expect(card(page, PROMPT_BOUND_ITEM)).toHaveAttribute(
      "data-column",
      "In Progress",
    );
    await expect(
      semteamsBoard
        .getByTestId("work-column-in-progress")
        .locator(`[data-item="${PROMPT_BOUND_ITEM}"]`),
    ).toHaveCount(1);

    // -----------------------------------------------------------------
    // Step 3 — read-only is structural. No drag affordance anywhere in
    // the board, and the work API has no write verb.
    // -----------------------------------------------------------------
    await expect(
      page.locator('[data-testid="work-board"] [draggable="true"]'),
      "a board element is draggable; read-only must be structural, not a disabled handler",
    ).toHaveCount(0);

    const write = await request.post("/api/work/items", {
      data: { title: "should not be accepted" },
    });
    expect(
      write.status(),
      `POST /api/work/items must be 405 Method Not Allowed, got ${write.status()} ${await write.text()}`,
    ).toBe(405);

    // -----------------------------------------------------------------
    // Step 4 — #311 declares no linked runs: the one legal "none".
    // -----------------------------------------------------------------
    await openItem(page, NO_RUNS_ITEM);
    await loadLinkedRuns(page);
    await expect(page.getByTestId("work-linked-runs")).toHaveAttribute(
      "data-lookup",
      "complete",
    );
    await expect(page.getByTestId("no-linked-run")).toBeVisible();
    await expect(page.getByTestId("linked-run-row")).toHaveCount(0);
    await expect(
      card(page, NO_RUNS_ITEM).getByTestId("overlay-execution-stage"),
    ).toHaveAttribute("data-state", "none");
    await expect(
      card(page, NO_RUNS_ITEM).getByTestId("overlay-execution-stage"),
    ).toContainText("no linked run");

    // -----------------------------------------------------------------
    // Step 5 — #313 binds a run entity that cannot exist. That is a real
    // failed resolution: partial, with the reason naming the dead run, and
    // never "no linked run".
    // -----------------------------------------------------------------
    await openItem(page, DEAD_RUN_ITEM);
    await loadLinkedRuns(page);
    await expect(page.getByTestId("work-linked-runs")).toHaveAttribute(
      "data-lookup",
      "partial",
    );
    await expect(
      page.getByTestId("no-linked-run"),
      "a binding that resolved to nothing was reported as 'no linked run'; only an empty linked_runs list may say that",
    ).toHaveCount(0);
    await expect(page.getByTestId("work-linked-runs-lookup")).toContainText(
      DEAD_RUN_ID,
    );

    const deadRow = page.getByTestId("linked-run-row");
    await expect(deadRow).toHaveCount(1);
    await expect(deadRow).toHaveAttribute("data-run", DEAD_RUN_ID);
    await expect(
      deadRow.getByTestId("overlay-execution-stage"),
    ).toHaveAttribute("data-state", "unknown");
    await expect(
      deadRow.getByTestId("overlay-execution-stage-reason"),
    ).toContainText("has no facts");
    // No coordinator loop was resolved, so there is nothing to drill into.
    await expect(page.getByTestId("work-drill-in")).toHaveCount(0);
    await expect(page.getByTestId("work-drill-in-unavailable")).toBeVisible();
    await expect(
      card(page, DEAD_RUN_ITEM).getByTestId("overlay-execution-stage"),
    ).toHaveAttribute("data-state", "unknown");

    // -----------------------------------------------------------------
    // Step 6 — #312 is bound, by exact coordinator prompt, to the run this
    // spec created.
    // -----------------------------------------------------------------
    await openItem(page, PROMPT_BOUND_ITEM);
    await loadLinkedRuns(page);
    await expect(
      page.getByTestId("work-linked-runs"),
      "the prompt binding did not resolve to this spec's run. Check agent.loop.description on the coordinator equals the prompt exactly.",
    ).toHaveAttribute("data-lookup", "complete");

    const boundRow = page.getByTestId("linked-run-row");
    await expect(
      boundRow,
      "expected exactly one run for the prompt. More than one means the stack was reused from an earlier run: a prompt binding matches every coordinator with that prompt.",
    ).toHaveCount(1);
    await expect(boundRow).toHaveAttribute(
      "data-run",
      new RegExp(`\\.chain\\.agent\\.execution\\.${coordinatorId}$`),
    );
    const stage = boundRow.getByTestId("overlay-execution-stage");
    await expect(stage).toHaveAttribute("data-state", "known");
    await expect(stage).toHaveAttribute("data-value", "completed");
    await expect(stage).toContainText("completed");
    // Research runs carry no verification fact; it stays unknown with its reason.
    await expect(boundRow.getByTestId("overlay-verification")).toHaveAttribute(
      "data-state",
      "unknown",
    );

    // A linked run never moves a card: stage is run state, the column is not.
    await expect(
      card(page, PROMPT_BOUND_ITEM).getByTestId("overlay-execution-stage"),
    ).toHaveAttribute("data-value", "completed");
    await expect(card(page, PROMPT_BOUND_ITEM)).toHaveAttribute(
      "data-column",
      "In Progress",
    );
    await expect(
      card(page, PROMPT_BOUND_ITEM).getByTestId("overlay-pm-status"),
    ).toHaveAttribute("data-value", "In Progress");

    const drillIn = page.getByTestId("work-drill-in");
    await expect(drillIn).toHaveAttribute(
      "href",
      new RegExp(`\\?task=${coordinatorId}$`),
    );
    await drillIn.click();

    // -----------------------------------------------------------------
    // Step 7 — the runs lens. The ops observer is a control of the
    // coordinator task, not a card of its own, and its story row is the
    // explained event. Navigated by link (URL state) rather than by
    // clicking a card, as in chain-drill-in.
    // -----------------------------------------------------------------
    await expect(page).toHaveURL(new RegExp(`[?&]task=${coordinatorId}`));
    await expect(
      page.getByTestId("task-detail-panel"),
      "the runs lens did not open the coordinator task from the drill-in link",
    ).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId("panel-activity")).toBeVisible();

    await expect(
      page.locator(
        `[data-testid='task-card'][data-task-id='${coordinatorId}']`,
      ),
    ).toBeVisible({ timeout: 30_000 });

    // A rule-fired loop is classified from its task_id (rule-<run entity>-
    // <nanos>), but it only folds into its coordinator once a one-time graph
    // read confirms it is not a run member (no agent.loop.run on its loop
    // entity). That read runs on a 2.5 s tick with bounded retries, and until
    // it lands the observer is an ordinary top-level card. So wait for the
    // control row, and only then expect the observer's own card to be gone;
    // asserting its absence at any earlier instant would be a race.
    const controlRows = page.getByTestId("control-row");
    await expect(
      controlRows,
      "the coordinator task shows no control. The observer is classified from its task_id (rule-<run entity>-<nanos>) and folds in once a graph read confirms it is not a run member (no agent.loop.run on its loop entity); check controlsStore's membership read.",
    ).toHaveCount(1, { timeout: 30_000 });
    await expect(controlRows).toHaveAttribute("data-loop-id", observerId);
    await expect(
      page.locator(`[data-testid='task-card'][data-task-id='${observerId}']`),
      "the ops observer is still a top-level card after it was folded into its run as a control",
    ).toHaveCount(0, { timeout: 30_000 });

    // The focus is the coordinator (no breadcrumb), which is where the
    // story renders its controls.
    await expect(page.getByTestId("focus-breadcrumb")).toHaveCount(0);
    const events = page.getByTestId("control-event");
    await expect(events).toHaveCount(1, { timeout: 30_000 });
    const event = events.first();

    await expect(event.getByTestId("control-rule")).toContainText("unknown");
    await expect(event.getByTestId("control-rule")).toContainText("#298");
    // The fact is read from the graph after the row mounts; wait for it.
    await expect(event.getByTestId("control-fact")).toContainText(
      "agent.run.phase",
      { timeout: 30_000 },
    );
    await expect(event.getByTestId("control-fact")).toContainText("completed");
    await expect(event.getByTestId("control-identity")).toContainText(
      "ops-chain-observer",
    );
    await expect(event.getByTestId("control-identity")).toContainText(
      observerId,
    );
    // The observer finished (it was complete before /work was opened), so
    // the control was applied rather than merely accepted.
    await expect(event.getByTestId("control-outcome")).toContainText("applied");

    // -----------------------------------------------------------------
    // Step 8 — the table is the same items as the board, and an item's
    // loaded runs show in its row. Reached by URL (see the known-gap note
    // at the top of this file): the in-page toggle is exercised, as an
    // expected failure, by the tests below.
    // -----------------------------------------------------------------
    await page.goto("/work");
    await waitForBoard(page);
    const boardCards = await page.getByTestId("work-item-card").count();

    await page.goto("/work?view=table");
    await expect(page.getByTestId("work-table")).toBeVisible({
      timeout: 30_000,
    });
    await expect(page).toHaveURL(/[?&]view=table(&|$)/);
    await expect(page.getByTestId("work-board")).toHaveCount(0);
    await expect(
      page.getByTestId("work-item-row"),
      "table and board must list the same items",
    ).toHaveCount(boardCards);

    await page.goto(
      `/work?view=table&item=${encodeURIComponent(NO_RUNS_ITEM)}`,
    );
    await expect(page.getByTestId("work-item-overview")).toHaveAttribute(
      "data-item",
      NO_RUNS_ITEM,
      { timeout: 30_000 },
    );
    const noRunsRow = page
      .getByTestId("work-item-row")
      .and(page.locator(`[data-item="${NO_RUNS_ITEM}"]`));
    await expect(noRunsRow.getByTestId("overlay-linked-runs")).toHaveAttribute(
      "data-state",
      "unknown",
    );
    await loadLinkedRuns(page);
    await expect(noRunsRow.getByTestId("overlay-linked-runs")).toHaveAttribute(
      "data-state",
      "none",
    );
  });

  // -------------------------------------------------------------------
  // Known gap, asserted as an expected failure so it cannot be forgotten.
  //
  // In SvelteKit 2.46.4, `replaceState(url, state)` rewrites the address bar
  // and `page.state` but NOT `page.url`. The work lens (and the runs lens'
  // taskStore, on main) derive their selection from `page.url.searchParams`,
  // so an in-page click changes the address bar and nothing else: the Table
  // toggle leaves the board up, a card does not open its overview, a task
  // card does not open its panel. Deep links work, because `page.url` is
  // right on load. Measured 2026-10-08 on this stack in a real browser.
  // That is also why chain-drill-in navigates with page.goto: "replaceState
  // races" is really "replaceState never updates page.url".
  //
  // These tests state the behaviour an operator expects. They pass today
  // only because they are annotated to fail; when the URL-state fix lands
  // Playwright reports "expected to fail, but passed" and the annotation
  // (and this comment) must be deleted.
  // -------------------------------------------------------------------
  test("in-page Table toggle renders the table", async ({ page }) => {
    test.fail(
      true,
      "replaceState does not update page.url (SvelteKit 2.46.4), so the view derived from it never changes",
    );
    await page.goto("/work");
    await waitForBoard(page);
    await page.getByTestId("view-table").click();
    await expect(page).toHaveURL(/[?&]view=table(&|$)/);
    await expect(page.getByTestId("work-table")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("in-page card selection opens the item overview", async ({ page }) => {
    test.fail(
      true,
      "replaceState does not update page.url (SvelteKit 2.46.4), so the item derived from it never changes",
    );
    await page.goto("/work");
    await waitForBoard(page);
    await card(page, NO_RUNS_ITEM).click();
    await expect(page).toHaveURL(/[?&]item=/);
    await expect(page.getByTestId("work-item-overview")).toBeVisible({
      timeout: 5_000,
    });
  });
});
