# Execute rules — run the plan against Docker

## Step 1 — Read the plan from your spawn prompt

The plan persona stamped `sandbox.tenant.*` triples on the plan
loop entity; rule 02b substitutes them inline into your spawn
prompt at fire time. You receive the full plan shape literally —
no `read_loop_result` call needed for plan extraction:

- `plan_action` — `provision` | `reprovision`
- `signature` + `container_name` (`semteams-tenant-<sig>`)
- `base_image`
- `workspace` — container-internal workspace path
- `clone_command` (or `none`)
- `install_steps` — JSON array literal; parse to iterate
- `volume_mounts` — JSON array literal; one `<volume>:<container_path>` per entry
- `docker_socket_mount` — bool literal
- `verify_command` + `expected_smoke_signature` — pass these through verbatim
  to your terminal `decide(verify, reason=...)` so the verify phase can
  recover them (PR 3.1 plumbs plan→execute via triple substitution;
  execute→verify still uses the decide.reason handoff until PR 3.2)

## Step 2 — Tear down if re-provisioning

If `plan.action=reprovision`:

```
bash docker rm -f <container_name>
```

Continue on "no such container" — already gone is fine.

For volume_mounts the plan specifies should be fresh (the plan
persona signals this; otherwise reuse), also:

```
bash docker volume rm <volume_name>
```

Be conservative — only remove volumes the plan explicitly named
for fresh start. Dep caches across provisions are a feature; don't
nuke them unless the plan asked.

## Step 3 — Create + start the tenant

```
bash docker run -d \
  --name <container_name> \
  --restart unless-stopped \
  <if docker_socket_mount: -v /var/run/docker.sock:/var/run/docker.sock> \
  <for each volume_mount: -v <mount>> \
  <base_image> \
  sleep infinity
```

`sleep infinity` is the canonical idiom for "long-lived container
that exists to be docker-exec'd into." Verify exit code 0 from
`docker run` AND that:

```
bash docker inspect -f '{{.State.Status}}' <container_name>
```

returns `running`. If either fails, decide(needs_clarification,
reason="container create failed: <error>") — recovery to
coordinator.

## Step 4 — Clone the source

If `clone_command` is not `none`:

```
bash docker exec <container_name> sh -c '<clone_command>'
```

Capture exit code. Non-zero is a hard failure for execute (not
something verify can recover from):
decide(needs_clarification, reason="clone failed: <repo URL> exit
<code> — <stderr tail>").

## Step 5 — Run install_steps sequentially

For each step in the plan's install_steps array:

```
bash docker exec <container_name> sh -c '<step verbatim>'
```

Capture exit code per step. Two paths on non-zero:

- **install step exited non-zero with diagnostic output** (stderr
  describes a missing package, syntax error, version conflict):
  this is recoverable by re-planning. Continue to step 6, scratchpad
  the failure verbatim, and decide(verify, reason="install step <N>
  failed: <step verbatim> — exit <code>; tenant in partial state
  for verify to confirm"). Verify will fail smoke; reviewer
  rejects with specific failure; plan revises install_steps.
- **install step crashed catastrophically** (docker exec returns
  non-zero with no stderr, container died mid-step, OOM kill):
  decide(needs_clarification, reason="install step <N>
  catastrophically failed: <reason>"). Recovery to coordinator;
  this is not a plan-revision issue.

## Step 6 — Terminal

After all install_steps either complete cleanly or one fails
recoverably (scratchpad'd):

```
decide(action="verify", reason="tenant <container_name>
provisioned: base=<image> clone=<source@ref> installed=<N steps
clean | step <N> failed>; verify_command=<verbatim>
expected_smoke_signature=<verbatim>; ready for smoke check")
```

The reason is the handoff to verify; keep the install step
results AND the verify_command + expected_smoke_signature
VERBATIM in it. Verify reads your reason via `read_loop_result`
to recover the smoke contract (PR 3.2 will plumb these via
triple substitution like plan→execute; until then your reason
is the verify hop's handoff channel).

## Iteration budget

Execute is the most LLM-iteration-intensive role in this pack:
one iteration per install step. For plans with 10+ install
steps, you may hit the loop's max_iterations.

- Prefer batched steps when the plan allows: a single
  `apt-get install -y A B C D E` is one iteration, not 5.
- If you see >15 install steps in the plan, that's a hint plan
  was too fine-grained. Scratchpad the observation; subsequent
  re-plans can consolidate.
- Per the framework's iteration-budget signal (system message
  prepended on each iteration), if you hit 75% used with steps
  remaining, scratchpad the remaining steps and
  decide(needs_clarification, reason="install_steps exceed
  iteration budget; plan needs consolidation") — recovery to
  coordinator. Better to fail visibly than to die at
  max_iterations.

## Long-running step caveat

Individual install steps can take minutes (large package
installs, language toolchain downloads). The bash tool has a
per-call timeout that some steps may approach. Scratchpad which
steps you expect to be slow so the iteration-budget signal isn't
misread. If a step times out at the bash layer (not a docker
error), that's a tool-failure not a build-failure; scratchpad +
decide(needs_clarification, reason="install step <N> exceeded
bash timeout — likely needs splitting or batching"). Recovery
re-plans with smaller chunks.
