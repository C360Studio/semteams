// Agent API client
// Handles communication with teams-dispatch and teams-loop backend services

import { userIdentity } from "$lib/stores/userIdentity.svelte";
import type {
  AgentLoop,
  ApprovalAcceptResponse,
  ApprovalRequest,
  ControlSignal,
  LoopTrajectory,
  SignalResponse,
} from "$lib/types/agent";

const DISPATCH_BASE = "/teams-dispatch";
const GRAPHQL_ENDPOINT = "/graphql";

export class AgentApiError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "AgentApiError";
  }
}

// ---------------------------------------------------------------------------
// Trajectory — GraphQL `trajectory(loopId, limit, cursor)` on graph-gateway
// (semstreams beta.160). Replaces the deleted REST
// /teams-loop/trajectories[/{loopId}] endpoints. Mirrors graphApi.ts's
// executeQuery pattern (POST /graphql, unwrap data/errors) but throws
// AgentApiError so existing consumers' catch blocks keep working unchanged.
//
// The gateway does not perform GraphQL field-selection filtering — it
// forwards the backend's full JSON response verbatim under `data.trajectory`
// (see component.go's writeGraphQLSuccessWithExtensions). The query below
// still lists every field explicitly: it documents the real wire contract
// and keeps this client correct if the gateway starts honouring selection
// sets later.
// ---------------------------------------------------------------------------

const TRAJECTORY_QUERY = `
  query LoopTrajectory($loopId: String!, $limit: Int, $cursor: String) {
    trajectory(loopId: $loopId, limit: $limit, cursor: $cursor) {
      schema_version
      loop_id
      coverage
      terminal_observed
      observed_totals {
        facts
        tokens_in
        tokens_out
        elapsed_ms
        message_count
        tool_count
        url_count
        model_requests
        model_completions
        tool_requests
        tool_completions
        context_compactions
        terminal_observations
        requested_observations
        completed_observations
        failed_observations
        cancelled_observations
      }
      facts {
        schema_version
        loop_digest
        attempt_id
        attempt_ordinal
        kind
        source_kind
        source_correlation
        causal_iteration
        causal_phase
        causal_ordinal
        observed_at
        elapsed_ms
        status
        tokens_in
        tokens_out
        message_count
        tool_count
        url_count
        model_preview
        provider_preview
        tool_preview
        capability_preview
        error_category
        evidence_digest
        evidence_size
        evidence {
          storage_instance
          key
          content_type
          size
        }
        evidence_capture
        evidence_failure
      }
      next_cursor
    }
  }
`;

// One page's worth of facts per request. Comfortably under the backend's
// maxTrajectoryQueryLimit (256) while keeping individual GraphQL payloads
// small.
const TRAJECTORY_PAGE_LIMIT = 100;

// Hard stop for a single getTrajectory()/getLoopTrajectory() call — a very
// deeply-iterated loop should degrade (partial trajectory + next_cursor left
// populated) rather than page forever.
const TRAJECTORY_FACT_CAP = 500;

// The backend classifies "no facts recorded yet" (e.g. a loop that hasn't
// reached its first observation) as an invalid-request error, not an empty
// page (handleTrajectoryQueryWithMaxPayload wraps errTrajectoryNotFound as
// "trajectory not found: trajectory not found"). Treating that as a thrown
// AgentApiError would regress the common "task just started" case — the old
// REST endpoint returned 200 with an empty step list. Recognise the marker
// and synthesize an empty page instead so callers see "no steps yet", not an
// error banner.
const TRAJECTORY_NOT_FOUND_MARKER = "trajectory not found";

function emptyTrajectoryPage(loopId: string): LoopTrajectory {
  return {
    schema_version: "v1",
    loop_id: loopId,
    coverage: "observed",
    terminal_observed: false,
    observed_totals: {
      facts: 0,
      tokens_in: 0,
      tokens_out: 0,
      elapsed_ms: 0,
      message_count: 0,
      tool_count: 0,
      url_count: 0,
      model_requests: 0,
      model_completions: 0,
      tool_requests: 0,
      tool_completions: 0,
      context_compactions: 0,
      terminal_observations: 0,
      requested_observations: 0,
      completed_observations: 0,
      failed_observations: 0,
      cancelled_observations: 0,
    },
    facts: [],
  };
}

