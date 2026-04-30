# Output contract (R3.2.2 stub)

R3.2.2 proves only that the mode-transition fires. Concrete contract:

1. Call `read_loop_result` on the prior research-reviewer loop ID
   (passed as a property `research_reviewer_loop_id` on your task)
   to confirm the artifact is reachable. **If `read_loop_result`
   errors** (e.g. the placeholder loop_id substitution did not
   resolve, or the reviewer loop hasn't been written to the
   AGENT_LOOPS bucket yet), proceed to the completion message
   anyway — the R3.2.2 smoke check is the persona-swap firing,
   not the read.
2. Terminate with a **completion** (assistant text response — no
   tool call) whose body acknowledges receipt:

   ```
   Stabilised research artifact received from reviewer loop
   <id>. R3.2.2 mode-transition smoke check complete.
   Full dev-via-spec planning contract lands in R3.3.
   ```

That's it. Do not enumerate epics. Do not propose architecture.
Do not dispatch sub-agents. R3.3 fills in this contract; for now
the test of "did the persona-swap rule fire correctly" is the
only signal R3.2.2 needs.

Termination is the completion message itself — no terminal tool
call. The framework records the completion as the loop's result.
