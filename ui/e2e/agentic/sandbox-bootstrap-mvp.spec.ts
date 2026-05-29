import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-042 §addendum 2026-05-29 PR 2 — coordinator →
 * sandbox-bootstrap-category arc → coordinator return path,
 * end-to-end on mock-LLM against configs/e2e-flow-bootstrap.json.
 *
 * Validates the sandbox-bootstrap pack (rules + personas landed in
 * 454f117 + 75a9ee2) against the §A foundation PR's product-shell
 * tools (query_sandbox_tenant + emit_bootstrap_{plan,verify,
 * committed}, sandboxfleet.TenantRegistry).
 *
 * Loop sequence:
 *
 *   Loop 0: coordinator (dispatch) → decide(bootstrap_sandbox)
 *           → rule 01 fires → spawn provisioner-bootstrap-plan
 *
 *   Loop A: provisioner-bootstrap-plan → read_loop_result →
 *           query_sandbox_tenant → emit_bootstrap_plan →
 *           decide(execute)
 *           → rule 02b fires → spawn provisioner-bootstrap-execute
 *
 *   Loop B: provisioner-bootstrap-execute → read_loop_result →
 *           bash (docker run + clone + install placeholder) →
 *           decide(verify)
 *           → rule 03 fires → spawn provisioner-bootstrap-verify
 *
 *   Loop C: provisioner-bootstrap-verify → read_loop_result →
 *           bash (docker exec smoke placeholder) →
 *           emit_bootstrap_verify(ok) → decide(emit)
 *           → rule 04 fires → spawn reviewer-bootstrap
 *
 *   Loop D: reviewer-bootstrap → read_loop_result →
 *           bash (docker inspect + readiness-probe placeholder) →
 *           emit_bootstrap_committed → decide(approved)
 *           → rule 07 fires (CHAINED ALLOWLIST) → spawn coordinator
 *           (wake-up)
 *
 *   Loop E: coordinator (wake-up) → read_loop_result →
 *           decide(respond_direct) → rule 03b fires → publish on
 *           user.response.* + stamp coordinator.user_reply triple.
 *           Terminal — no further rules fire.
 *
 * Validates:
 *   - All SIX loops reach terminal complete state. Recovery rules
 *     05 (reviewer-rejected-retry) and 06 (needs-clarification-
 *     replan) are loaded but do NOT fire (reviewer approves).
 *   - Role distribution proves every rule fired in order:
 *       2 × coordinator                       (Loop 0 + Loop E)
 *       1 × provisioner-bootstrap-plan        (Loop A — rule 01)
 *       1 × provisioner-bootstrap-execute     (Loop B — rule 02b)
 *       1 × provisioner-bootstrap-verify      (Loop C — rule 03)
 *       1 × reviewer-bootstrap                (Loop D — rule 04)
 *   - Wire-shape: Loop 0 is dispatch-spawned (default_role=coordinator
 *     resolves; LoopInfo.role empty on the wire per
 *     [[feedback_loopinfo_role_omitempty]]); every other loop carries
 *     an explicit role from its publish_agent spawn.
 *   - No approval gate on the bootstrap arc.
 *   - No seventh loop appears (settle assertion against rule 07
 *     re-firing on the wake-up coordinator's terminal, or rule 05
 *     mis-firing on reviewer-bootstrap approved).
 *   - The chained-allowlist wake-up (rule 07) decides respond_direct
 *     under mock-LLM. The autoresearch / research downstream branches
 *     are reachable per the allowlist but the fixture takes the
 *     deliver-to-user branch for the structural test.
 *
 * Registry state validation is OUT OF SCOPE for PR 2 — the mock-LLM
 * personas don't compute the canonical signature themselves and
 * cross-loop signature threading requires real-LLM reading of
 * intermediate triples via read_loop_result. The §A foundation's
 * own unit tests + the §A/§B real-LLM smoke (PR 3) validate
 * registry transitions empirically.
 *
 * Required fixture: test/fixtures/journeys/sandbox-bootstrap-mvp.yaml
 * Required config: configs/e2e-flow-bootstrap.json (sandbox-bootstrap
 *   rule pack wired in PR 2).
 * Compose profile: none beyond the agentic-e2e baseline.
 */

