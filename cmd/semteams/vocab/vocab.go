// Package vocab declares the SemTeams-local predicate vocabulary.
//
// beta.159's authoring policy (vocabulary.RequireDeclaredPredicate) demands
// that every predicate a configuration surface exposes — rule condition
// fields, rule action predicates, projection-contract groups, ownership
// claims — is DECLARED in the vocabulary registry at the composition root.
// Upstream registers its own vocabularies via vocabulary/builtins.Register();
// this package is the product-shell counterpart for the predicates SemTeams
// authors itself. Runtime graph persistence stays syntax-only, so predicates
// minted dynamically at execution time (e.g. sandbox.attestation.verified-<probe>)
// are deliberately NOT listed here.
//
// Framework alignment (cmd/semteams/tools/README.md discipline): upstream
// explicitly directs applications to "define their own domain-specific
// vocabulary in their own packages and register predicates using the
// vocabulary registry" (vocabulary/predicates.go) — this package implements
// that contract; it is not a product-local workaround.
//
// Scope: KEPT lanes only (research, autoresearch, coordinator, agent-run
// markers, chain pause/decision). Parked-pack predicates (ADR-058) are
// re-declared here as part of each pack's restore checklist.
package vocab

import "github.com/c360studio/semstreams/vocabulary"

// Register declares every SemTeams-authored predicate. Safe to call more
// than once. Call it at the composition root BEFORE configuration/rule
// validation, right after vocabulary/builtins.Register().
func Register() {
	for _, meta := range predicates {
		vocabulary.RegisterPredicate(meta)
	}
}

