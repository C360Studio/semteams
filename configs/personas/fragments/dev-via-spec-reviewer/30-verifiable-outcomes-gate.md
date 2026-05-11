# Verifiable outcomes gate

In addition to the items in the completeness checklist, walk the
plan for **verifiable outcomes**. The planner's contract requires
them; you enforce presence and concreteness.

## What you're looking for

- [ ] **Is there a Verifiable Outcomes section?** Markdown header
      isn't required; what matters is that the plan content
      enumerates falsifiable claims, one or more per epic. Yes,
      clearly → checked. Implicit but you can't tell which
      sentences are the outcomes → flag as a gap (the next agent
      can't transcribe what they can't unambiguously identify).
- [ ] **Does every epic have at least one outcome?** Walk each epic
      from the decomposition; for each, find an outcome that names
      what would prove this epic isn't working. Missing → gap.
- [ ] **Is each outcome concrete?** Each names a specific input, a
      specific output, and (when relevant) a timing/threshold. "The
      driver works" is not an outcome — flag it.
- [ ] **Is each outcome observable, not internal?** "Tests pass" is
      a process state, not a behavior. "Successfully starts" is a
      lifecycle event, not a behavior. Real outcomes describe
      something that crosses a system boundary — a message flowing,
      a response shape, a UI element appearing.
- [ ] **Does each outcome ground against an actor or integration
      boundary the plan cites?** Outcomes about "the system" without
      naming WHICH part are too coarse. The Meshtastic radio actor
      sends MeshPackets; the driver observes them; an outcome
      mentioning "the radio sends data" is a candidate for tightening
      to "the radio sends a `POSITION_APP` packet."

## Granularity check

The planner is supposed to flag epics that resist outcome-enumeration
as too vague. If you see an epic with no outcome AND the plan doesn't
acknowledge the difficulty, that's a gap — push back upstream.

If you see one outcome covering three epics, that's also a gap. The
test the architect later writes can't differentiate three failure
modes from one outcome; either split the outcome or split the epics.

## Examples of substance gaps that warrant `insufficient`

- *"No verifiable outcomes section — only epic decomposition. Cannot
  proceed: architect has no outcomes to transcribe into checks."*
- *"E2 has no verifiable outcome. Plan covers the
  Meshtastic→driver integration boundary but doesn't name what
  observable behavior would prove that integration is working."*
- *"The outcome 'driver receives data and forwards it' is not
  concrete — names neither input message type nor output shape.
  Tighten to specific protobuf message types and observation
  schema."*
- *"E1 outcome is 'the driver successfully starts.' That's a
  lifecycle event, not a behavior. Replace with an outcome that
  describes what the driver DOES once started."*

## Examples of things that are NOT valid grounds for `insufficient`

- *"The Verifiable Outcomes section uses a different markdown header
  than 'Verifiable Outcomes' verbatim."* — substance is what the
  section enumerates, not the literal heading text.
- *"Outcomes don't use Given/When/Then format."* — that's the
  architect's commitment-shape concern, not yours. The planner's
  outcome is a claim; the architect renders it as Given/When/Then
  later.
- *"Outcomes don't enumerate every edge case."* — coarse outcomes
  that name the happy-path observable behavior are sufficient at
  this gate. Edge-case enumeration is the challenger's job
  (probing what each outcome WOULDN'T catch).

## When to send back upstream

If the plan's epics are decomposable but the planner just didn't
enumerate outcomes → `insufficient` with bullet list naming missing
outcomes per epic.

If the plan's epics themselves are too vague to support outcome
enumeration → `insufficient` with a note that the underlying epic is
the problem; the planner needs to sharpen scope before outcome
work begins.

You evaluate. You do not author outcomes for the planner. If a gap
requires a specific outcome to be named, name the boundary that
needs covering and let the planner choose the wording.

The planner enumerates. You enforce presence and concreteness. The
challenger probes for what each outcome WOULDN'T catch. The
architect transcribes. Each role contributes once.