test.describe(
  "ADR-042 §addendum PR 2 — Sandbox-bootstrap category mock-LLM journey",
  () => {
    test.beforeAll(async ({ request }) => {
      const health = await request.get("/health");
      expect(health.ok(), "Backend not healthy — stack not running?").toBe(
        true,
      );
    });

    // Six loops, autonomous chain (no approval gate, no recovery).
    // 3 minutes covers mock-LLM scheduling jitter + bash placeholder
    // exec overhead on the always-warm sandbox container.
    test.setTimeout(180_000);

    test(
      "user prompt → full sandbox-bootstrap arc → coordinator delivers reply",
      async ({ page, request }) => {
        // -------------------------------------------------------------
        // Step 1 — open Board so the SSE activity stream is connected
        // before any loops appear.
        // -------------------------------------------------------------
        await page.goto("/");

        await expect(page.getByTestId("connection-status")).toHaveAttribute(
          "data-summary",
          "healthy",
          { timeout: 15000 },
        );
        await expect(page.getByTestId("kanban-board")).toBeVisible();

        // -------------------------------------------------------------
        // Step 2 — send a bootstrap-sandbox-worthy prompt. The
        // coordinator's decide will classify it as
        // action="bootstrap_sandbox" and the rule chain takes over.
        // The prompt names the canonical inputs the fixture's
        // emit_bootstrap_plan call carries — so the signature
        // emit_bootstrap_plan computes server-side matches what the
        // reviewer's emit_bootstrap_committed echoes back.
        // -------------------------------------------------------------
        await page.getByTestId("chat-input").fill(
          "Set up a sandbox tenant for running `task test:integration` on the semteams repo at main, with Go 1.26 and Docker socket access for testcontainers.",
        );
        await page.getByTestId("send-button").click();

        // -------------------------------------------------------------
        // Step 3 — wait for all SIX loops to reach terminal complete
        // state. Rules involved:
        //   01  (coordinator → provisioner-bootstrap-plan)
        //   02b (plan → execute)
        //   03  (execute → verify)
        //   04  (verify → reviewer-bootstrap)
        //   07  (reviewer-bootstrap approved → wake-up coordinator,
        //        chained allowlist)
        //   03b (wake-up coordinator respond_direct → publish prose;
        //        no spawn)
        // Recovery rules 05 + 06 are loaded but do not fire.
        // -------------------------------------------------------------
        const allTerminal = await pollUntil(
          async () => {
            const resp = await request.get("/teams-dispatch/loops");
            if (!resp.ok()) return null;
            const list = (await resp.json()) as Array<{ state: string }>;
            if (list.length !== 6) return null;
            return list.every((l) => l.state === "complete") ? list : null;
          },
          { timeoutMs: 120_000 },
        );

        expect(
          allTerminal,
          "expected all 6 loops to reach terminal complete state (coordinator dispatch + bootstrap arc ×4 + wake-up coordinator)",
        ).toBeTruthy();

        // -------------------------------------------------------------
        // Step 4 — role distribution proves every rule fired in order.
        //
        // Wire-shape note: dispatch-spawned loops ride on
        // dispatch.default_role and don't get role stamped back onto
        // the LoopInfo wire JSON; rule-spawned loops do. So Loop 0
        // (dispatch coordinator) has empty role on the wire, and the
        // wake-up coordinator (Loop E — rule 07 publish_agent)
        // carries role="coordinator" explicitly.
        //
        // Expected role distribution (after default_role resolution
        // for the empty-role dispatch loop):
        //   2 × coordinator                       (Loop 0 dispatch + Loop E rule 07)
        //   1 × provisioner-bootstrap-plan        (Loop A — rule 01)
        //   1 × provisioner-bootstrap-execute     (Loop B — rule 02b)
        //   1 × provisioner-bootstrap-verify      (Loop C — rule 03)
        //   1 × reviewer-bootstrap                (Loop D — rule 04)
        // -------------------------------------------------------------
        const finalLoops = (await request
          .get("/teams-dispatch/loops")
          .then((r) => r.json())) as Array<{
          role: string;
          state: string;
          pending_approval?: { tool_name?: string } | null;
        }>;

        const dispatchLoops = finalLoops.filter((l) => !l.role);
        expect(
          dispatchLoops.length,
          `expected exactly 1 dispatch-spawned loop with empty role (Loop 0); got ${dispatchLoops.length}`,
        ).toBe(1);

        const expectedDefaultRole = "coordinator";
        const roles = finalLoops.map((l) => l.role || expectedDefaultRole);
        const coordinatorCount = roles.filter(
          (r) => r === "coordinator",
        ).length;
        const planCount = roles.filter(
          (r) => r === "provisioner-bootstrap-plan",
        ).length;
        const executeCount = roles.filter(
          (r) => r === "provisioner-bootstrap-execute",
        ).length;
        const verifyCount = roles.filter(
          (r) => r === "provisioner-bootstrap-verify",
        ).length;
        const reviewerCount = roles.filter(
          (r) => r === "reviewer-bootstrap",
        ).length;

        expect(
          coordinatorCount,
          `expected 2 coordinator loops (Loop 0 dispatch + Loop E rule 07 wake-up), got roles=${JSON.stringify(roles)}. Missing → rule 07 (sandbox-bootstrap/07-reviewer-approved-to-coordinator) did not fire on reviewer-bootstrap(approved).`,
        ).toBe(2);
        expect(
          planCount,
          `expected 1 provisioner-bootstrap-plan loop (Loop A via rule 01), got roles=${JSON.stringify(roles)}. Missing → rule 01 (sandbox-bootstrap/01-coordinator-bootstrap-spawn) did not fire on coordinator(bootstrap_sandbox).`,
        ).toBe(1);
        expect(
          executeCount,
          `expected 1 provisioner-bootstrap-execute loop (Loop B via rule 02b), got roles=${JSON.stringify(roles)}. Missing → rule 02b (sandbox-bootstrap/02b-plan-to-execute) did not fire on plan(execute).`,
        ).toBe(1);
        expect(
          verifyCount,
          `expected 1 provisioner-bootstrap-verify loop (Loop C via rule 03), got roles=${JSON.stringify(roles)}. Missing → rule 03 (sandbox-bootstrap/03-execute-to-verify) did not fire on execute(verify).`,
        ).toBe(1);
        expect(
          reviewerCount,
          `expected 1 reviewer-bootstrap loop (Loop D via rule 04), got roles=${JSON.stringify(roles)}. Missing → rule 04 (sandbox-bootstrap/04-verify-to-reviewer) did not fire on verify(emit).`,
        ).toBe(1);

        // -------------------------------------------------------------
        // Step 5 — no approval gate. The bootstrap arc is fully
        // autonomous: chain reaches terminal → coordinator wakes →
        // delivers reply. No human-in-the-loop step.
        // -------------------------------------------------------------
        const stillPendingAny = finalLoops.find(
          (l) => l.pending_approval != null,
        );
        expect(
          stillPendingAny,
          "no loop should ever require approval — sandbox-bootstrap arc is autonomous under the §addendum roster",
        ).toBeUndefined();

        // -------------------------------------------------------------
        // Step 6 — settle assertion: no seventh loop appears. Three
        // regressions this catches:
        //   (a) rule 07 accidentally re-firing on its own wake-up
        //       coordinator's terminal — would loop infinitely;
        //   (b) rule 05 mis-firing on reviewer-bootstrap approved
        //       (rather than insufficient) — would spawn a recovery
        //       plan loop;
        //   (c) the chained allowlist's `autoresearch` / `research`
        //       entries mis-routing the wake-up to a downstream arc
        //       under mock-LLM (the fixture's wake-up decides
        //       respond_direct, so this surface should stay closed).
        // -------------------------------------------------------------
        await new Promise((r) => setTimeout(r, 2000));
        const settledList = (await request
          .get("/teams-dispatch/loops")
          .then((r) => r.json())) as unknown[];
        expect(
          settledList.length,
          "no seventh loop should appear — rule 07 must not re-fire, rule 05 must not fire on approved, chained allowlist must not surface a downstream-arc spawn under the deliver-to-user wake-up branch",
        ).toBe(6);

        // -------------------------------------------------------------
        // Step 7 — user-response publish proof. Loop-count assertions
        // alone would silently pass a regression where rule 03b fails
        // to fire — the chain still terminates with 6 loops, but the
        // user never sees a reply. message-logger captures the
        // canonical user.response.* publish; assert at least one
        // entry matches.
        // -------------------------------------------------------------
        const respResp = await request.get(
          "/message-logger/entries?limit=500&subject=user.response.*",
        );
        expect(
          respResp.ok(),
          "/message-logger/entries returned non-OK",
        ).toBe(true);
        const respPayloads = (await respResp.json()) as Array<{
          subject: string;
        }>;
        expect(
          respPayloads.length,
          "expected at least one user.response.* publish (rule 03b → dispatch.user.response). Zero entries → either rule 03b did not fire, or dispatch's user.response output port is misconfigured.",
        ).toBeGreaterThanOrEqual(1);
      },
    );
  },
);

/**
 * pollUntil — race a polling fn against a deadline.
 * Returns the fn's value on success, or null on deadline.
 */
async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 250;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
