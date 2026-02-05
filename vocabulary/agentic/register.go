package agentic

import "github.com/c360studio/semstreams/vocabulary"

// Register registers all agentic predicates with the vocabulary registry,
// including IRI mappings to W3C S-Agent-Comm ontology for standards compliance.
//
// Call this function during application initialization to enable predicate
// metadata lookup and IRI-based export.
//
// Example:
//
//	func init() {
//	    agentic.Register()
//	}
//
//	// Later, retrieve metadata including IRI mapping
//	meta := vocabulary.GetPredicateMetadata(agentic.IntentGoal)
//	fmt.Println(meta.StandardIRI)  // https://w3id.org/agent-ontology/core#Intent
func Register() {
	registerIntentPredicates()
	registerCapabilityPredicates()
	registerDelegationPredicates()
	registerAccountabilityPredicates()
	registerExecutionPredicates()
	registerActionPredicates()
	registerTaskPredicates()
}

// registerIntentPredicates registers predicates for agent intentions and goals.
func registerIntentPredicates() {
	vocabulary.Register(IntentGoal,
		vocabulary.WithDescription("The objective or goal an agent aims to achieve"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriIntent))

	vocabulary.Register(IntentType,
		vocabulary.WithDescription("Category of intent (e.g., data-analysis, content-generation)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriHasIntentType))

	vocabulary.Register(IntentParameter,
		vocabulary.WithDescription("Typed parameter for the intent"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriHasParameter))

	vocabulary.Register(IntentAuthorized,
		vocabulary.WithDescription("Delegation authorizing this intent"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriAuthorizedBy))

	vocabulary.Register(IntentProduces,
		vocabulary.WithDescription("Action produced by this intent"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriProducesAction))
}

// registerCapabilityPredicates registers predicates for agent capabilities.
func registerCapabilityPredicates() {
	vocabulary.Register(CapabilityName,
		vocabulary.WithDescription("Identifier for an agent capability"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriCapability))

	vocabulary.Register(CapabilityDescription,
		vocabulary.WithDescription("Human-readable description of the capability"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(CapabilityExpression,
		vocabulary.WithDescription("Semantic fingerprint for capability matching and embedding"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriCapabilityExpression))

	vocabulary.Register(CapabilityConfidence,
		vocabulary.WithDescription("Agent's self-assessed confidence in capability (0.0-1.0)"),
		vocabulary.WithDataType("float64"),
		vocabulary.WithRange("0-1"),
		vocabulary.WithIRI(IriCapabilityConfidence))

	vocabulary.Register(CapabilitySkill,
		vocabulary.WithDescription("Atomic skill implementing the capability"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriHasSkill))

	vocabulary.Register(CapabilityConstraint,
		vocabulary.WithDescription("Execution constraint on the capability"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriCapabilityConstraint))

	vocabulary.Register(CapabilityPermission,
		vocabulary.WithDescription("Permission required for the capability"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriRequiresPermission))
}

// registerDelegationPredicates registers predicates for authority delegation.
func registerDelegationPredicates() {
	vocabulary.Register(DelegationFrom,
		vocabulary.WithDescription("Agent granting delegated authority"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriDelegatedBy),
		vocabulary.WithInverseOf(DelegationTo))

	vocabulary.Register(DelegationTo,
		vocabulary.WithDescription("Agent receiving delegated authority"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriDelegatesTo),
		vocabulary.WithInverseOf(DelegationFrom))

	vocabulary.Register(DelegationScope,
		vocabulary.WithDescription("Boundary of delegated authority"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriDelegationScope))

	vocabulary.Register(DelegationCapability,
		vocabulary.WithDescription("Capability allowed by the delegation"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriAllowedCapability))

	vocabulary.Register(DelegationValidFrom,
		vocabulary.WithDescription("When the delegation becomes valid"),
		vocabulary.WithDataType("time.Time"),
		vocabulary.WithIRI(IriValidFrom))

	vocabulary.Register(DelegationValidUntil,
		vocabulary.WithDescription("When the delegation expires"),
		vocabulary.WithDataType("time.Time"),
		vocabulary.WithIRI(IriValidUntil))

	vocabulary.Register(DelegationChain,
		vocabulary.WithDescription("Multi-level delegation chain identifier"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriDelegationChain))
}

// registerAccountabilityPredicates registers predicates for accountability tracking.
func registerAccountabilityPredicates() {
	vocabulary.Register(AccountabilityActor,
		vocabulary.WithDescription("Agent performing the accountable action"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriActor))

	vocabulary.Register(AccountabilityAction,
		vocabulary.WithDescription("Action being accounted for"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(AccountabilityAssigned,
		vocabulary.WithDescription("Party assigned responsibility"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriAssignedTo))

	vocabulary.Register(AccountabilityRationale,
		vocabulary.WithDescription("Reasoning for the responsibility attribution"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriRationale))

	vocabulary.Register(AccountabilityCompliance,
		vocabulary.WithDescription("Compliance assessment result"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriComplianceAssessment))

	vocabulary.Register(AccountabilityTimestamp,
		vocabulary.WithDescription("When the accountability event occurred"),
		vocabulary.WithDataType("time.Time"))
}

// registerExecutionPredicates registers predicates for execution context.
func registerExecutionPredicates() {
	vocabulary.Register(ExecutionEnvironment,
		vocabulary.WithDescription("Runtime environment type (sandbox, container, etc.)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriExecutionEnvironment))

	vocabulary.Register(ExecutionSecurity,
		vocabulary.WithDescription("Security context for execution"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriSecurityContext))

	vocabulary.Register(ExecutionConstraint,
		vocabulary.WithDescription("Resource constraint for execution"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriResourceConstraint))

	vocabulary.Register(ExecutionRateLimit,
		vocabulary.WithDescription("Rate limiting constraint"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriRateLimit))

	vocabulary.Register(ExecutionBudget,
		vocabulary.WithDescription("Cost or resource budget for execution"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriBudget))

	vocabulary.Register(ExecutionInput,
		vocabulary.WithDescription("Input state for execution"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(ExecutionOutput,
		vocabulary.WithDescription("Output state from execution"),
		vocabulary.WithDataType("string"))
}

// registerActionPredicates registers predicates for concrete actions.
func registerActionPredicates() {
	vocabulary.Register(ActionType,
		vocabulary.WithDescription("Category of the action"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(ActionExecutedBy,
		vocabulary.WithDescription("Agent that executed the action"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(ActionProduced,
		vocabulary.WithDescription("Artifact produced by the action"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriArtifact))

	vocabulary.Register(ActionContext,
		vocabulary.WithDescription("Execution context for the action"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(ActionTrace,
		vocabulary.WithDescription("Trace or audit record for the action"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(IriTraceEvent))
}

// registerTaskPredicates registers predicates for task management.
func registerTaskPredicates() {
	vocabulary.Register(TaskAssigned,
		vocabulary.WithDescription("Agent assigned to the task"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(TaskCapability,
		vocabulary.WithDescription("Capability required for the task"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(TaskSubtask,
		vocabulary.WithDescription("Child task in hierarchical decomposition"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(TaskDependency,
		vocabulary.WithDescription("Task that must complete before this one"),
		vocabulary.WithDataType("string"))

	vocabulary.Register(TaskStatus,
		vocabulary.WithDescription("Current status of the task"),
		vocabulary.WithDataType("string"))
}
