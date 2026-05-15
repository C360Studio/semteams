# Stabilisation gate

Before applying the OSH driver checklist (or any other prompt-
specific checklist), apply the **stabilisation gate**. A research
arc is stabilised when the reviewer's checklist passes AND the
latest revision had no new substrate mutations — re-iterating
against an augmented corpus confirms that conclusions hold under
the new evidence.

The substrate-mutations check is your responsibility, not a
rule's. Rules don't synthesise structured judgments from
unstructured content — that's your role. Concretely:

1. After `read_loop_result`, locate the artifact's `revision`
   field and its `substrate_mutations` array.
2. Count entries whose `revision` matches the artifact's
   current `revision` (i.e., mutations that landed in *this*
   pass).
3. **If the count is non-zero**, the substrate just changed.
   Even if every checklist item is satisfied, the artifact
   has not stabilised — the researcher must re-iterate against
   the augmented corpus to confirm the same conclusions hold.
   Call:

   ```
   decide(action="insufficient",
          reason="awaiting stabilisation; substrate mutated
                  this revision (added <N> source(s)), re-iterate
                  against augmented corpus before approval")
   ```

4. **If the count is zero**, proceed to the standard
   prompt-specific checklist. Approve only if all checklist
   items are present and well-formed.

Why this lives here and not in a rule: the rule engine cannot
read the artifact's `substrate_mutations` array. Rules match on
metadata triples and predicate substitution is string-only — they
cannot count entries in a nested array. The reviewer (you) reads
the artifact, applies the predicate, and emits the structured
terminal decision (`decide`); the stabilisation rule trusts that
decision.

The stabilisation gate fires before the prompt-specific
checklist. A retry with new mutations is `insufficient` regardless
of what else the artifact covers.
