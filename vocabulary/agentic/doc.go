// Package agentic provides vocabulary constants for AI agent interoperability.
//
// This package defines predicates and IRIs aligned with the W3C Semantic Agent
// Communication Community Group (SAC-CG) ontology for expressing agent behaviors,
// capabilities, delegations, and accountability in knowledge graphs.
//
// # Overview
//
// The agentic vocabulary covers key concepts for multi-agent systems:
//
//   - Intent: What an agent aims to achieve (goals, parameters, authorization)
//   - Capability: What an agent can do (skills, constraints, permissions)
//   - Delegation: Authority transfer between agents (scope, chain, validity)
//   - Accountability: Responsibility attribution (actor, rationale, compliance)
//   - Execution: Runtime environment (security, constraints, budgets)
//   - Action: Concrete execution events (traces, artifacts)
//   - Task: Work units exchanged between agents (assignment, dependencies)
//
// # Design Philosophy
//
// SemStreams uses a "pragmatic semantic web" approach:
//
//   - Internal predicates use dotted notation (agent.intent.goal) for NATS compatibility
//   - IRI constants map to W3C S-Agent-Comm ontology for standards compliance
//   - Registration links both, enabling internal processing and external export
//
// # W3C S-Agent-Comm Alignment
//
// The predicates map to the S-Agent-Comm ontology modules:
//
//   - core: Agent, Action, Intent, Capability, Delegation, ExecutionContext
//   - intent: IntentType, IntentParameter, ActionType
//   - capability: Skill, CapabilityConstraint
//   - delegation: DelegationScope, DelegationChain
//   - accountability: AccountabilityEvent, ResponsibilityAttribution
//   - execution-context: ExecutionEnvironment, SecurityContext
//
// # Usage
//
// Use predicates directly in triples:
//
//	import "github.com/c360studio/semstreams/vocabulary/agentic"
//
//	// Express an agent's intent
//	triple := message.Triple{
//	    Subject:   agentID,
//	    Predicate: agentic.IntentGoal,
//	    Object:    "analyze customer feedback and summarize themes",
//	}
//
//	// Express a capability with confidence
//	triple := message.Triple{
//	    Subject:   agentID,
//	    Predicate: agentic.CapabilityConfidence,
//	    Object:    0.95,
//	}
//
// Register predicates with IRI mappings for export:
//
//	agentic.Register()  // Registers all predicates with S-Agent-Comm IRIs
//
//	meta := vocabulary.GetPredicateMetadata(agentic.IntentGoal)
//	// meta.StandardIRI == agentic.IriIntent
//
// # Conceptual Alignments
//
// S-Agent-Comm concepts align with existing semantic web standards. While the
// primary IRI mappings use S-Agent-Comm, these conceptual equivalences enable
// reasoning across ontologies:
//
//   - agent.intent.goal ≈ prov:Plan (PROV-O) - both represent planned objectives
//   - agent.capability.name ≈ cco:Capability (CCO) - both represent agent abilities
//   - agent.delegation.from ≈ prov:actedOnBehalfOf (PROV-O) - both express delegation
//   - agent.accountability.actor ≈ prov:wasAssociatedWith (PROV-O) - both link agents to actions
//
// # References
//
//   - W3C SAC-CG: https://www.w3.org/community/sac-cg/
//   - S-Agent-Comm Ontology: https://w3id.org/agent-ontology/
//   - PROV-O: https://www.w3.org/TR/prov-o/
//   - CCO: https://github.com/CommonCoreOntology/CommonCoreOntologies
package agentic