interface TrajectoryGraphQLResponse {
  data?: { trajectory: LoopTrajectory | null };
  errors?: Array<{ message: string; path?: string[] }>;
}

async function fetchTrajectoryPage(
  loopId: string,
  cursor?: string,
): Promise<LoopTrajectory> {
  const variables: Record<string, unknown> = {
    loopId,
    limit: TRAJECTORY_PAGE_LIMIT,
  };
  if (cursor) variables.cursor = cursor;

  let response: Response;
  try {
    response = await fetch(GRAPHQL_ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query: TRAJECTORY_QUERY, variables }),
    });
  } catch (error) {
    throw new AgentApiError(
      `Network error during getTrajectory: ${error instanceof Error ? error.message : "Unknown error"}`,
      0,
      { originalError: error },
    );
  }

  if (!response.ok) {
    throw new AgentApiError(
      `Failed to get trajectory: ${response.statusText}`,
      response.status,
    );
  }

  let parsed: TrajectoryGraphQLResponse;
  try {
    parsed = await response.json();
  } catch (error) {
    throw new AgentApiError("Invalid JSON response from getTrajectory", 500, {
      originalError: error,
    });
  }

  if (parsed.errors && parsed.errors.length > 0) {
    const message = parsed.errors[0].message;
    if (message.toLowerCase().includes(TRAJECTORY_NOT_FOUND_MARKER)) {
      return emptyTrajectoryPage(loopId);
    }
    throw new AgentApiError(message, 200, { errors: parsed.errors });
  }

  if (!parsed.data?.trajectory) {
    throw new AgentApiError("No trajectory data in response", 500);
  }

  return parsed.data.trajectory;
}

function sumObservedTotals(
  a: LoopTrajectory["observed_totals"],
  b: LoopTrajectory["observed_totals"],
): LoopTrajectory["observed_totals"] {
  return {
    facts: a.facts + b.facts,
    tokens_in: a.tokens_in + b.tokens_in,
    tokens_out: a.tokens_out + b.tokens_out,
    elapsed_ms: a.elapsed_ms + b.elapsed_ms,
    message_count: a.message_count + b.message_count,
    tool_count: a.tool_count + b.tool_count,
    url_count: a.url_count + b.url_count,
    model_requests: a.model_requests + b.model_requests,
    model_completions: a.model_completions + b.model_completions,
    tool_requests: a.tool_requests + b.tool_requests,
    tool_completions: a.tool_completions + b.tool_completions,
    context_compactions: a.context_compactions + b.context_compactions,
    terminal_observations: a.terminal_observations + b.terminal_observations,
    requested_observations:
      a.requested_observations + b.requested_observations,
    completed_observations:
      a.completed_observations + b.completed_observations,
    failed_observations: a.failed_observations + b.failed_observations,
    cancelled_observations:
      a.cancelled_observations + b.cancelled_observations,
  };
}

/**
 * Fetch a loop's full observed trajectory, walking `next_cursor` pages until
 * the backend signals exhaustion (no next_cursor), a cursor repeats (a
 * defensive cycle guard — a misbehaving/looping cursor must not spin the UI
 * forever), or the merged fact count reaches TRAJECTORY_FACT_CAP —
 * whichever comes first. `observed_totals` is page-scoped per the wire
 * contract (it "summarizes only the facts returned in one page" — see
 * agentic.TrajectoryObservedTotals upstream), so pages are summed
 * field-by-field to describe the merged fact set. `terminal_observed` is
 * OR'd across pages for the same reason.
 */
async function fetchFullTrajectory(loopId: string): Promise<LoopTrajectory> {
  const seenCursors = new Set<string>();
  let cursor: string | undefined;
  let merged: LoopTrajectory | null = null;

  for (;;) {
    const page = await fetchTrajectoryPage(loopId, cursor);
    if (!merged) {
      merged = { ...page, facts: [...page.facts] };
    } else {
      merged.facts.push(...page.facts);
      merged.terminal_observed = merged.terminal_observed || page.terminal_observed;
      merged.observed_totals = sumObservedTotals(
        merged.observed_totals,
        page.observed_totals,
      );
    }

    const next = page.next_cursor;
    merged.next_cursor = next || undefined;
    if (!next || seenCursors.has(next) || merged.facts.length >= TRAJECTORY_FACT_CAP) {
      break;
    }
    seenCursors.add(next);
    cursor = next;
  }

  return merged;
}

