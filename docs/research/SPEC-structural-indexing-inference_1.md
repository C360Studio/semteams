# Specification: Structural Indexing and Inference System

## Document Info

| Field | Value |
|-------|-------|
| **Feature** | Structural Indexing and Inference |
| **Version** | 1.0.0 |
| **Status** | Ready for Implementation |
| **Priority** | High |
| **Tier** | Tier 1 (Core) + Tier 2 (LLM Enhancement) |

## 1. Overview

### 1.1 Problem Statement

SemStreams currently lacks structural graph indexing capabilities that would enable:
1. **Query optimization**: Filtering noisy entities, estimating multi-hop distances
2. **Structural inference**: Detecting missing relationships from graph topology
3. **Anomaly detection**: Identifying structural inconsistencies for review

### 1.2 Solution Summary

Implement two structural indexing algorithms (k-core decomposition, pivot-based distance indexing) with shared code that serves both query optimization and inference detection. Add an LLM-assisted review pipeline for anomaly approval with human fallback.

### 1.3 Goals

- Compute k-core decomposition to identify graph backbone and filter noise
- Compute pivot-based distance index for O(1) multi-hop distance estimation
- Detect structural anomalies (semantic-structural gaps, core isolation, transitivity gaps)
- Provide LLM-assisted review with configurable auto-approve/reject thresholds
- Fallback to human review for uncertain cases
- Share index computation between query and inference paths
- Fit within existing Tier 1/2 progressive enhancement model

### 1.4 Non-Goals

- Real-time index updates (batch computation with community detection is acceptable)
- Graph partitioning or sharding
- Distributed index computation
- UI implementation (separate spec)

## 2. Architecture

### 2.1 Package Structure

```
processor/graph/
├── structuralindex/                 # NEW: Shared structural algorithms
│   ├── types.go                     # KCoreIndex, PivotIndex, StructuralIndices
│   ├── kcore.go                     # K-core computation
│   ├── pivot.go                     # Pivot-based distance indexing
│   ├── provider.go                  # StructuralGraphProvider wrapper
│   ├── storage.go                   # NATS KV persistence
│   ├── kcore_test.go
│   ├── pivot_test.go
│   └── README.md
├── inference/                       # NEW: Structural inference
│   ├── types.go                     # AnomalyType, AnomalyStatus, StructuralAnomaly
│   ├── detector.go                  # Orchestrator for all detectors
│   ├── semantic_gap.go              # Semantic-structural gap detector
│   ├── core_anomaly.go              # K-core based anomaly detection
│   ├── transitivity.go              # Transitivity gap detector
│   ├── review_worker.go             # LLM review worker (EnhancementWorker pattern)
│   ├── storage.go                   # ANOMALY_INDEX KV operations
│   ├── applier.go                   # Relationship application
│   ├── http_handlers.go             # Human review API endpoints
│   ├── detector_test.go
│   ├── review_worker_test.go
│   └── README.md
├── clustering/                      # EXISTING - will use structuralindex
├── indexmanager/                    # EXISTING - will use structuralindex for queries
├── querymanager/                    # EXISTING - will use structuralindex for PathRAG
└── processor.go                     # MODIFY - orchestrate new components
```

### 2.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Graph Processor                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Structural Index Subsystem                        │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐    ┌──────────────────┐                       │   │
│  │  │  KCoreComputer   │    │  PivotComputer   │                       │   │
│  │  │  - Peeling algo  │    │  - PageRank pivots│                      │   │
│  │  │  - Core buckets  │    │  - BFS distances │                       │   │
│  │  └────────┬─────────┘    └────────┬─────────┘                       │   │
│  │           │                       │                                  │   │
│  │           └───────────┬───────────┘                                  │   │
│  │                       ▼                                              │   │
│  │           ┌──────────────────────┐                                   │   │
│  │           │  StructuralIndices   │                                   │   │
│  │           │  - KCoreIndex        │                                   │   │
│  │           │  - PivotIndex        │                                   │   │
│  │           └──────────┬───────────┘                                   │   │
│  │                      │                                               │   │
│  │        ┌─────────────┼─────────────┐                                │   │
│  │        ▼             ▼             ▼                                │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────┐                       │   │
│  │  │ Query    │  │ Inference│  │ KV Storage   │                       │   │
│  │  │ Optimize │  │ Detect   │  │ STRUCTURAL_  │                       │   │
│  │  └──────────┘  └──────────┘  │ INDEX bucket │                       │   │
│  │                              └──────────────┘                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Inference Subsystem                               │   │
│  │                                                                      │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │                   Anomaly Detectors                          │    │   │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐               │    │   │
│  │  │  │ Semantic   │ │ Core       │ │Transitivity│               │    │   │
│  │  │  │ Structural │ │ Anomaly    │ │ Gap        │               │    │   │
│  │  │  │ Gap        │ │ Detector   │ │ Detector   │               │    │   │
│  │  │  └────────────┘ └────────────┘ └────────────┘               │    │   │
│  │  └──────────────────────────┬──────────────────────────────────┘    │   │
│  │                             ▼                                        │   │
│  │                 ┌──────────────────────┐                            │   │
│  │                 │  ANOMALY_INDEX KV    │                            │   │
│  │                 │  status: "pending"   │                            │   │
│  │                 └──────────┬───────────┘                            │   │
│  │                            │                                         │   │
│  │                            ▼                                         │   │
│  │                 ┌──────────────────────┐                            │   │
│  │                 │   ReviewWorker       │◄─── Tier 2 (LLM)           │   │
│  │                 │   - KV watcher       │                            │   │
│  │                 │   - LLM review       │                            │   │
│  │                 │   - Auto approve/rej │                            │   │
│  │                 └──────────┬───────────┘                            │   │
│  │                            │                                         │   │
│  │            ┌───────────────┼───────────────┐                        │   │
│  │            ▼               ▼               ▼                        │   │
│  │     ┌──────────┐    ┌──────────┐    ┌──────────┐                   │   │
│  │     │ Auto     │    │ Human    │    │ Rejected │                   │   │
│  │     │ Applied  │    │ Review   │    │          │                   │   │
│  │     └──────────┘    └──────────┘    └──────────┘                   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Data Flow

