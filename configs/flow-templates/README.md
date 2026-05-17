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

**Phase 1 (ADR-042) seeded an empty inventory.** Phase 2 authors
the first templates (`research-pipeline`, `dev-via-spec-pipeline`,
`web-research`). Until then, this directory is intentionally empty
of `.json` files; the loader logs a debug message and the tool
surface returns "No flow templates configured."

See [`docs/adr/042-coordinator-instantiated-flows-via-templates.md`](../../docs/adr/042-coordinator-instantiated-flows-via-templates.md)
for the full design.
