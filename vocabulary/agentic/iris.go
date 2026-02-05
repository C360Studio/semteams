package agentic

// W3C S-Agent-Comm Ontology Namespaces
//
// The S-Agent-Comm ontology provides a standardized vocabulary for AI agent
// interoperability, covering core concepts, intents, capabilities, delegation,
// accountability, and execution context.
const (
	// CoreNamespace is the base IRI for core agent concepts.
	CoreNamespace = "https://w3id.org/agent-ontology/core#"

	// IntentNamespace is the base IRI for intent-related concepts.
	IntentNamespace = "https://w3id.org/agent-ontology/intent#"

	// CapabilityNamespace is the base IRI for capability-related concepts.
	CapabilityNamespace = "https://w3id.org/agent-ontology/capability#"

	// DelegationNamespace is the base IRI for delegation-related concepts.
	DelegationNamespace = "https://w3id.org/agent-ontology/delegation#"

	// AccountabilityNamespace is the base IRI for accountability-related concepts.
	AccountabilityNamespace = "https://w3id.org/agent-ontology/accountability#"

	// ExecutionNamespace is the base IRI for execution context concepts.
	ExecutionNamespace = "https://w3id.org/agent-ontology/execution-context#"
)

// Core Classes
//
// Fundamental agent ontology classes representing the primary entities
// in multi-agent systems.
const (
	// IriAgent represents an autonomous entity capable of perception and action.
	IriAgent = CoreNamespace + "Agent"

	// IriAction represents a concrete activity performed by an agent.
	IriAction = CoreNamespace + "Action"

	// IriIntent represents a goal or objective an agent aims to achieve.
	IriIntent = CoreNamespace + "Intent"

	// IriCapability represents an ability an agent has to perform actions.
	IriCapability = CoreNamespace + "Capability"

	// IriDelegation represents an authority transfer between agents.
	IriDelegation = CoreNamespace + "Delegation"

	// IriExecutionContext represents the runtime environment for agent actions.
	IriExecutionContext = CoreNamespace + "ExecutionContext"

	// IriTask represents a unit of work assigned to or performed by an agent.
	IriTask = CoreNamespace + "Task"

	// IriArtifact represents an output or result produced by an agent action.
	IriArtifact = CoreNamespace + "Artifact"

	// IriTraceEvent represents an audit event in agent execution history.
	IriTraceEvent = CoreNamespace + "TraceEvent"
)

// Intent Classes & Properties
//
// Classes and properties for expressing agent intentions, goals,
// and the actions they produce.
const (
	// IriIntentType represents the category or classification of an intent.
	IriIntentType = IntentNamespace + "IntentType"

	// IriIntentParameter represents a typed parameter for an intent.
	IriIntentParameter = IntentNamespace + "IntentParameter"

	// IriHasIntentType relates an intent to its type classification.
	// Domain: Intent, Range: IntentType
	IriHasIntentType = IntentNamespace + "hasIntentType"

	// IriHasParameter relates an intent to its parameters.
	// Domain: Intent, Range: IntentParameter
	IriHasParameter = IntentNamespace + "hasParameter"

	// IriAuthorizedBy relates an intent to the delegation that authorizes it.
	// Domain: Intent, Range: Delegation
	IriAuthorizedBy = IntentNamespace + "authorizedBy"

	// IriProducesAction relates an intent to the action it produces.
	// Domain: Intent, Range: Action
	IriProducesAction = IntentNamespace + "producesAction"
)