```
1. Community Detection Triggered (existing flow)
       │
       ▼
2. Structural Index Computation (NEW)
       │
       ├── K-Core Decomposition
       │     ├── Build degree map from GraphProvider
       │     ├── Peeling algorithm (remove lowest degree iteratively)
       │     └── Store core numbers + buckets
       │
       ├── Pivot Index Construction  
       │     ├── Select pivots via PageRank
       │     ├── BFS from each pivot
       │     └── Store distance vectors
       │
       └── Persist to STRUCTURAL_INDEX KV
       │
       ▼
3. Anomaly Detection (NEW)
       │
       ├── Semantic-Structural Gap Detection
       │     ├── Find semantically similar entities (embeddings)
       │     ├── Check structural distance (pivot index)
       │     └── Flag high-similarity + high-distance pairs
       │
       ├── Core Anomaly Detection
       │     ├── Hub isolation (high-core, low peer connectivity)
       │     └── Core demotion (dropped from previous computation)
       │
       ├── Transitivity Gap Detection
       │     ├── Find A→B→C paths
       │     └── Check if A-C distance > expected
       │
       └── Save anomalies to ANOMALY_INDEX (status: "pending")
       │
       ▼
4. LLM Review (Tier 2) (NEW)
       │
       ├── ReviewWorker watches ANOMALY_INDEX
       ├── Fetch entity context
       ├── Build review prompt
       ├── Call LLM
       │
       └── Decision routing:
             ├── confidence ≥ auto_approve_threshold → Apply relationship
             ├── confidence ≤ auto_reject_threshold → Mark rejected  
             └── else → Escalate to human review
       │
       ▼
5. Human Review (optional) (NEW)
       │
       ├── GET /inference/anomalies/pending
       ├── Review with LLM reasoning context
       └── POST /inference/anomalies/{id}/review
```

## 3. Type Definitions

### 3.1 Structural Index Types

```go
// processor/graph/structuralindex/types.go

package structuralindex

import "time"

// KCoreIndex stores k-core decomposition results
type KCoreIndex struct {
    // CoreNumbers maps entity ID to its core number
    // Core number k means entity has at least k neighbors also in core k
    CoreNumbers map[string]int `json:"core_numbers"`
    
    // MaxCore is the highest core number in the graph
    MaxCore int `json:"max_core"`
    
    // CoreBuckets groups entities by core number for efficient filtering
    // Key: core number, Value: entity IDs in that core
    CoreBuckets map[int][]string `json:"core_buckets"`
    
    // Metadata
    ComputedAt  time.Time `json:"computed_at"`
    EntityCount int       `json:"entity_count"`
}

// PivotIndex stores pivot-based distance vectors for O(1) distance estimation
type PivotIndex struct {
    // Pivots is the ordered list of pivot entity IDs (selected by PageRank)
    Pivots []string `json:"pivots"`
    
    // DistanceVectors maps entity ID to its distance vector
    // Vector[i] = shortest path distance to Pivots[i]
    // MaxHopDistance (255) indicates unreachable
    DistanceVectors map[string][]int `json:"distance_vectors"`
    
    // Metadata
    ComputedAt  time.Time `json:"computed_at"`
    EntityCount int       `json:"entity_count"`
}

// StructuralIndices bundles both indices
type StructuralIndices struct {
    KCore *KCoreIndex `json:"kcore,omitempty"`
    Pivot *PivotIndex `json:"pivot,omitempty"`
}

// Constants
const (
    MaxHopDistance    = 255 // Sentinel for unreachable nodes
    DefaultPivotCount = 16  // Typical pivot count (10-20 works well)
)
```

