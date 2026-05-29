# Semteams — research-pack parallel fan-out 2026-05-29

**Real-LLM verification of parallel-investigation flow for the research
pack on a streaming-protocol comparison prompt.**

Run timestamp: 2026-05-29T15:17 → 15:22 UTC. Wallclock to user-facing
reply: **2 minutes 8 seconds.** Outcome: **8/8 chain loops reached the
canonical respond_direct terminal, reviewer-approved.** Models: Gemini
3.1 Pro for coordinator (1 call), Gemini 2.5 Flash for the research /
synthesize / reviewer roles (12 calls across the chain). Total spend:
**~$0.30.**

## What this run verifies

The research pack previously walked sub-investigations sequentially —
one angle at a time. For a multi-faceted prompt like *"compare
post-pandemic recovery of US fast-casual restaurant chains across
Chipotle, Panera, Shake Shack, Sweetgreen, Cava, Wingstop"*, the chain
would spend N × T on the gather phase. This validation cycle wired the
new parallel pattern: the planner emits an explicit list of sub-topics,
the framework spawns one investigator per sub-topic concurrently, and a
join step waits until all complete before the aggregator runs.

The verification had four structural questions:

1. **Does the planner reliably emit an explicit decomposition?** —
   addressed by adding `subtopics` to the planner's terminal contract
   and updating its persona to treat each sub-topic as one parallel
   investigator's scope.
2. **Does the framework's parallel-spawn primitive (`for_each`) actually
   spawn N concurrent investigators against a real LLM, not just a mock
   one?** — addressed by smoke runs against real Gemini Flash.
3. **Does the join machinery (counter-on-parent + length-equality match)
   fire exactly once when all N have stamped completion?** — addressed
   across smoke runs 1-6, with three upstream issues filed for engine
   semantics that didn't survive real-LLM stamping cadence.
4. **Does the aggregator (synthesize) actually consume N parallel
   investigators' findings — not just one — and produce a single
   reviewer-readable artifact?** — addressed by adopting the framework's
   new `.triples` substitution (closed in beta.86 a few days before this
   run) to inline the sibling investigator IDs directly into the
   aggregator's spawn prompt.

N=1 (prompts that don't usefully decompose) is a first-class case: the
same machinery degenerates to a single investigator with no overhead.
We deliberately rejected adding a separate "deep research" classifier
because it would have introduced a boundary the model would game on
every edge prompt.

## The scenario

Prompt handed to the coordinator, verbatim:

> Compare MQTT vs NATS for IoT edge deployments — which has lower
> latency on constrained ARM devices?

Sufficiently decomposable to exercise the parallel path (planner
emitted N=3 sub-topics: MQTT protocol analysis, NATS protocol analysis,
performance benchmarking considerations). Real evidence-gathering
required — the investigators used web search and direct URL fetching
against real sources.

## What happened (run-6 final shape)

```
15:17:21  coordinator         → decide(research)               [routes to research pack]
15:17:23  plan                → decide(gather, subtopics=[3])
15:17:40  gather × 3 ─────────→ decide(synthesize)             [running in parallel; ~30s span]
          ├─ MQTT protocol analysis
          ├─ NATS protocol analysis
          └─ Performance benchmarking considerations
15:18:47  synthesize          → decide(emit)                    [aggregated artifact]
15:19:14  reviewer            → decide(approved)
15:19:29  coordinator wake-up → decide(respond_direct)          [user-facing reply]
```

8 loops total. The three parallel investigators finished within ~30
seconds of each other; sequential would have taken ~90 seconds at the
gather phase alone, so the parallel wallclock saving is real and
measurable. Per-loop iteration counts ranged 2-5, with the slowest
gatherer making 5 web-search calls before terminating.

The aggregator received the three investigators' findings as a
JSON-encoded array of loop identifiers inlined directly into its spawn
prompt (via the framework's `.triples` substitution). It parsed the
array, called `read_loop_result` once per sibling, and produced one
artifact with deduplicated actors, merged integration points, and
sub-topic-indexed task breakdowns. The reviewer evaluated it on substance
(real source citations, real protocol details, real benchmarking
considerations) and approved without recovery.

## The validation arc (smoke runs 1-6)

Six smoke runs over ~2 hours of working with the framework team. Each
surfaced something specific. Three of the findings were genuine
engine-level gaps that we filed upstream rather than working around in
our code — pattern was to file + wait or file + apply a documented
unblock, not to bolt application-side state machinery on top of the
framework.

See `evidence/timeline.md` for the compressed per-run table. Key
findings:

- **Three upstream issues filed.** [semstreams#158](https://github.com/C360Studio/semstreams/issues/158)
  (text-only LLM completions strand work), [semstreams#159](https://github.com/C360Studio/semstreams/issues/159)
  (completion-state stamp race), [semstreams#160](https://github.com/C360Studio/semstreams/issues/160)
  (template-variable substitution prefix collision).
- **One workaround temporarily in our code.** A 2-line spawn-time
  lineage triple sidesteps the #159 timing race; retires the day the
  upstream fix lands.
- **One documented-pattern adoption.** The framework's `tool_choice=required`
  guard (shipped in their beta.80 closure pass for the same failure
  class) was applied to every flash-model spawn rule in our pack to
  prevent the model from terminating with prose instead of a structured
  tool call.

## What this unlocks

**Future task packs follow the same shape.** The framework's primitive
set (parallel fan-out spawn → cross-entity counter writes → length
comparison with dynamic substitution → array-as-inlined-data for the
consumer) is now coherent. Adding a new flow (audit, monitoring,
compliance) starts from the substrate, not from rediscovering engine
semantics.

**Parallel-by-default is the new normal.** Decomposition lives in the
planner where the information already is; the same machinery handles
N=1 prompts (no overhead) and N=6 prompts (real wallclock saving) with
no class boundary to defend.

**Pattern for working with the framework team.** Real-LLM validation
surfacing engine gaps → file at the right layer → ship workaround app-
side only when explicitly time-bounded → workaround retires when the
upstream fix lands. Three filings, two already closed at the framework
layer, one in design discussion. Our application-side surface stays
intentionally thin.

## What's next

- **[semstreams#159](https://github.com/C360Studio/semstreams/issues/159)
  in design discussion upstream.** When the framework lands the fix
  (atomic completion-state batch or spawn-time parent stamping), we
  delete the workaround triple and our rules return to the canonical
  `agent.loop.parent` reference. Net code reduction.
- **Consolidation PR** to land the validated fixes on main. Currently
  sitting on a feature branch.
- **Next pack target: TBD.** When the next category lands, the substrate
  (parallel spawn, counter writes, length-eq join, .triples inlining,
  tool_choice on flash spawns) is reusable. Cost of adding a pack should
  be closer to "write the prompts + rule wiring" than "rediscover engine
  semantics."

## Files

- **`README.md`** — this document.
- **`evidence/timeline.md`** — compressed per-run table for the six
  smoke runs, with the specific finding from each.
- **`evidence/chain-shape.txt`** — verbatim chain trace from run 6
  (the canonical happy-path).
- **`evidence/upstream-issues.md`** — one-paragraph summary of each
  upstream issue filed during the cycle.
