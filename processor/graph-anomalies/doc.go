// Package graphanomalies provides the graph-anomalies component for detecting
// cluster and community anomalies in the knowledge graph.
//
// # Overview
//
// The graph-anomalies component detects anomalies within clusters and communities
// using k-core decomposition and pivot-based distance indexing. These metrics
// identify unusual patterns, outliers, and structural anomalies in graph communities.
//
// # Tier
//
// Tier: STATISTICAL (Tier 1) - Required for statistical and semantic tiers.
// This component depends on graph-embedding and graph-clustering results.
//
// # Component Interface
//
// This component implements the semstreams component framework:
//   - component.Discoverable (6 methods): Meta, InputPorts, OutputPorts,
//     ConfigSchema, Health, DataFlow
//   - component.LifecycleComponent (3 methods): Initialize, Start, Stop
//
// # Communication Patterns
//
// Inputs:
//   - KV watch on OUTGOING_INDEX: Triggers recomputation on graph changes
//   - KV watch on INCOMING_INDEX: Provides incoming edge information
//
// Outputs:
//   - KV bucket STRUCTURAL_INDEX: Stores k-core numbers and pivot distances
//
// # Anomaly Detection Metrics
//
// K-Core Decomposition:
//   - Computes the k-core number for each node
//   - Higher k-core indicates more densely connected subgraph
//   - Nodes with unusual k-core values relative to their community are flagged
//
// Pivot-Based Distance Indexing:
//   - Selects pivot nodes for efficient distance approximation
//   - Computes distances from each node to pivot nodes
//   - Identifies nodes with unusual distance patterns (potential anomalies)
//
// # Configuration
//
// Key configuration options:
//   - compute_interval: Time between recomputation cycles (default: 1h)
//   - pivot_count: Number of pivot nodes for distance indexing (default: 16)
//   - max_hop_distance: Maximum BFS traversal depth (default: 10)
//   - compute_on_startup: Whether to compute immediately on start (default: true)
//
// # Tiered Deployment
//
// The graph-anomalies component is used in Statistical (Tier 1) and Semantic
// (Tier 2) deployments to detect anomalies in clustered communities.
// It is NOT used in Structural (Tier 0) deployments.
//
// # Usage
//
//	// Register the component
//	registry := component.NewRegistry()
//	graphanomalies.Register(registry)
//
//	// Create via factory
//	comp, err := graphanomalies.CreateGraphAnomalies(configJSON, deps)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Lifecycle management
//	comp.Initialize()
//	comp.Start(ctx)
//	defer comp.Stop(5 * time.Second)
package graphanomalies
