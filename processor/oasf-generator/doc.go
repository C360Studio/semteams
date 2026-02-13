// Package oasfgenerator provides an OASF (Open Agent Specification Framework) record generator
// processor component for SemStreams.
//
// # Overview
//
// The OASF generator watches for agent entity changes in the ENTITY_STATES KV bucket and
// generates OASF records that can be used for AGNTCY directory registration. OASF is the
// standard format for describing agent capabilities in the AGNTCY (Internet of Agents) ecosystem.
//
// # NATS Topology
//
// The component uses the following NATS resources:
//
//	Input:
//	  - ENTITY_STATES KV Watch: Monitors for agent entity changes
//	  - oasf.generate.request: On-demand generation requests (optional)
//
//	Output:
//	  - OASF_RECORDS KV Write: Stores generated OASF records
//	  - oasf.record.generated.*: Publishes generation events
//
// # Predicate Mapping
//
// The generator maps SemStreams predicates to OASF fields:
//
//	SemStreams Predicate         -> OASF Field
//	agent.capability.name        -> skills[].name
//	agent.capability.description -> skills[].description
//	agent.capability.expression  -> skills[].id
//	agent.capability.confidence  -> skills[].confidence
//	agent.capability.permission  -> skills[].permissions[]
//	agent.intent.goal            -> description
//	agent.intent.type            -> domains[].name
//	agent.action.type            -> extensions.action_types[]
//
// # Configuration
//
// Example configuration:
//
//	{
//	  "entity_kv_bucket": "ENTITY_STATES",
//	  "oasf_kv_bucket": "OASF_RECORDS",
//	  "watch_pattern": "*.agent.*",
//	  "generation_debounce": "1s",
//	  "default_agent_version": "1.0.0",
//	  "include_extensions": true
//	}
//
// # Usage
//
// The component is typically used as part of a flow configuration:
//
//	components:
//	  - name: oasf-gen
//	    type: oasf-generator
//	    config:
//	      entity_kv_bucket: ENTITY_STATES
//	      oasf_kv_bucket: OASF_RECORDS
//
// The generated OASF records can then be used by the directory-bridge component
// to register agents with AGNTCY directories.
//
// # OASF Record Structure
//
// Generated records follow the OASF schema:
//
//	{
//	  "name": "agent-architect",
//	  "version": "1.0.0",
//	  "schema_version": "1.0.0",
//	  "authors": ["system"],
//	  "created_at": "2024-01-15T10:30:00Z",
//	  "description": "Designs software architecture",
//	  "skills": [
//	    {
//	      "id": "software-design",
//	      "name": "Software Design",
//	      "description": "Creates software architecture diagrams",
//	      "confidence": 0.95,
//	      "permissions": ["file_system_read"]
//	    }
//	  ],
//	  "domains": [
//	    {
//	      "name": "software-architecture"
//	    }
//	  ],
//	  "extensions": {
//	    "semstreams_entity_id": "acme.ops.agentic.system.agent.architect",
//	    "source": "semstreams"
//	  }
//	}
//
// # See Also
//
// - ADR-019: AGNTCY Integration
// - docs/concepts/20-oasf-integration.md
// - https://docs.agntcy.org/pages/syntaxes/oasf
package oasfgenerator
