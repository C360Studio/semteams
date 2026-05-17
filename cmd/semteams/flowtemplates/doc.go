// Package flowtemplates is the product-shell loader for flow templates.
// It seeds the FLOW_TEMPLATES KV bucket from a directory of JSON files at
// boot, mirroring the upstream persona.LoadFromDirectory pattern.
//
// # Why this lives in product-shell, not upstream
//
// Upstream semstreams ships flowtemplate.Manager (KV-backed CRUD) and the
// six template tools but does NOT ship a directory-loader analog to
// persona.LoadFromDirectory. ADR-042 §"Framework-alignment review" pins
// this as the migration target: upstream flowtemplate.LoadFromDirectory
// should follow the persona-loader shape. Until that lands upstream, the
// product-shell variant here is justified per ADR-031 §discipline
// (framework gap, product instance documents the migration target).
//
// # File layout
//
// Templates live as flat JSON files under <root>/*.json (depth-1, no
// nesting). Each file unmarshals to a flowtemplate.Template; the
// file's ID field (not the filename) determines the KV key. Idempotent
// re-seed: existing template IDs get Update; new IDs get Create.
//
// # Boot ordering
//
// LoadFromDirectory runs after flowtemplate.Manager is constructed and
// before agentic-tools' RegisterBuiltins is wired into the tool
// registry — ADR-042 Phase 1. The bucket needs to be populated before
// list_flow_templates is callable.
package flowtemplates
