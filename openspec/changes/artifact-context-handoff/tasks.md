# Tasks

## 1. UI Handoff State
- [x] 1.1 Add a chat handoff store for pending prompt drafts and attached artifact context.
- [x] 1.2 Show and clear attached artifact context in the chat bar.
- [x] 1.3 Append attached artifact context to the next coordinator message and clear it after send.

## 2. Artifact Card Actions
- [x] 2.1 Render generic copy and team-handoff controls for non-OpenSpec emitted artifacts.
- [x] 2.2 Keep OpenSpec-specific review/export controls unchanged.
- [x] 2.3 Ensure handoff controls expose multiple public teams instead of a single assumed next step.
- [x] 2.4 Surface descendant loops in the parent task detail panel so nested emitted artifacts are reachable.

## 3. Evidence
- [x] 3.1 Add unit tests for artifact copy and generic team handoff.
- [x] 3.2 Add unit tests for chat context visibility, clearing, and outbound prompt composition.
- [x] 3.3 Add a Playwright journey for research-artifact handoff into an editable coordinator prompt.
- [x] 3.4 Validate the OpenSpec change, focused UI tests, and artifact handoff journey.
