package agentic

// Agentic Vocabulary Predicates
//
// These predicates use the SemStreams three-level dotted notation for NATS
// compatibility while mapping to W3C S-Agent-Comm IRIs for standards compliance.
//
// Domain: "agent" - All agentic predicates share this domain prefix.
//
// Categories:
//   - intent: Goals and objectives an agent aims to achieve
//   - capability: Abilities an agent has to perform actions
//   - delegation: Authority transfer between agents
//   - accountability: Responsibility tracking and compliance
//   - execution: Runtime environment and constraints
//   - action: Concrete execution events
//   - task: Work units exchanged between agents

// Intent Predicates
//
// Express what an agent aims to achieve, including goals, parameters, and
// the authorization chain for the intent.
const (
	// IntentGoal is the objective or goal statement an agent aims to achieve.
	// Example: "analyze customer feedback and identify emerging themes"
	// DataType: string
	// IRI: agent-ontology:Intent
	IntentGoal = "agent.intent.goal"

	// IntentType is the category or classification of the intent.
	// Example: "data-analysis", "content-generation", "decision-support"
	// DataType: string
	// IRI: agent-ontology:hasIntentType
	IntentType = "agent.intent.type"

	// IntentParameter is a typed parameter for the intent.
	// Example: "input_dataset=customer_reviews_2024"
	// DataType: string
	// IRI: agent-ontology:hasParameter
	IntentParameter = "agent.intent.parameter"

	// IntentAuthorized is the delegation authorizing this intent.
	// Example: entity ID of the delegation that permits this intent
	// DataType: string (entity ID)
	// IRI: agent-ontology:authorizedBy
	IntentAuthorized = "agent.intent.authorized"

	// IntentProduces is the action produced by this intent.
	// Example: entity ID of the resulting action
	// DataType: string (entity ID)
	// IRI: agent-ontology:producesAction
	IntentProduces = "agent.intent.produces"
)

// Capability Predicates
//
// Express what an agent can do, including skills, confidence levels,
// constraints, and required permissions.
const (
	// CapabilityName is the identifier for a capability.
	// Example: "text-summarization", "code-review", "data-visualization"
	// DataType: string
	// IRI: agent-ontology:Capability
	CapabilityName = "agent.capability.name"

	// CapabilityDescription is a human-readable description of the capability.
	// Example: "Summarizes long documents while preserving key information"
	// DataType: string
	CapabilityDescription = "agent.capability.description"

	// CapabilityExpression is a semantic fingerprint for capability matching.
	// Used for embedding-based capability discovery and matching.
	// Example: "analyze text extract themes identify patterns"
	// DataType: string
	// IRI: agent-ontology:capabilityExpression
	CapabilityExpression = "agent.capability.expression"

	// CapabilityConfidence is the agent's self-assessed confidence (0.0-1.0).
	// Example: 0.95 (high confidence in this capability)
	// DataType: float64
	// Range: 0-1
	// IRI: agent-ontology:capabilityConfidence
	CapabilityConfidence = "agent.capability.confidence"

	// CapabilitySkill is an atomic skill implementing the capability.
	// Example: entity ID of a specific skill
	// DataType: string (entity ID)
	// IRI: agent-ontology:hasSkill
	CapabilitySkill = "agent.capability.skill"

	// CapabilityConstraint is an execution constraint on the capability.
	// Example: "max_tokens=4096", "requires_gpu=true"
	// DataType: string
	// IRI: agent-ontology:CapabilityConstraint
	CapabilityConstraint = "agent.capability.constraint"

	// CapabilityPermission is a required permission for the capability.
	// Example: "file_system_read", "network_access", "tool_execution"
	// DataType: string
	// IRI: agent-ontology:requiresPermission
	CapabilityPermission = "agent.capability.permission"
)