### 3.2 Inference Types

```go
// processor/graph/inference/types.go

package inference

import "time"

// AnomalyType classifies the structural inference signal
type AnomalyType string

const (
    // AnomalySemanticStructuralGap: High embedding similarity, high graph distance
    // Suggests these entities should probably be related
    AnomalySemanticStructuralGap AnomalyType = "semantic_structural_gap"
    
    // AnomalyCoreIsolation: High k-core entity with few same-core peer connections
    // Suggests missing hub relationships
    AnomalyCoreIsolation AnomalyType = "core_isolation"
    
    // AnomalyCoreDemotion: Entity dropped in k-core between computations
    // Suggests lost relationships or data quality issue
    AnomalyCoreDemotion AnomalyType = "core_demotion"
    
    // AnomalyTransitivityGap: A→B and B→C exist, but A-C distance > expected
    // Suggests missing transitive relationship
    AnomalyTransitivityGap AnomalyType = "transitivity_gap"
    
    // AnomalyCommunityBridge: Entity semantically spans multiple communities
    // Suggests potential bridge node needing explicit relationships
    AnomalyCommunityBridge AnomalyType = "community_bridge"
)

// AnomalyStatus tracks the review lifecycle
type AnomalyStatus string

const (
    StatusPending      AnomalyStatus = "pending"        // Awaiting review
    StatusLLMReviewing AnomalyStatus = "llm_reviewing"  // LLM currently reviewing
    StatusLLMApproved  AnomalyStatus = "llm_approved"   // LLM approved (high confidence)
    StatusLLMRejected  AnomalyStatus = "llm_rejected"   // LLM rejected (high confidence)
    StatusHumanReview  AnomalyStatus = "human_review"   // Escalated to human
    StatusApproved     AnomalyStatus = "approved"       // Human approved
    StatusRejected     AnomalyStatus = "rejected"       // Human rejected
    StatusApplied      AnomalyStatus = "applied"        // Relationship created in graph
)

// StructuralAnomaly represents a detected inference opportunity
type StructuralAnomaly struct {
    // Identity
    ID   string      `json:"id"`
    Type AnomalyType `json:"type"`
    
    // Entities involved
    EntityA string `json:"entity_a"`
    EntityB string `json:"entity_b,omitempty"` // Empty for single-entity anomalies
    
    // Scoring
    Confidence float64        `json:"confidence"` // 0.0-1.0
    Evidence   map[string]any `json:"evidence"`   // Type-specific evidence
    
    // Suggested action
    Suggestion *RelationshipSuggestion `json:"suggestion,omitempty"`
    
    // Lifecycle
    Status     AnomalyStatus `json:"status"`
    DetectedAt time.Time     `json:"detected_at"`
    
    // Review tracking
    ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
    ReviewedBy   string     `json:"reviewed_by,omitempty"` // "llm" or user identifier
    ReviewNotes  string     `json:"review_notes,omitempty"`
    LLMReasoning string     `json:"llm_reasoning,omitempty"`
    
    // Context for human review (populated by LLM review)
    EntityAContext string `json:"entity_a_context,omitempty"`
    EntityBContext string `json:"entity_b_context,omitempty"`
}

// RelationshipSuggestion proposes a new relationship to add
type RelationshipSuggestion struct {
    FromEntity    string  `json:"from_entity"`
    ToEntity      string  `json:"to_entity"`
    Predicate     string  `json:"predicate"`
    Confidence    float64 `json:"confidence"`
    Reasoning     string  `json:"reasoning"`
    Bidirectional bool    `json:"bidirectional"` // Create both directions?
}

// ReviewDecision represents a human review decision
type ReviewDecision struct {
    AnomalyID         string        `json:"anomaly_id"`
    Decision          AnomalyStatus `json:"decision"` // approved or rejected
    ReviewedBy        string        `json:"reviewed_by"`
    Notes             string        `json:"notes,omitempty"`
    OverridePredicate string        `json:"override_predicate,omitempty"` // Optional predicate override
}

// InferenceResult summarizes an inference detection run
type InferenceResult struct {
    StartedAt    time.Time            `json:"started_at"`
    CompletedAt  time.Time            `json:"completed_at"`
    Anomalies    []*StructuralAnomaly `json:"anomalies"`
    AppliedCount int                  `json:"applied_count"`
    Truncated    bool                 `json:"truncated"` // Hit max anomalies limit
}
```

## 4. Interface Definitions

### 4.1 Structural Index Interfaces