var predicates = []vocabulary.PredicateMetadata{
	// --- research pack ---
	{
		Name:        "research.gather.completed-subtopic",
		Description: "Scatter/gather join marker: one gatherer loop finished this subtopic (stamped on the plan loop entity; the join rule fires on length_eq N)",
		DataType:    "string",
	},
	{
		Name:        "research.artifact.path",
		Description: "Filesystem path of the rendered research artifact (stamped on the synthesize/reviewer loop entity)",
		DataType:    "string",
	},

	// --- autoresearch pack ---
	{
		Name:        "autoresearch.run.status",
		Description: "Autoresearch run status (running | stopped); replace-owned by rule-pack.semteams",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.run.cap",
		Description: "Iteration cap for the autoresearch run",
		DataType:    "number",
	},
	{
		Name:        "autoresearch.run.command",
		Description: "Measurement command for the autoresearch run",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.run.surface",
		Description: "Bounded edit surface for the autoresearch run",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.run.metric-parser",
		Description: "Metric-parser hint for reading the measurement command's output",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.iteration.pending",
		Description: "Presence marker: an iteration is in flight (replace-owned; removed at completion, re-added on dispatch)",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.experiment.completed",
		Description: "Per-iteration journal entry: an execute loop completed (accumulating)",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.experiment.loop-failed",
		Description: "Per-iteration journal entry: an execute loop failed involuntarily (accumulating)",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.best.value",
		Description: "Best measurement value so far (replace-owned scalar; lower is better in v1)",
		DataType:    "number",
	},
	{
		Name:        "autoresearch.best.experiment-id",
		Description: "Loop ID of the experiment that produced the current best value (replace-owned)",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.measurement.outcome",
		Description: "Outcome token of one measurement (kept | reverted | failed)",
		DataType:    "string",
	},
	{
		Name:        "autoresearch.measurement.value",
		Description: "Scalar value of one measurement",
		DataType:    "number",
	},
	{
		Name:        "autoresearch.artifact.path",
		Description: "Filesystem path of the rendered autoresearch rollup artifact",
		DataType:    "string",
	},

	// --- agent-run lifecycle stamps authored by SemTeams rules ---
	// (upstream's agentic/agentrun registers agent.run.phase + the
	// last-transition audit family; outcome/handoff are product stamps)
	{
		Name:        "agent.run.outcome",
		Description: "Terminal outcome of the run (success | failed), stamped by pack terminal rules; drives executing→completed/failed",
		DataType:    "string",
	},
	{
		Name:        "agent.run.handoff",
		Description: "Dispatched→executing handoff marker stamped by agent-run rules 01/01b after a task publish",
		DataType:    "string",
	},

	// --- agent-run pause/resume markers (ADR-053, SemTeams-authored) ---
	{
		Name:        "agent.run.approval-pending",
		Description: "Run-pause marker: a gated tool call awaits human approval (stamped by approvalpause + agent-run rule 12; removed on resume)",
		DataType:    "string",
	},
	{
		Name:        "agent.run.approval-resumed",
		Description: "Run-resume audit marker: the approval decision that un-parked the run",
		DataType:    "string",
	},
	{
		Name:        "agent.run.clarification-pending",
		Description: "Run-pause marker: decide(ask_user) awaits the human's reply (agent-run rules 07/08; removed on resume)",
		DataType:    "string",
	},
	{
		Name:        "agent.run.clarification-resumed",
		Description: "Run-resume audit marker: the user reply that un-parked the run (agent-run rule 10)",
		DataType:    "string",
	},

	// --- agent.lineage.<key> threads this shell authors rules against ---
	// The framework MINTS these via its lineage namespace delegation at
	// spawn time (related_loops); declaring them here is what lets our
	// rules condition on / substitute them under the authoring policy.
	{
		Name:        "agent.lineage.run-loop-entity-id",
		Description: "Threaded run-entity anchor for run-entity-descended loops (agent-run rules 06/08/13 anchor branch)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.plan-loop-entity-id",
		Description: "Threaded plan-loop anchor gatherers stamp their join marker against (research 03a/03b)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.researcher",
		Description: "Threaded synthesize-loop ID for the research reviewer's retry path (research 05)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.autoresearch-run",
		Description: "Threaded run-entity ID for autoresearch phase loops (autoresearch 03/07/08/09/10b)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.command",
		Description: "Threaded measurement command for autoresearch execute loops (autoresearch 03/05)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.surface",
		Description: "Threaded edit surface for autoresearch execute loops (autoresearch 03/05)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.metric-parser",
		Description: "Threaded metric-parser hint for autoresearch execute loops (autoresearch 03/05)",
		DataType:    "string",
	},
	{
		Name:        "agent.lineage.trigger-loop",
		Description: "Threaded triggering-loop ID for ops observer spawns (ops observe rules)",
		DataType:    "string",
	},

	// --- coordinator clarification prose ---
	{
		Name:        "coordinator.clarification.question",
		Description: "The user-facing clarification question the coordinator asked (decide(ask_user) reason)",
		DataType:    "string",
	},
	{
		Name:        "coordinator.clarification.reply",
		Description: "The user's reply that resumes a clarification-paused run",
		DataType:    "string",
	},

	// --- upstream-engine stamps this shell's rules condition on ---
	// The rule engine writes rule.task.spawned after a publish_agent
	// (processor/rule/actions.go), but neither vocabulary/agentic nor
	// vocabulary/rulepacks declares it; consumers must. Candidate for an
	// upstream ask: declare engine-stamped predicates in a framework vocab.
	{
		Name:        "rule.task.spawned",
		Description: "Task ID the rule engine recorded after a successful publish_agent (agent-run 01/01b gate on its presence)",
		DataType:    "string",
	},

	// --- chain pause / operator decision (ADR-037, chainpause subscriber) ---
	{
		Name:        "chain.paused.marker",
		Description: "Presence marker: a managed-arc loop failed and the chain is paused for operator triage",
		DataType:    "string",
	},
	{
		Name:        "chain.evidence.summary-ready",
		Description: "Evidence summary for a completed chain is available for ops consumption",
		DataType:    "string",
	},
}
