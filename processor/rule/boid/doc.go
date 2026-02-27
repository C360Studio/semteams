// Package boid implements Boids-inspired local coordination rules for multi-agent teams.
//
// This package provides three local rules based on Craig Reynolds' Boids simulation (1986):
//
//   - Cohesion: Steer toward high-centrality nodes in the active subgraph matching the role's objective
//   - Alignment: Match traversal direction of same-role agents
//   - Separation: Avoid overlapping k-hop neighborhoods with other agents
//
// # Architecture
//
// Agent positions are tracked in the AGENT_POSITIONS KV bucket. Rules watch this bucket
// and evaluate local coordination signals based on graph topology. Signals are published
// to agent.boid.<loopID> subjects for consumption by the agentic-loop.
//
// # Graph Topology as Coordination Substrate
//
// Unlike spatial Boids, which use Euclidean distance, these rules use the knowledge graph
// topology directly:
//
//   - Proximity: k-hop distance via PivotIndex.IsWithinHops()
//   - Center of mass: PageRank centrality over active subgraph
//   - Heading: Traversal direction (relationship types being followed)
//   - Flock boundaries: Community membership
//
// # Configuration
//
// Rules are configured via JSON with role-specific thresholds:
//
//	{
//	  "type": "boid",
//	  "metadata": {
//	    "boid_rule": "separation",
//	    "role_thresholds": {
//	      "general": 2,
//	      "architect": 3
//	    }
//	  }
//	}
//
// # Research Context
//
// This is an experimental implementation for validating the hypothesis that explicit
// workflow choreography can be replaced by intent-weighted local rules for certain
// agent team configurations. See docs/research/boids-hypothesis.md for details.
package boid