export const agentApi = {
  /**
   * Send a message to the dispatch. Supports optional run-level reply
   * anchors for the ADR-053 4b-2 clarification resume path:
   *   - opts.runId      → run_id in the body (re-attaches to the paused run)
   *   - opts.inReplyTo  → in_reply_to in the body (marks reply to the asking loop)
   * Omitting opts (or leaving fields undefined) produces the same body as
   * the original single-arg signature so existing callers are unaffected.
   */
  async sendMessage(
    content: string,
    opts?: { runId?: string; inReplyTo?: string },
  ): Promise<{ content: string }> {
    const body: Record<string, string> = { content };
    if (opts?.runId) body.run_id = opts.runId;
    if (opts?.inReplyTo) body.in_reply_to = opts.inReplyTo;
    const response = await fetch(`${DISPATCH_BASE}/message`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to send message: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  async listLoops(): Promise<AgentLoop[]> {
    const response = await fetch(`${DISPATCH_BASE}/loops`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to list loops: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  async getLoop(id: string): Promise<AgentLoop> {
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to get loop: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  async sendSignal(
    id: string,
    type: ControlSignal,
    reason?: string,
  ): Promise<SignalResponse> {
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}/signal`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type, ...(reason ? { reason } : {}) }),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to send signal: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  /**
   * Submit a human approval response for a gated tool call. Drives
   * the semstreams beta.19 approval flow: the agent loop is paused
   * waiting for this call. The backend publishes an ApprovalResponse
   * on `agent.approval_response.<loop_id>` and the loop resumes.
   *
   * Sets the X-User-Id header from the userIdentity store so the
   * product middleware (cmd/semteams/middleware.go) lifts the value
   * into agentic-dispatch's identity ctx before resolution. The
   * backend's resolution order — ctx > body.user_id > "http-user" —
   * means a deployment without the middleware still resolves the
   * caller via the body fallback.
   *
   * Throws AgentApiError on non-2xx responses. Notable status codes:
   *   - 400: malformed body or unknown decision value
   *   - 404: loop unknown to dispatch (process restart, etc.)
   *   - 409: loop tracked but not awaiting approval (already resolved
   *          or never gated). Caller should refresh state.
   *   - 500: NATS publish failure; safe to retry.
   */
  async submitApproval(
    id: string,
    request: ApprovalRequest,
  ): Promise<ApprovalAcceptResponse> {
    const identity = userIdentity.value;
    // Treat an explicitly-empty user_id the same as absent: fall back
    // to the store value. Upstream IdentityFromRequest treats empty
    // body fields as "no claim" too, so an empty body would resolve
    // via ctx-or-default — but sending the empty string makes the
    // body and header disagree, which is noise.
    const callerUserID = request.user_id?.trim();
    const body: ApprovalRequest = {
      ...request,
      // Body fallback so the request resolves even if the middleware
      // is not deployed. Middleware-injected ctx still wins per
      // upstream IdentityFromRequest precedence.
      user_id: callerUserID && callerUserID !== "" ? callerUserID : identity,
    };
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}/approval`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-User-Id": identity,
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to submit approval: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  /**
   * Fetch a loop's full trajectory — the immutable, privacy-bounded fact
   * log (facts[]) that powers the story view and evidence panel. Paginates
   * through next_cursor internally (see fetchFullTrajectory).
   *
   * getTrajectory and getLoopTrajectory are the same call under two names:
   * both predate the beta.160 migration off REST
   * /teams-loop/trajectories[/{loopId}], and each still has a live call
   * site (TrajectoryViewer.svelte and TaskStory.svelte/RunEvidencePanel.svelte
   * respectively), so both stay exported rather than picking a single name
   * and touching every caller.
   */
  async getTrajectory(loopId: string): Promise<LoopTrajectory> {
    return fetchFullTrajectory(loopId);
  },

  async getLoopTrajectory(loopId: string): Promise<LoopTrajectory> {
    return fetchFullTrajectory(loopId);
  },
};
