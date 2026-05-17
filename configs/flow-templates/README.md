# Flow templates

Parameterised flow definitions seeded into the `FLOW_TEMPLATES`
NATS KV bucket at boot via `cmd/semteams/flowtemplates.LoadFromDirectory`.

Each `*.json` file in this directory is a single
`flowtemplate.Template` (see upstream
`github.com/c360studio/semstreams/flowtemplate/template.go`). The
loader skips non-`.json` files and any nested directories.

## File shape

```json
{
  "id": "kebab-case-id",
  "name": "Human-friendly name",
  "description": "Optional context for the coordinator.",
  "body": "<flow JSON with {{.ParamName}} placeholders>",
  "parameters": [
    {
      "name": "ParamName",
      "description": "What the placeholder expects.",
      "default": "default-string-value",
      "required": false
    }
  ]
}
```

`body` is a text/template-rendered flow JSON. The runtime
substitution happens when the coordinator calls
`instantiate_flow_template`.

### A note on slug parameters

Some templates declare a paired `topic` / `topic_slug` (or
`target_framework` / `target_slug`) parameter. The slug variant
is coordinator-derived because Go's `text/template` substitution
is string-only with no built-in funcmap for case-folding or
slugifying. The coordinator persona (Phase 3) is responsible for
deriving the kebab-case slug from the human-readable input and
supplying both parameters when calling `instantiate_flow_template`.
The contract test pins the rendered flow ID to kebab-case so a
missing-slug parameter fails loud at boot rather than producing
unparseable flow IDs at runtime.

## Loader semantics

- **Source of truth**: files here override any KV entries with the
  same `id`. Runtime edits via `create_flow_template` /
  `update_flow_template` are ephemeral and reset on next boot.
- **Idempotent re-seed**: existing IDs get `Update`; new IDs get
  `Create`.
- **Missing directory**: warning, not fatal.
- **Malformed JSON / failed `Template.Validate()`**: file skipped
  with a warning. Boot stays alive.
- **CLI override**: `--flow-templates <path>` or env
  `SEMTEAMS_FLOW_TEMPLATES_PATH`.

## Status

**Phase 2a (ADR-042) shipped three skeleton templates:**

| Template | Domain | Parameters |
|---|---|---|
| `research-pipeline` | Software (source-repo substrate via semsource) | `topic`, `topic_slug`, `max_iterations`, `model` |
| `dev-via-spec-pipeline` | Software (researcher → architect → builder) | `target_framework`, `target_slug`, `sandbox_image`, `max_iterations`, `model` |
| `web-research` | Non-software (web substrate, OSINT discipline) | `topic`, `topic_slug`, `max_sources`, `confidence_threshold`, `model` |

Phase 2a bodies are minimal flow skeletons — they pass
`Flow.Validate()` (id + name + valid runtime_state + empty
nodes/connections) but do NOT yet wire the agentic components.
Phase 2b expands each body to a runnable topology mirroring the
legacy `configs/dev-research.json` / `configs/osh-demo.json`
shapes, with parameter substitution at LLM-author-controllable
knobs only.

The current bodies validate the seed + instantiate path
end-to-end: coordinator can `list_flow_templates`,
`get_flow_template`, `instantiate_flow_template`, and the
rendered flow passes `Flow.Validate()`. They do NOT yet produce
runnable flows; Phase 5's real-LLM smokes are gated on Phase 2b
expanding the bodies.

See [`docs/adr/042-coordinator-instantiated-flows-via-templates.md`](../../docs/adr/042-coordinator-instantiated-flows-via-templates.md)
for the full design.
