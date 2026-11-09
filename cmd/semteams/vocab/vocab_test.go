package vocab_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
	"github.com/c360studio/semteams/cmd/semteams/vocab"
)

type expectedVocabPredicate struct {
	name        string
	description string
	dataType    string
}

var expectedSemTeamsPredicates = []expectedVocabPredicate{
	{
		name:        "research.gather.completed-subtopic",
		description: "Scatter/gather join marker: one gatherer loop finished this subtopic (stamped on the plan loop entity; the join rule fires on length_eq N)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "research.artifact.path",
		description: "Filesystem path of the rendered research artifact (stamped on the synthesize/reviewer loop entity)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.reply.approved",
		description: "Immutable approved source loop entity reference, relayed from the run to its canonical origin for routed reply delivery",
		dataType:    vocabulary.DataTypeEntityID,
	},
	{
		name:        "autoresearch.reply.clarification",
		description: "Immutable clarification source loop entity reference, relayed from the run to its canonical origin for routed reply delivery",
		dataType:    vocabulary.DataTypeEntityID,
	},
	{
		name:        "autoresearch.reply.failed",
		description: "Immutable failed source loop entity reference, relayed from the run to its canonical origin for routed reply delivery",
		dataType:    vocabulary.DataTypeEntityID,
	},
	{
		name:        "autoresearch.run.status",
		description: "Autoresearch run status (running | stopped); replace-owned by rule-pack.semteams",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.run.cap",
		description: "Iteration cap for the autoresearch run",
		dataType:    vocabulary.DataTypeInt,
	},
	{
		name:        "autoresearch.run.command",
		description: "Measurement command for the autoresearch run",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.run.surface",
		description: "Bounded edit surface for the autoresearch run",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.run.metric-parser",
		description: "Metric-parser hint for reading the measurement command's output",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.iteration.pending",
		description: "Presence marker: an iteration is in flight (replace-owned; removed at completion, re-added on dispatch)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.experiment.completed",
		description: "Per-iteration journal entry: an execute loop completed (accumulating)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.experiment.loop-failed",
		description: "Per-iteration journal entry: an execute loop failed involuntarily (accumulating)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.best.value",
		description: "Best measurement value so far (replace-owned scalar; lower is better in v1)",
		dataType:    vocabulary.DataTypeFloat,
	},
	{
		name:        "autoresearch.best.experiment-id",
		description: "Loop ID of the experiment that produced the current best value (replace-owned)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.measurement.outcome",
		description: "Outcome token of one measurement (kept | reverted | failed)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "autoresearch.measurement.value",
		description: "Scalar value of one measurement",
		dataType:    vocabulary.DataTypeFloat,
	},
	{
		name:        "autoresearch.artifact.path",
		description: "Filesystem path of the rendered autoresearch rollup artifact",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.run.outcome",
		description: "Terminal outcome of the run (success | failed), stamped by pack terminal rules; drives executing→completed/failed",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.run.handoff",
		description: "Dispatched→executing handoff marker stamped by agent-run rules 01/01b after a task publish",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.run.approval-outstanding",
		description: "Atomic count of observed approval gates without answers",
		dataType:    vocabulary.DataTypeInt,
	},
	{
		name:        "agent.run.approval-pending",
		description: "Append-only observed gate JSON pair [loopID,executionID]; pending minus answered identifies waiting gates",
		dataType:    vocabulary.DataTypeJSON,
	},
	{
		name:        "agent.run.approval-answered",
		description: "Append-only answered gate JSON pair [loopID,executionID]; each answer also appends the pending pair",
		dataType:    vocabulary.DataTypeJSON,
	},
	{
		name:        "agent.run.clarification-pending",
		description: "Run-pause marker: decide(ask_user) awaits the human's reply (agent-run rules 07/08; removed on resume)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.run.clarification-resumed",
		description: "Run-resume audit marker: the user reply that un-parked the run (agent-run rule 10)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.run-loop-entity-id",
		description: "Threaded run-entity anchor for run-entity-descended loops (agent-run rules 06/08/13 anchor branch)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.plan-loop-entity-id",
		description: "Threaded plan-loop anchor gatherers stamp their join marker against (research 03a/03b)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.researcher",
		description: "Threaded synthesize-loop ID for the research reviewer's retry path (research 05)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.autoresearch-run",
		description: "Threaded run-entity ID for autoresearch phase loops (autoresearch 03/07/08/09/10b)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.command",
		description: "Threaded measurement command for autoresearch execute loops (autoresearch 03/05)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.surface",
		description: "Threaded edit surface for autoresearch execute loops (autoresearch 03/05)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.metric-parser",
		description: "Threaded metric-parser hint for autoresearch execute loops (autoresearch 03/05)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "agent.lineage.trigger-loop",
		description: "Threaded triggering-loop ID for ops observer spawns (ops observe rules)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "coordinator.clarification.question",
		description: "The user-facing clarification question the coordinator asked (decide(ask_user) reason)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "coordinator.clarification.reply",
		description: "The user's reply that resumes a clarification-paused run",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "rule.task.spawned",
		description: "Task ID the rule engine recorded after a successful publish_agent (agent-run 01/01b gate on its presence)",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "chain.paused.marker",
		description: "Presence marker: a managed-arc loop failed and the chain is paused for operator triage",
		dataType:    vocabulary.DataTypeString,
	},
	{
		name:        "chain.evidence.summary-ready",
		description: "Evidence summary for a completed chain is available for ops consumption",
		dataType:    vocabulary.DataTypeString,
	},
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestRegisterDeclaresSemTeamsVocabularyIdempotently(t *testing.T) {
	restore := vocabulary.SnapshotRegistry()
	defer restore()
	vocabulary.ClearRegistry()

	if got := len(vocabulary.ListRegisteredPredicates()); got != 0 {
		t.Fatalf("expected empty registry after ClearRegistry, got %d", got)
	}

	vocab.Register()
	vocab.Register() // idempotent re-registration must preserve count and metadata

	registered := vocabulary.ListRegisteredPredicates()
	sort.Strings(registered)
	if len(registered) != 38 || len(expectedSemTeamsPredicates) != 38 {
		t.Fatalf("registered predicate count = %d, want 38 (%v)", len(registered), registered)
	}

	for _, exp := range expectedSemTeamsPredicates {
		if err := vocabulary.RequireDeclaredPredicate(exp.name); err != nil {
			t.Errorf("RequireDeclaredPredicate(%q): %v", exp.name, err)
			continue
		}
		meta := vocabulary.GetPredicateMetadata(exp.name)
		if meta == nil {
			t.Errorf("GetPredicateMetadata(%q) returned nil", exp.name)
			continue
		}
		segs := strings.Split(exp.name, ".")
		if meta.Domain != segs[0] || meta.Category != segs[1] {
			t.Errorf("%s: Domain/Category = %q/%q, want %q/%q",
				exp.name, meta.Domain, meta.Category, segs[0], segs[1])
		}
		if meta.Description != exp.description {
			t.Errorf("%s: Description = %q, want %q", exp.name, meta.Description, exp.description)
		}
		if meta.DataType != exp.dataType {
			t.Errorf("%s: DataType = %q, want %q", exp.name, meta.DataType, exp.dataType)
		}
	}

	if err := vocabulary.RequireDeclaredPredicate("workflow.state.phase"); err == nil {
		t.Fatal("expected rulepacks-only predicate workflow.state.phase to remain undeclared")
	}
}