// Delegation Predicates
//
// Express authority transfer between agents, including scope, validity,
// and delegation chains.
const (
	// DelegationFrom is the agent granting delegated authority.
	// Example: entity ID of the delegating agent
	// DataType: string (entity ID)
	// IRI: agent-ontology:delegatedBy
	// InverseOf: DelegationTo
	DelegationFrom = "agent.delegation.from"

	// DelegationTo is the agent receiving delegated authority.
	// Example: entity ID of the delegate agent
	// DataType: string (entity ID)
	// IRI: agent-ontology:delegatesTo
	// InverseOf: DelegationFrom
	DelegationTo = "agent.delegation.to"

	// DelegationScope is the boundary of delegated authority.
	// Example: "repository:acme/project", "domain:customer-service"
	// DataType: string
	// IRI: agent-ontology:DelegationScope
	DelegationScope = "agent.delegation.scope"

	// DelegationCapability is a capability allowed by the delegation.
	// Example: entity ID of an allowed capability
	// DataType: string (entity ID)
	// IRI: agent-ontology:allowedCapability
	DelegationCapability = "agent.delegation.capability"

	// DelegationValidFrom is when the delegation becomes valid.
	// Example: "2024-01-15T09:00:00Z"
	// DataType: time.Time
	// IRI: agent-ontology:validFrom
	DelegationValidFrom = "agent.delegation.valid_from"

	// DelegationValidUntil is when the delegation expires.
	// Example: "2024-12-31T23:59:59Z"
	// DataType: time.Time
	// IRI: agent-ontology:validUntil
	DelegationValidUntil = "agent.delegation.valid_until"

	// DelegationChain is a multi-level delegation chain.
	// Example: entity ID of the delegation chain
	// DataType: string (entity ID)
	// IRI: agent-ontology:DelegationChain
	DelegationChain = "agent.delegation.chain"
)

// Accountability Predicates
//
// Express responsibility attribution, compliance assessment, and audit trails
// for agent actions.
const (
	// AccountabilityActor is the agent performing an accountable action.
	// Example: entity ID of the acting agent
	// DataType: string (entity ID)
	// IRI: agent-ontology:actor
	AccountabilityActor = "agent.accountability.actor"

	// AccountabilityAction is the action being accounted for.
	// Example: entity ID of the action
	// DataType: string (entity ID)
	AccountabilityAction = "agent.accountability.action"

	// AccountabilityAssigned is the party assigned responsibility.
	// Example: entity ID of the responsible agent or person
	// DataType: string (entity ID)
	// IRI: agent-ontology:assignedTo
	AccountabilityAssigned = "agent.accountability.assigned"

	// AccountabilityRationale is the reasoning for the attribution.
	// Example: "Agent executed action under delegated authority from user"
	// DataType: string
	// IRI: agent-ontology:rationale
	AccountabilityRationale = "agent.accountability.rationale"

	// AccountabilityCompliance is the compliance assessment result.
	// Example: "compliant", "non-compliant", "pending-review"
	// DataType: string
	// IRI: agent-ontology:ComplianceAssessment
	AccountabilityCompliance = "agent.accountability.compliance"

	// AccountabilityTimestamp is when the accountability event occurred.
	// Example: "2024-06-15T14:30:00Z"
	// DataType: time.Time
	AccountabilityTimestamp = "agent.accountability.timestamp"
)

// Execution Context Predicates
//
// Express runtime environment, security context, and resource constraints
// for agent execution.
const (
	// ExecutionEnvironment is the runtime environment type.
	// Example: "sandbox", "container", "bare-metal", "cloud-function"
	// DataType: string
	// IRI: agent-ontology:ExecutionEnvironment
	ExecutionEnvironment = "agent.execution.environment"

	// ExecutionSecurity is the security context for execution.
	// Example: "restricted", "elevated", "system"
	// DataType: string
	// IRI: agent-ontology:SecurityContext
	ExecutionSecurity = "agent.execution.security"

	// ExecutionConstraint is a resource constraint for execution.
	// Example: "memory_limit=1GB", "cpu_limit=2cores"
	// DataType: string
	// IRI: agent-ontology:ResourceConstraint
	ExecutionConstraint = "agent.execution.constraint"

	// ExecutionRateLimit is a rate limiting constraint.
	// Example: "100/minute", "1000/hour"
	// DataType: string
	// IRI: agent-ontology:RateLimit
	ExecutionRateLimit = "agent.execution.rate_limit"

	// ExecutionBudget is a cost or resource budget.
	// Example: "tokens=100000", "cost_usd=10.00"
	// DataType: string
	// IRI: agent-ontology:Budget
	ExecutionBudget = "agent.execution.budget"

	// ExecutionInput is the input state for execution.
	// Example: entity ID of the input artifact or state
	// DataType: string (entity ID)
	ExecutionInput = "agent.execution.input"

	// ExecutionOutput is the output state from execution.
	// Example: entity ID of the output artifact or state
	// DataType: string (entity ID)
	ExecutionOutput = "agent.execution.output"
)

