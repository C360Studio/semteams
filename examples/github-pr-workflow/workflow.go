// Package githubprworkflow demonstrates a reactive workflow that automates
// the path from a GitHub issue to a merged pull request. Three adversarial
// agents — qualifier, developer, and reviewer — iterate against each other
// until the PR is approved or the review cycle limit is reached.
package githubprworkflow

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/processor/reactive"
)

const (
	// StateBucket is the NATS KV bucket that stores workflow execution state.
	StateBucket = "GITHUB_ISSUE_PR_STATE"

	// MaxReviewCycles is the maximum number of reviewer rejection/retry loops
	// before the workflow escalates to human intervention.
	MaxReviewCycles = 3

	// WorkflowTimeout is the maximum wall-clock duration for any single
	// execution before the engine marks it as timed out.
	WorkflowTimeout = 30 * time.Minute

	// DefaultTokenBudget is the maximum tokens (in+out) per workflow execution.
	// ~$5 at GPT-4o pricing — enough for qualify + develop + review.
	DefaultTokenBudget = 500_000

	// DefaultMaxConcurrentWorkflows limits parallel active workflows.
	DefaultMaxConcurrentWorkflows = 3

	// DefaultHourlyTokenCeiling is the global hourly token limit across all workflows.
	// ~$20/hour hard ceiling.
	DefaultHourlyTokenCeiling = 2_000_000

	// DefaultIssueCooldown is the minimum time between processing new issues.
	DefaultIssueCooldown = 60 * time.Second
)

// Phase constants represent the distinct states an issue-to-PR execution
// can occupy. They are written into IssueToPRState.Phase and drive all
// KV-triggered rule conditions.
const (
	PhaseQualified        = "qualified"
	PhaseRejected         = "rejected"
	PhaseNotABug          = "not_a_bug"
	PhaseWontFix          = "wont_fix"
	PhaseNeedsInfo        = "needs_info"
	PhaseAwaitingInfo     = "awaiting_info"
	PhaseDevComplete      = "dev_complete"
	PhaseDeveloping       = "developing"
	PhaseApproved         = "approved"
	PhaseChangesRequested = "changes_requested"
	PhaseEscalated        = "escalated"
)