```go
// processor/graph/structuralindex/types.go (continued)

// KCoreIndex methods
func (idx *KCoreIndex) GetCore(entityID string) int
func (idx *KCoreIndex) FilterByMinCore(entityIDs []string, minCore int) []string
func (idx *KCoreIndex) GetEntitiesInCore(core int) []string
func (idx *KCoreIndex) GetEntitiesAboveCore(minCore int) []string

// PivotIndex methods
func (idx *PivotIndex) EstimateDistance(entityA, entityB string) (lower, upper int)
func (idx *PivotIndex) IsWithinHops(entityA, entityB string, maxHops int) bool
func (idx *PivotIndex) GetReachableCandidates(source string, maxHops int) []string
```

### 4.2 Storage Interfaces

```go
// processor/graph/structuralindex/storage.go

// StructuralIndexStorage persists structural indices to NATS KV
type StructuralIndexStorage interface {
    // SaveKCore persists k-core index
    SaveKCore(ctx context.Context, index *KCoreIndex) error
    
    // LoadKCore retrieves k-core index
    LoadKCore(ctx context.Context) (*KCoreIndex, error)
    
    // SavePivot persists pivot index
    SavePivot(ctx context.Context, index *PivotIndex) error
    
    // LoadPivot retrieves pivot index
    LoadPivot(ctx context.Context) (*PivotIndex, error)
    
    // GetEntityCore retrieves core number for single entity (fast path)
    GetEntityCore(ctx context.Context, entityID string) (int, error)
}

// processor/graph/inference/storage.go

// AnomalyStorage persists anomalies for review
type AnomalyStorage interface {
    // Save persists an anomaly (creates or updates)
    Save(ctx context.Context, anomaly *StructuralAnomaly) error
    
    // Get retrieves an anomaly by ID
    Get(ctx context.Context, id string) (*StructuralAnomaly, error)
    
    // GetByStatus retrieves all anomalies with given status
    GetByStatus(ctx context.Context, status AnomalyStatus) ([]*StructuralAnomaly, error)
    
    // UpdateStatus updates anomaly status and optional notes
    UpdateStatus(ctx context.Context, id string, status AnomalyStatus, notes string) error
    
    // Watch returns channel of anomalies as they're created/updated
    // Used by ReviewWorker to process new pending anomalies
    Watch(ctx context.Context) (<-chan *StructuralAnomaly, error)
    
    // Delete removes an anomaly
    Delete(ctx context.Context, id string) error
    
    // Cleanup removes old anomalies (applied/rejected older than retention)
    Cleanup(ctx context.Context, retention time.Duration) (int, error)
}
```

### 4.3 Detector Interfaces

```go
// processor/graph/inference/detector.go

// AnomalyDetector is implemented by each detection algorithm
type AnomalyDetector interface {
    // Name returns detector identifier for logging/metrics
    Name() string
    
    // Detect runs detection and returns anomalies
    Detect(ctx context.Context) ([]*StructuralAnomaly, error)
    
    // Configure updates detector configuration
    Configure(config map[string]any) error
}

// DetectorOrchestrator coordinates all detectors
type DetectorOrchestrator interface {
    // RegisterDetector adds a detector
    RegisterDetector(detector AnomalyDetector)
    
    // RunDetection executes all registered detectors
    RunDetection(ctx context.Context) (*InferenceResult, error)
}
```

### 4.4 Review Interfaces

```go
// processor/graph/inference/review_worker.go

// EntityFetcher retrieves entity context for LLM prompts
type EntityFetcher interface {
    // GetEntitySummary returns human-readable summary of entity
    GetEntitySummary(ctx context.Context, entityID string) (string, error)
}

// RelationshipApplier creates inferred relationships in the graph
type RelationshipApplier interface {
    // ApplyRelationship creates the suggested relationship
    // source identifies the creator ("llm_inference", "human_review", etc.)
    ApplyRelationship(ctx context.Context, suggestion RelationshipSuggestion, source string) error
}
```

## 5. Algorithm Specifications

### 5.1 K-Core Decomposition

**Algorithm**: Peeling (iterative degree removal)

**Time Complexity**: O(V + E)

**Space Complexity**: O(V)

```
Input: Graph G = (V, E) via GraphProvider
Output: KCoreIndex with core number for each vertex

1. Initialize:
   - degree[v] = |neighbors(v)| for all v ∈ V
   - coreNumbers = empty map
   - removed = empty set

2. Sort vertices by degree ascending

3. For each vertex v in sorted order:
   a. If v in removed: skip
   b. core[v] = degree[v]
   c. Add v to removed
   d. For each neighbor u of v:
      - If u not in removed AND degree[u] > core[v]:
        - degree[u] = degree[u] - 1

4. Build coreBuckets from coreNumbers

5. Return KCoreIndex
```

### 5.2 Pivot-Based Distance Index

**Algorithm**: PageRank pivot selection + BFS distance computation