// Action Predicates
//
// Express concrete execution events, including the executing agent,
// produced artifacts, and trace records.
const (
	// ActionType is the category of the action.
	// Example: "tool-call", "api-request", "file-write", "decision"
	// DataType: string
	ActionType = "agent.action.type"

	// ActionExecutedBy is the agent that executed the action.
	// Example: entity ID of the executing agent
	// DataType: string (entity ID)
	ActionExecutedBy = "agent.action.executed_by"

	// ActionProduced is an artifact produced by the action.
	// Example: entity ID of the produced artifact
	// DataType: string (entity ID)
	// IRI: agent-ontology:Artifact
	ActionProduced = "agent.action.produced"

	// ActionContext is the execution context for the action.
	// Example: entity ID of the execution context
	// DataType: string (entity ID)
	ActionContext = "agent.action.context"

	// ActionTrace is a trace or audit record for the action.
	// Example: entity ID of the trace event
	// DataType: string (entity ID)
	// IRI: agent-ontology:TraceEvent
	ActionTrace = "agent.action.trace"
)

// Task Predicates
//
// Express work units exchanged between agents, including assignment,
// capability requirements, dependencies, and status.
const (
	// TaskAssigned is the agent assigned to the task.
	// Example: entity ID of the assigned agent
	// DataType: string (entity ID)
	TaskAssigned = "agent.task.assigned"

	// TaskCapability is a capability required for the task.
	// Example: entity ID of the required capability
	// DataType: string (entity ID)
	TaskCapability = "agent.task.capability"

	// TaskSubtask is a child task in hierarchical decomposition.
	// Example: entity ID of the subtask
	// DataType: string (entity ID)
	TaskSubtask = "agent.task.subtask"

	// TaskDependency is a task that must complete before this one.
	// Example: entity ID of the dependency task
	// DataType: string (entity ID)
	TaskDependency = "agent.task.dependency"

	// TaskStatus is the current status of the task.
	// Example: "pending", "in_progress", "completed", "failed", "cancelled"
	// DataType: string
	TaskStatus = "agent.task.status"
)

// Identity Predicates
//
// Express DID-based cryptographic identity for agents, including
// decentralized identifiers, verifiable credentials, and issuers.
const (
	// IdentityDID is the decentralized identifier for an agent.
	// Example: "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK"
	// DataType: string
	// IRI: agent-ontology:Identity
	IdentityDID = "agent.identity.did"

	// IdentityCredential is a verifiable credential held by the agent.
	// Example: entity ID of the credential
	// DataType: string (entity ID)
	// IRI: agent-ontology:hasCredential
	IdentityCredential = "agent.identity.credential"

	// IdentityIssuer is the DID of an entity that issued a credential.
	// Example: "did:key:z6MkIssuer..."
	// DataType: string
	// IRI: agent-ontology:issuedBy
	IdentityIssuer = "agent.identity.issuer"

	// IdentityVerified indicates if the identity has been verified.
	// Example: true
	// DataType: bool
	// IRI: agent-ontology:verified
	IdentityVerified = "agent.identity.verified"

	// IdentityDisplayName is the human-readable name for the agent.
	// Example: "Code Review Agent"
	// DataType: string
	IdentityDisplayName = "agent.identity.display_name"

	// IdentityRole is the agent's role in the system.
	// Example: "architect", "editor", "reviewer"
	// DataType: string
	IdentityRole = "agent.identity.role"
)