// NewIssueToPRWorkflow builds the reactive workflow definition for the
// adversarial issue qualification, development, and review pipeline.
func NewIssueToPRWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("github-issue-to-pr").
		WithDescription("Adversarial issue qualification, development, and review pipeline").
		WithStateBucket(StateBucket).
		WithStateFactory(func() any { return &IssueToPRState{} }).
		WithMaxIterations(MaxReviewCycles + 3). // qualify(1) + develop(1+N retries) + review cycles
		WithTimeout(WorkflowTimeout).
		WithOnComplete("github.workflow.complete").
		WithOnFail("github.workflow.failed").
		WithOnEscalate("github.workflow.escalated").

		// Rule 1: spawn-qualifier
		// When a new issue event arrives, spawn the qualifier agent to decide
		// whether the issue is worth acting on.
		AddRuleFromBuilder(
			reactive.NewRule("spawn-qualifier").
				OnSubject("github.event.issue", func() any { return &GitHubIssueWebhookEvent{} }).
				When("issue action is opened", func(ctx *reactive.RuleContext) bool {
					event, ok := ctx.Message.(*GitHubIssueWebhookEvent)
					if !ok {
						return false
					}
					return event.Action == "opened"
				}).
				PublishAsync(
					"agent.task.qualifier",
					buildQualifierTask,
					"agentic.loop_completed.v1",
					handleQualifierResult,
				).
				WithCooldown(DefaultIssueCooldown).
				WithMaxFirings(1),
		).

		// Rule 2: spawn-developer
		// When the qualifier marks the issue as qualified, spawn the developer
		// agent to implement a fix and open a PR.
		AddRuleFromBuilder(
			reactive.NewRule("spawn-developer").
				WatchKV(StateBucket, "*").
				When("phase is qualified", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseQualified
				}).
				When("token budget not exceeded", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return (s.TotalTokensIn + s.TotalTokensOut) < DefaultTokenBudget
				}).
				PublishAsync(
					"agent.task.developer",
					buildDeveloperTask,
					"agentic.loop_completed.v1",
					handleDeveloperResult,
				),
		).

		// Rule 3: spawn-reviewer
		// When development is complete and review cycles remain, spawn the
		// reviewer agent for an adversarial code review.
		AddRuleFromBuilder(
			reactive.NewRule("spawn-reviewer").
				WatchKV(StateBucket, "*").
				When("phase is dev_complete", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseDevComplete
				}).
				When("review cycles not exhausted", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return s.ReviewRejections < MaxReviewCycles
				}).
				When("token budget not exceeded", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return (s.TotalTokensIn + s.TotalTokensOut) < DefaultTokenBudget
				}).
				PublishAsync(
					"agent.task.reviewer",
					buildReviewerTask,
					"agentic.loop_completed.v1",
					handleReviewerResult,
				),
		).

		// Rule 4: review-approved
		// When the reviewer approves the PR, emit a completion event and mark
		// the execution as successfully finished.
		AddRuleFromBuilder(
			reactive.NewRule("review-approved").
				WatchKV(StateBucket, "*").
				When("phase is approved", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseApproved
				}).
				CompleteWithEvent("github.workflow.complete", buildCompletionPayload),
		).

		// Rule 5: review-rejected-retry
		// When the reviewer requests changes and retry budget remains, reset
		// the phase to "developing" so the developer rule fires again.
		AddRuleFromBuilder(
			reactive.NewRule("review-rejected-retry").
				WatchKV(StateBucket, "*").
				When("phase is changes_requested", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseChangesRequested
				}).
				When("retry budget not exhausted", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return s.ReviewRejections < MaxReviewCycles
				}).
				Mutate(func(ctx *reactive.RuleContext, _ any) error {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return nil
					}
					s.Phase = PhaseDeveloping
					s.DevelopmentAttempts++
					return nil
				}),
		).

		// Rule 6: escalate-deadlock
		// When the reviewer keeps requesting changes and the retry budget is
		// exhausted, escalate to a human rather than looping forever.
		AddRuleFromBuilder(
			reactive.NewRule("escalate-deadlock").
				WatchKV(StateBucket, "*").
				When("phase is changes_requested", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseChangesRequested
				}).
				When("retry budget exhausted", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return s.ReviewRejections >= MaxReviewCycles
				}).
				Mutate(func(ctx *reactive.RuleContext, _ any) error {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return nil
					}
					s.EscalatedToHuman = true
					s.Phase = PhaseEscalated
					reactive.SetStatus(ctx.State, reactive.StatusEscalated)
					return nil
				}),
		).

		// Rule 7: issue-rejected
		// When the qualifier deems the issue invalid (rejected, not a bug, or
		// won't fix), complete the workflow without spawning a developer.
		AddRuleFromBuilder(
			reactive.NewRule("issue-rejected").
				WatchKV(StateBucket, "*").
				When("phase is a rejection variant", func(ctx *reactive.RuleContext) bool {
					phase := reactive.GetPhase(ctx.State)
					return phase == PhaseRejected ||
						phase == PhaseNotABug ||
						phase == PhaseWontFix
				}).
				CompleteWithMutation(func(ctx *reactive.RuleContext, _ any) error {
					reactive.SetStatus(ctx.State, reactive.StatusCompleted)
					return nil
				}),
		).

		// Rule 8: needs-info
		// When the qualifier determines more information is required before
		// acting, park the execution in "awaiting_info" to avoid re-triggering.
		AddRuleFromBuilder(
			reactive.NewRule("needs-info").
				WatchKV(StateBucket, "*").
				When("phase is needs_info", func(ctx *reactive.RuleContext) bool {
					return reactive.GetPhase(ctx.State) == PhaseNeedsInfo
				}).
				Mutate(func(ctx *reactive.RuleContext, _ any) error {
					es := reactive.ExtractExecutionState(ctx.State)
					if es != nil {
						es.Phase = PhaseAwaitingInfo
					}
					return nil
				}),
		).

		// Rule 9: budget-exceeded
		// When total tokens exceed the per-execution budget, escalate to a
		// human rather than continuing to burn tokens.
		AddRuleFromBuilder(
			reactive.NewRule("budget-exceeded").
				WatchKV(StateBucket, "*").
				When("token budget exceeded", func(ctx *reactive.RuleContext) bool {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return false
					}
					return (s.TotalTokensIn + s.TotalTokensOut) >= DefaultTokenBudget
				}).
				When("not already terminal", func(ctx *reactive.RuleContext) bool {
					return !reactive.IsTerminal(ctx.State)
				}).
				Mutate(func(ctx *reactive.RuleContext, _ any) error {
					s, ok := ctx.State.(*IssueToPRState)
					if !ok {
						return nil
					}
					s.Phase = PhaseEscalated
					s.EscalatedToHuman = true
					reactive.SetStatus(ctx.State, reactive.StatusEscalated)
					reactive.SetError(ctx.State, fmt.Sprintf(
						"token budget exceeded: %d tokens used (limit: %d)",
						s.TotalTokensIn+s.TotalTokensOut, DefaultTokenBudget))
					return nil
				}),
		).
		MustBuild()
}

// PhaseIsActive returns true if the phase indicates an active workflow.
func PhaseIsActive(phase string) bool {
	switch phase {
	case PhaseQualified, PhaseDeveloping, PhaseDevComplete,
		PhaseChangesRequested, PhaseNeedsInfo, PhaseAwaitingInfo:
		return true
	}
	return false
}