**Time Complexity**: O(k * (V + E)) where k = pivot count

**Space Complexity**: O(V * k)

```
Input: Graph G, pivotCount k
Output: PivotIndex with distance vectors

1. Select Pivots (PageRank):
   a. Initialize scores[v] = 1/|V| for all v
   b. For 20 iterations:
      - newScores[v] = (1-d)/|V| for all v (d=0.85)
      - For each edge (u,v):
        - newScores[v] += d * scores[u] / outDegree(u)
      - scores = newScores
   c. Sort by score descending, take top k as pivots

2. Compute Distance Vectors:
   a. Initialize distanceVectors[v] = [255, 255, ...] for all v
   b. For each pivot p at index i:
      - Run BFS from p
      - For each reached vertex v at distance d:
        - distanceVectors[v][i] = d

3. Return PivotIndex
```

### 5.3 Distance Estimation (Triangle Inequality)

```
Input: PivotIndex, entityA, entityB
Output: (lowerBound, upperBound) for d(A, B)

vecA = distanceVectors[A]
vecB = distanceVectors[B]

lowerBound = 0
upperBound = ∞

For each pivot i:
  diff = |vecA[i] - vecB[i]|
  sum = vecA[i] + vecB[i]
  
  lowerBound = max(lowerBound, diff)  // Triangle inequality lower
  upperBound = min(upperBound, sum)   // Triangle inequality upper

Return (lowerBound, upperBound)
```

### 5.4 Semantic-Structural Gap Detection

```
Input: SimilarityFinder, PivotIndex, KCoreIndex, config
Output: List of StructuralAnomaly

For each entity A:
  1. Find semantically similar entities:
     similar = FindSimilarEntities(A, threshold=0.7, limit=50)
  
  2. For each similar entity B:
     a. Get structural distance estimate:
        (lower, upper) = PivotIndex.EstimateDistance(A, B)
     
     b. If lower >= minStructuralDistance (default: 3):
        - Calculate confidence:
          base = similarity score
          boost = (lower - minStructuralDistance) / 10 (capped at 0.2)
          coreBoost = 0.1 if both in core >= 2
          confidence = min(base + boost + coreBoost, 1.0)
        
        - Create anomaly with suggestion:
          predicate = "inferred.related_to" (or type-specific)
          
  3. Sort by confidence, take top maxGapsPerEntity

Return anomalies
```

### 5.5 Core Isolation Detection

```
Input: KCoreIndex, GraphProvider, config
Output: List of StructuralAnomaly

1. Get entities in high cores (>= minCoreForHubAnalysis)

2. Group by core number

3. For each core level with 2+ peers:
   For each entity E in core:
     a. Get neighbors of E
     b. Count same-core neighbors
     c. peerConnectivity = sameCoreNeighbors / (totalPeers - 1)
     
     d. If peerConnectivity < hubIsolationThreshold (default: 0.3):
        - Find unconnected peers
        - Create anomaly:
          confidence = 1.0 - peerConnectivity
          suggestion = connect to highest-core unconnected peer

Return anomalies
```

## 6. Configuration Schema

### 6.1 Structural Index Configuration

```go
// Add to processor/graph/processor.go ClusteringConfig

type StructuralIndexConfig struct {
    // Enabled activates structural index computation
    Enabled bool `json:"enabled" schema:"type:bool,default:true"`
    
    // KCore configuration
    KCore KCoreConfig `json:"kcore"`
    
    // Pivot configuration
    Pivot PivotConfig `json:"pivot"`
}

type KCoreConfig struct {
    // Enabled activates k-core computation
    Enabled bool `json:"enabled" schema:"type:bool,default:true"`
    
    // MinSearchCore filters search results to entities in core >= this value
    // 0 = no filtering, 2 = filter out leaf nodes
    MinSearchCore int `json:"min_search_core" schema:"type:int,default:0"`
}

type PivotConfig struct {
    // Enabled activates pivot index computation
    Enabled bool `json:"enabled" schema:"type:bool,default:true"`
    
    // PivotCount is number of landmark nodes to use
    // More pivots = better estimates but more storage/compute
    PivotCount int `json:"pivot_count" schema:"type:int,default:16,min:4,max:64"`
}
```

### 6.2 Inference Configuration