// Capability Classes & Properties
//
// Classes and properties for expressing what agents can do,
// including skills, constraints, and permissions.
const (
	// IriSkill represents an atomic, executable ability.
	IriSkill = CapabilityNamespace + "Skill"

	// IriCapabilityConstraint represents a limitation or condition on capability use.
	IriCapabilityConstraint = CapabilityNamespace + "CapabilityConstraint"

	// IriHasSkill relates a capability to its implementing skills.
	// Domain: Capability, Range: Skill
	IriHasSkill = CapabilityNamespace + "hasSkill"

	// IriCapabilityExpression provides a semantic fingerprint for capability matching.
	// Domain: Capability, Range: xsd:string
	IriCapabilityExpression = CapabilityNamespace + "capabilityExpression"

	// IriCapabilityConfidence represents self-assessed confidence in a capability.
	// Domain: Capability, Range: xsd:decimal (0.0-1.0)
	IriCapabilityConfidence = CapabilityNamespace + "capabilityConfidence"

	// IriRequiresPermission relates a capability to required permissions.
	// Domain: Capability, Range: Permission
	IriRequiresPermission = CapabilityNamespace + "requiresPermission"
)

// Delegation Classes & Properties
//
// Classes and properties for expressing authority transfer between agents,
// including scope, chains, and validity periods.
const (
	// IriDelegationScope represents the boundary of delegated authority.
	IriDelegationScope = DelegationNamespace + "DelegationScope"

	// IriDelegationChain represents a sequence of delegations forming a trust path.
	IriDelegationChain = DelegationNamespace + "DelegationChain"

	// IriDelegatedBy relates a delegation to the granting agent.
	// Domain: Delegation, Range: Agent
	IriDelegatedBy = DelegationNamespace + "delegatedBy"

	// IriDelegatesTo relates a delegation to the receiving agent.
	// Domain: Delegation, Range: Agent
	IriDelegatesTo = DelegationNamespace + "delegatesTo"

	// IriAllowedCapability relates a delegation to capabilities it permits.
	// Domain: Delegation, Range: Capability
	IriAllowedCapability = DelegationNamespace + "allowedCapability"

	// IriValidFrom specifies when a delegation becomes valid.
	// Domain: Delegation, Range: xsd:dateTime
	IriValidFrom = DelegationNamespace + "validFrom"

	// IriValidUntil specifies when a delegation expires.
	// Domain: Delegation, Range: xsd:dateTime
	IriValidUntil = DelegationNamespace + "validUntil"
)

// Accountability Classes & Properties
//
// Classes and properties for expressing responsibility attribution,
// compliance assessment, and audit trails.
const (
	// IriAccountabilityEvent represents a recordable event for accountability purposes.
	IriAccountabilityEvent = AccountabilityNamespace + "AccountabilityEvent"

	// IriResponsibilityAttribution represents assignment of responsibility for an action.
	IriResponsibilityAttribution = AccountabilityNamespace + "ResponsibilityAttribution"

	// IriComplianceAssessment represents evaluation of action compliance with rules.
	IriComplianceAssessment = AccountabilityNamespace + "ComplianceAssessment"

	// IriActor relates an accountability event to the performing agent.
	// Domain: AccountabilityEvent, Range: Agent
	IriActor = AccountabilityNamespace + "actor"

	// IriAssignedTo relates responsibility to the accountable party.
	// Domain: ResponsibilityAttribution, Range: Agent
	IriAssignedTo = AccountabilityNamespace + "assignedTo"

	// IriRationale provides the reasoning for an attribution or assessment.
	// Domain: ResponsibilityAttribution|ComplianceAssessment, Range: xsd:string
	IriRationale = AccountabilityNamespace + "rationale"
)

// Execution Context Classes & Properties
//
// Classes and properties for expressing runtime environment, security context,
// and resource constraints for agent execution.
const (
	// IriExecutionEnvironment represents the runtime environment type.
	IriExecutionEnvironment = ExecutionNamespace + "ExecutionEnvironment"

	// IriSecurityContext represents the security settings for execution.
	IriSecurityContext = ExecutionNamespace + "SecurityContext"

	// IriResourceConstraint represents a limitation on resource usage.
	IriResourceConstraint = ExecutionNamespace + "ResourceConstraint"

	// IriRateLimit represents a constraint on execution frequency.
	IriRateLimit = ExecutionNamespace + "RateLimit"

	// IriBudget represents a cost or resource budget for execution.
	IriBudget = ExecutionNamespace + "Budget"
)
