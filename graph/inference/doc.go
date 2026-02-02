// Package inference provides structural anomaly detection for identifying potential
// missing relationships in the knowledge graph.
//
// # Overview
//
// The inference system analyzes the gap between semantic similarity and structural
// distance to identify entities that should probably be related but aren't explicitly
// connected. It supports four anomaly detection methods:
//
//   - Semantic-Structural Gap: High embedding similarity with high graph distance
//   - Core Isolation: Hub entities with few same-core peer connections
//   - Core Demotion: Entities that dropped k-core level between computations
//   - Transitivity Gap: Missing transitive relationships (A→B→C exists but A-C distant)
//
// Detected anomalies flow through an approval pipeline:
//
//	Detectors → Pending → LLM Review → Human Review (optional) → Applied/Rejected
//
// # Architecture
//
//	Community Detection Trigger
//	           ↓
//	┌──────────────────────────────────────────────┐
//	│              Anomaly Detectors               │
//	├──────────┬──────────┬──────────┬─────────────┤
//	│ Semantic │   Core   │   Core   │Transitivity │
//	│   Gap    │ Isolation│ Demotion │    Gap      │
//	└──────────┴──────────┴──────────┴─────────────┘
//	           ↓
//	    ANOMALY_INDEX KV
//	           ↓
//	    ┌─────────────┐
//	    │ Review Worker│ ←── LLM API
//	    └─────────────┘
//	           ↓
//	    ┌─────────────┐
//	    │Human Review │ ←── HTTP Handlers
//	    │   Queue     │
//	    └─────────────┘
//	           ↓
//	    Edge Applier → Graph
//
// # Usage
//
// Configure and run the anomaly detector:
//
//	cfg := inference.DefaultConfig()
//	cfg.Enabled = true
//	cfg.SemanticGap.Enabled = true
//	cfg.Review.Enabled = true
//	cfg.Review.LLM = llm.Config{
//	    Provider: "openai",
//	    BaseURL:  "https://api.openai.com/v1",
//	}
//
//	detector, err := inference.NewDetector(cfg, deps)
//
//	// Run detection (typically triggered after community detection)
//	anomalies, err := detector.DetectAnomalies(ctx)
//
// Query and manage anomalies via HTTP handlers:
//
//	// List pending anomalies
//	GET /api/v1/anomalies?status=pending
//
//	// Approve an anomaly
//	POST /api/v1/anomalies/{id}/approve
//
//	// Reject an anomaly
//	POST /api/v1/anomalies/{id}/reject
//
// # Anomaly Types
//
// Semantic-Structural Gap ([AnomalySemanticStructuralGap]):
//
//	Detected when entities have high embedding similarity (≥0.7) but high
//	structural distance (≥3 hops). Evidence includes similarity score and
//	distance bounds from triangle inequality.
//
// Core Isolation ([AnomalyCoreIsolation]):
//
//	Detected when a high k-core entity has fewer same-core connections than
//	expected. Evidence includes core level, peer count, and connectivity ratio.
//
// Core Demotion ([AnomalyCoreDemotion]):
//
//	Detected when an entity drops k-core level between computations,
//	signaling lost relationships. Evidence includes previous/current levels.
//
// Transitivity Gap ([AnomalyTransitivityGap]):
//
//	Detected when A→B and B→C exist but A-C distance exceeds expected
//	for configured transitive predicates. Evidence includes the chain path.
//
// # Configuration
//
// Key configuration options:
//
//	Enabled:                   false      # Opt-in feature
//	RunWithCommunityDetection: true       # Trigger after community detection
//	MaxAnomaliesPerRun:        100        # Prevent runaway detection
//
//	SemanticGap:
//	  Enabled:               true
//	  MinSemanticSimilarity: 0.7          # Minimum embedding similarity
//	  MinStructuralDistance: 3            # Minimum graph distance (hops)
//	  MaxGapsPerEntity:      5            # Limit per entity
//
//	CoreAnomaly:
//	  Enabled:               true
//	  MinCoreForHubAnalysis: 3            # Minimum k-core for hub analysis
//	  HubIsolationThreshold: 0.5          # Peer connectivity ratio threshold
//	  TrackCoreDemotions:    true
//	  MinDemotionDelta:      2            # Core level drop to flag
//
//	Transitivity:
//	  Enabled:                 true
//	  TransitivePredicates:    ["member_of", "part_of", "located_in"]
//
//	Review:
//	  Enabled:              false         # Requires LLM setup
//	  AutoApproveThreshold: 0.9           # Auto-approve confidence
//	  AutoRejectThreshold:  0.3           # Auto-reject confidence
//	  FallbackToHuman:      true          # Escalate uncertain cases
//
//	VirtualEdges:
//	  AutoApply:
//	    Enabled:           false          # Auto-create edges
//	    MinConfidence:     0.95           # Confidence threshold for auto-apply
//	    PredicateTemplate: "inferred.semantic.{band}"
//	  ReviewQueue:
//	    Enabled:           false          # Queue uncertain gaps for review
//	    MinConfidence:     0.7            # Lower bound for review queue
//	    MaxConfidence:     0.95           # Upper bound (below auto-apply)
//
// # Anomaly Lifecycle
//
// Each anomaly progresses through states defined by [AnomalyStatus]:
//
//	StatusPending → StatusLLMReviewing → StatusLLMApproved/StatusLLMRejected
//	                                            ↓
//	                                   StatusHumanReview (if uncertain)
//	                                            ↓
//	                                   StatusApproved/StatusRejected
//	                                            ↓
//	                                   StatusApplied (edge created)
//
// High-confidence anomalies may skip review with StatusAutoApplied.
//
// # Thread Safety
//
// The Detector and Storage types are safe for concurrent use. HTTP handlers
// use optimistic locking via revision numbers when updating anomaly status.
//
// # Metrics
//
// The inference package exports Prometheus metrics:
//   - anomalies_detected_total: Anomalies detected by type
//   - anomalies_reviewed_total: Anomalies reviewed by decision
//   - anomalies_applied_total: Anomalies applied to graph
//   - detection_duration_seconds: Detection run duration
//   - review_duration_seconds: LLM review duration
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semstreams/graph/clustering]: Community detection that triggers inference
//   - [github.com/c360studio/semstreams/graph/embedding]: Embedding similarity for semantic gaps
//   - [github.com/c360studio/semstreams/graph/llm]: LLM integration for review pipeline
package inference