```go
type StructuralInferenceConfig struct {
    // Enabled activates inference detection
    Enabled bool `json:"enabled" schema:"type:bool,default:false"`
    
    // RunWithCommunityDetection triggers inference after community detection
    RunWithCommunityDetection bool `json:"run_with_community_detection" schema:"type:bool,default:true"`
    
    // MaxAnomaliesPerRun limits total anomalies detected per run
    MaxAnomaliesPerRun int `json:"max_anomalies_per_run" schema:"type:int,default:100"`
    
    // Detector-specific configs
    SemanticStructuralGap SemanticGapConfig   `json:"semantic_structural_gap"`
    CoreInference         CoreInferenceConfig `json:"core_inference"`
    TransitivityGap       TransitivityConfig  `json:"transitivity_gap"`
    
    // Review configuration
    Review ReviewConfig `json:"review"`
}

type SemanticGapConfig struct {
    Enabled              bool    `json:"enabled" schema:"type:bool,default:true"`
    MinSemanticSimilarity float64 `json:"min_semantic_similarity" schema:"type:float,default:0.7"`
    MinStructuralDistance int     `json:"min_structural_distance" schema:"type:int,default:3"`
    MaxGapsPerEntity      int     `json:"max_gaps_per_entity" schema:"type:int,default:5"`
}

type CoreInferenceConfig struct {
    Enabled               bool    `json:"enabled" schema:"type:bool,default:true"`
    MinCoreForHubAnalysis int     `json:"min_core_for_hub_analysis" schema:"type:int,default:2"`
    HubIsolationThreshold float64 `json:"hub_isolation_threshold" schema:"type:float,default:0.3"`
    TrackCoreDemotions    bool    `json:"track_core_demotions" schema:"type:bool,default:true"`
}

type TransitivityConfig struct {
    Enabled                 bool     `json:"enabled" schema:"type:bool,default:true"`
    MaxIntermediateHops     int      `json:"max_intermediate_hops" schema:"type:int,default:2"`
    MinExpectedTransitivity int      `json:"min_expected_transitivity" schema:"type:int,default:3"`
    TransitivePredicates    []string `json:"transitive_predicates"`
}

type ReviewConfig struct {
    // Enabled activates LLM review worker
    Enabled bool `json:"enabled" schema:"type:bool,default:false"`
    
    // Workers is concurrent review worker count
    Workers int `json:"workers" schema:"type:int,default:2"`
    
    // AutoApproveThreshold: LLM can auto-approve above this confidence
    AutoApproveThreshold float64 `json:"auto_approve_threshold" schema:"type:float,default:0.9"`
    
    // AutoRejectThreshold: LLM can auto-reject below this confidence
    AutoRejectThreshold float64 `json:"auto_reject_threshold" schema:"type:float,default:0.3"`
    
    // FallbackToHuman escalates uncertain cases to human review
    FallbackToHuman bool `json:"fallback_to_human" schema:"type:bool,default:true"`
    
    // LLM client configuration (reuses existing llm.Config)
    LLM llm.Config `json:"llm"`
}
```

### 6.3 Example Configuration

```json
{
  "clustering": {
    "enabled": true,
    "algorithm": {
      "max_iterations": 100,
      "levels": 3
    },
    
    "structural_index": {
      "enabled": true,
      "kcore": {
        "enabled": true,
        "min_search_core": 2
      },
      "pivot": {
        "enabled": true,
        "pivot_count": 16
      }
    },
    
    "inference": {
      "enabled": true,
      "run_with_community_detection": true,
      "max_anomalies_per_run": 100,
      
      "semantic_structural_gap": {
        "enabled": true,
        "min_semantic_similarity": 0.7,
        "min_structural_distance": 3,
        "max_gaps_per_entity": 5
      },
      "core_inference": {
        "enabled": true,
        "min_core_for_hub_analysis": 2,
        "hub_isolation_threshold": 0.3,
        "track_core_demotions": true
      },
      "transitivity_gap": {
        "enabled": true,
        "max_intermediate_hops": 2,
        "min_expected_transitivity": 3,
        "transitive_predicates": ["member_of", "part_of", "controlled_by"]
      },
      
      "review": {
        "enabled": true,
        "workers": 2,
        "auto_approve_threshold": 0.9,
        "auto_reject_threshold": 0.3,
        "fallback_to_human": true,
        "llm": {
          "provider": "openai",
          "base_url": "http://seminstruct:8080/v1",
          "model": "mistral-7b-instruct",
          "timeout_seconds": 30,
          "max_retries": 3
        }
      }
    }
  }
}
```

## 7. KV Bucket Specifications

### 7.1 STRUCTURAL_INDEX Bucket

**Purpose**: Persist structural index data

**Key Patterns**:
```
kcore._meta                    # K-core metadata (max_core, computed_at, etc.)
kcore.entity.{entity_id}       # Individual entity core number (for fast lookup)
kcore.bucket.{core_number}     # Entity IDs in each core bucket

pivot._meta                    # Pivot metadata (pivots list, computed_at)
pivot.entity.{entity_id}       # Distance vector for entity
```

**Retention**: Overwritten on each computation (no history needed)

### 7.2 ANOMALY_INDEX Bucket

**Purpose**: Persist anomalies through review lifecycle

**Key Patterns**:
```
anomaly.{uuid}                 # Full anomaly document
```

