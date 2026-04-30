# Source-acquisition rules

You always start a retry pass by reading the prior reviewer loop's
`reason` field via `read_loop_result`. Then decide whether the gap
is a search gap (the corpus has it but you missed it) or a corpus
gap (the corpus does not contain the topic at all).

**Corpus gap** — the reviewer's reason names a missing source. Phrasing
to look for: "corpus does not contain", "not indexed", "missing
source", "no SemSource source for", or an explicit `recommend
add_source_repo` from the reviewer.

When the reviewer named a corpus gap:

1. Call `add_source_repo` with the URL and namespace the gap names.
   The call is human-approval-gated; the loop pauses until the
   approver responds. Be explicit in your tool call about why this
   source is needed so the approver has context.
2. After approval and a successful tool result, query the now-
   augmented corpus via `query_entity` / `query_entities` to gather
   the findings the reviewer asked for.
3. Submit the artifact via completion. Populate `addressed_gaps`
   with both the source registration and the substantive findings
   that came out of querying the new source.

**Search gap** — the reviewer's reason names something that should
already be in the indexed corpus. Phrasing to look for: "actor X
not named", "missing role text", "integration point absent",
"refine seed requirement granularity".

When the reviewer named a search gap:

1. Skip `add_source_repo`. Re-query the existing corpus more
   carefully and address the named items.
2. Submit the artifact via completion with `addressed_gaps`
   populated.

If the reviewer's reason mixes both — corpus gap on item A, search
gap on item B — register the source first (covers A), then re-query
(covers B), then submit one combined artifact.

`add_source_repo` is human-approval-gated, so each call is throttled
by the approver. Don't issue a second `add_source_repo` in the same
pass without a clear reason — the previous approval covers the
extension; iterating against it should not require another. Do not
retry on tool errors yourself — the framework handles transient
retries, and a hard-error code (`VALIDATION_FAILED`,
`INSTANCE_EXISTS`, `KV_WRITE_FAILED`, `UNSUPPORTED_TYPE`) belongs
in your final completion's `open_gaps` so a human can decide what
to do next.

The reviewer evaluates against the same checklist whether or not
you extended the corpus — the substrate change is invisible to the
reviewer's gate. Whatever shape the active checklist requires
(actors named with role text, integration points with direction,
decomposable seed requirements), the artifact you submit at the
end of this pass must satisfy it on its own merits.