**Retention**: Configurable cleanup of applied/rejected anomalies

**Watch Pattern**: `anomaly.*` for ReviewWorker

## 8. HTTP API Endpoints

### 8.1 Human Review Endpoints

```
GET  /inference/anomalies/pending
     Returns anomalies with status "human_review"
     Response: [StructuralAnomaly, ...]

GET  /inference/anomalies/{id}
     Returns specific anomaly with full context
     Response: StructuralAnomaly

POST /inference/anomalies/{id}/review
     Submit human review decision
     Request: ReviewDecision
     Response: {"status": "applied|rejected"}

GET  /inference/stats
     Returns inference statistics
     Response: {
       "total_detected": int,
       "pending_review": int,
       "llm_approved": int,
       "llm_rejected": int,
       "human_approved": int,
       "human_rejected": int,
       "applied": int
     }
```

### 8.2 Query Integration Endpoints (Existing - Enhanced)

```
POST /search/semantic
     Enhanced with optional structural filtering
     Request: {
       "query": "...",
       "threshold": 0.3,
       "limit": 10,
       "min_core": 2        // NEW: Filter by k-core
     }

POST /entity/{id}/path
     Enhanced with pivot-based optimization
     Request: {
       "max_depth": 3,
       "max_nodes": 100,
       "use_pivot_pruning": true  // NEW: Enable pivot optimization
     }
```

## 9. Integration Points

### 9.1 Processor Integration

Modify `processor/graph/processor.go`:

```go
// Add to Processor struct
type Processor struct {
    // ... existing fields ...
    
    // Structural indexing
    structuralIndexStorage structuralindex.StructuralIndexStorage
    structuralIndices      *structuralindex.StructuralIndices
    
    // Inference
    anomalyStorage     inference.AnomalyStorage
    inferenceDetector  inference.DetectorOrchestrator
    reviewWorker       *inference.ReviewWorker
}

// Modify detectCommunities to include structural indexing
func (p *Processor) detectCommunities(ctx context.Context) error {
    // 1. Existing LPA detection
    communities, err := p.communityDetector.DetectCommunities(ctx)
    
    // 2. NEW: Compute structural indices
    if p.config.Clustering.StructuralIndex.Enabled {
        p.computeStructuralIndices(ctx)
    }
    
    // 3. NEW: Run inference detection
    if p.config.Clustering.Inference.Enabled {
        p.runStructuralInference(ctx)
    }
    
    // 4. Existing inference (community-based)
    if p.config.Clustering.Inference.Enabled {
        p.runInference(ctx, communities)
    }
    
    return nil
}
```

### 9.2 IndexManager Integration

Modify `processor/graph/indexmanager/semantic.go`:

```go
// Add structural filtering to SearchSemantic
func (m *Manager) SearchSemantic(ctx context.Context, query string, threshold float64, limit int) ([]SimilarityHit, error) {
    // Existing search logic...
    
    // NEW: Apply k-core filtering if configured
    if m.structuralIndices != nil && m.config.StructuralIndex.KCore.MinSearchCore > 0 {
        results = m.filterByKCore(results, m.config.StructuralIndex.KCore.MinSearchCore)
    }
    
    return results, nil
}
```

### 9.3 QueryManager Integration

Modify `processor/graph/querymanager/query.go`:

```go
// Enhance PathRAG with pivot pruning
func (qm *Manager) ExecutePath(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error) {
    // NEW: Pre-filter candidates using pivot index
    if qm.structuralIndices != nil && qm.structuralIndices.Pivot != nil {
        candidates := qm.structuralIndices.Pivot.GetReachableCandidates(start, pattern.MaxDepth)
        pattern.CandidateFilter = candidates
    }
    
    // Existing traversal with candidate filter applied...
}
```

## 10. Metrics

### 10.1 Prometheus Metrics

```go
// Structural Index Metrics
structural_index_computation_duration_seconds{type="kcore|pivot"}
structural_index_entities_total{type="kcore|pivot"}
structural_index_kcore_max_core
structural_index_pivot_count

// Inference Metrics  
inference_anomalies_detected_total{type="semantic_gap|core_isolation|..."}
inference_anomalies_by_status{status="pending|llm_approved|..."}
inference_review_duration_seconds{reviewer="llm|human"}
inference_relationships_applied_total{source="llm|human"}

// Review Worker Metrics
inference_review_worker_queue_size
inference_review_worker_llm_calls_total
inference_review_worker_llm_errors_total
inference_review_worker_auto_approved_total
inference_review_worker_auto_rejected_total
inference_review_worker_escalated_total
```

## 11. Testing Requirements

### 11.1 Unit Tests

**structuralindex package**:
- `TestKCoreComputer_SimpleGraph`: Verify core numbers on known graph
- `TestKCoreComputer_EmptyGraph`: Handle empty input
- `TestKCoreComputer_SingleNode`: Handle single node
- `TestKCoreComputer_DisconnectedComponents`: Verify independent components
- `TestPivotComputer_DistanceEstimates`: Verify triangle inequality bounds
- `TestPivotComputer_PivotSelection`: Verify PageRank selects high-centrality nodes
- `TestKCoreIndex_FilterByMinCore`: Verify filtering
- `TestPivotIndex_IsWithinHops`: Verify reachability checks

**inference package**:
- `TestSemanticGapDetector_FindsGaps`: Detect known gaps in test graph
- `TestSemanticGapDetector_NoFalsePositives`: Similar + close = no gap
- `TestCoreAnomalyDetector_HubIsolation`: Detect isolated hubs
- `TestCoreAnomalyDetector_CoreDemotion`: Detect demoted entities
- `TestTransitivityDetector_FindsGaps`: Detect transitivity violations
- `TestReviewWorker_AutoApprove`: High confidence triggers auto-apply
- `TestReviewWorker_AutoReject`: Low confidence triggers rejection
- `TestReviewWorker_Escalate`: Mid confidence escalates to human

### 11.2 Integration Tests

- `TestStructuralIndexIntegration`: Full computation with real NATS KV
- `TestInferenceIntegration`: Detection → Storage → Review flow
- `TestQueryWithStructuralFiltering`: Search with k-core filter
- `TestPathRAGWithPivotPruning`: PathRAG optimization verification

### 11.3 Test Data

Create test fixtures in `testdata/`:
- `small_graph.json`: 20 nodes, known k-core structure
- `hub_spoke.json`: Clear hub-and-spoke pattern
- `communities.json`: Multiple dense communities with bridges

## 12. Implementation Order

### Phase 1: Structural Index Core (Tier 1)
1. Create `structuralindex` package with types
2. Implement `KCoreComputer` with tests
3. Implement `PivotComputer` with tests
4. Implement `NATSStructuralIndexStorage`
5. Integrate with `processor.go` (compute after community detection)

### Phase 2: Query Integration (Tier 1)
1. Add `StructuralGraphProvider` wrapper
2. Integrate k-core filtering into `IndexManager.SearchSemantic`
3. Integrate pivot pruning into `QueryManager.ExecutePath`
4. Add configuration options

### Phase 3: Inference Detection (Tier 1)
1. Create `inference` package with types
2. Implement `SemanticStructuralGapDetector`
3. Implement `CoreAnomalyDetector`
4. Implement `TransitivityGapDetector`
5. Implement `DetectorOrchestrator`
6. Implement `NATSAnomalyStorage`
7. Integrate with `processor.go`

### Phase 4: LLM Review (Tier 2)
1. Implement `ReviewWorker` following `EnhancementWorker` pattern
2. Implement `RelationshipApplier`
3. Add LLM prompt templates
4. Add HTTP endpoints for human review
5. Add metrics

### Phase 5: Documentation & Polish
1. README.md for each new package
2. Configuration documentation
3. API documentation
4. Example configurations

## 13. Acceptance Criteria

### Must Have
- [ ] K-core decomposition computes correct core numbers
- [ ] Pivot index provides valid distance bounds (lower ≤ actual ≤ upper)
- [ ] Semantic-structural gap detector identifies known gaps in test data
- [ ] Anomalies persist to NATS KV with correct lifecycle states
- [ ] ReviewWorker processes pending anomalies
- [ ] Auto-approve/reject thresholds work correctly
- [ ] Human review API endpoints functional
- [ ] Configuration follows existing patterns
- [ ] All tests pass including integration tests

### Should Have
- [ ] Core demotion tracking between runs
- [ ] Transitivity gap detection
- [ ] Pivot-based PathRAG optimization measurably improves performance
- [ ] K-core search filtering reduces noise in results
- [ ] Metrics exposed for all operations

### Nice to Have
- [ ] Community bridge detection
- [ ] Predicate suggestion based on entity types
- [ ] Batch review UI support
- [ ] Historical anomaly analysis

## 14. Open Questions

1. **Pivot count tuning**: Should we auto-tune based on graph size?
2. **Index refresh frequency**: Same as community detection, or independent schedule?
3. **Anomaly retention**: How long to keep applied/rejected anomalies?
4. **LLM prompt optimization**: Should prompts be configurable per anomaly type?

## 15. References

- [K-Core Decomposition](https://en.wikipedia.org/wiki/Degeneracy_(graph_theory))
- [Pivot-Based Distance Indexing](https://dl.acm.org/doi/10.1145/1807167.1807252)
- [Triangle Inequality for Distance Estimation](https://www.vldb.org/pvldb/vol12/p1819-wang.pdf)
- Existing SemStreams docs: `docs/advanced/01-clustering.md`, `docs/architecture/graph-processor.md`
