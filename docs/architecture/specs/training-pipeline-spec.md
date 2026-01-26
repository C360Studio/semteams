# SemStreams Training Processor Specification

> **Status**: Planned (not yet implemented)
>
> **Core Dependencies**: This spec references `QUERY_LOG` stream and `RULE_DEFINITIONS`
> bucket which do not yet exist in core. Before implementation, these must be added to
> core or the extractors adapted to use existing data sources.

## Overview

The `training-processor` component generates training data for domain-specific SLM
fine-tuning. It transforms graph state, query logs, and documents into
instruction-response pairs suitable for QLoRA fine-tuning.

## Design Principles

1. **Tiered Autonomy:** Operates without human intervention when configured for
   deterministic sources; supports human-in-loop when available
2. **LLM Independence:** Core functionality works without LLM; LLM enables higher quality but isn't required
3. **Embedding Integration:** Uses semembed for quality control, not data generation
4. **Incremental Processing:** Processes new data since last export, not full recomputation
5. **Validation-Ready:** Outputs include holdout splits and confidence scores

## Component Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                         training-processor                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │
│  │  Extractors │───►│ Generators  │───►│  Filters    │───►│  Writers    │  │
│  └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘  │
│                                                                              │
│  Extractors:         Generators:        Filters:          Writers:          │
│  • entity-extractor  • template-gen     • dedup-filter    • jsonl-writer    │
│  • relation-extractor• llm-gen          • quality-filter  • bucket-writer   │
│  • rule-extractor    • augment-gen      • cluster-sample  • split-writer    │
│  • query-extractor                      • human-filter                      │
│  • doc-extractor                                                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

```text
                    SOURCES                          PROCESSORS                      OUTPUT
                    
┌──────────────────────────┐
│     ENTITY_STATES        │──┐
│  (descriptions, labels)  │  │
└──────────────────────────┘  │
                              │    ┌─────────────────┐
┌──────────────────────────┐  │    │                 │
│    OUTGOING_INDEX        │──┼───►│   Extractors    │
│    INCOMING_INDEX        │  │    │                 │
│  (relationships)         │  │    └────────┬────────┘
└──────────────────────────┘  │             │
                              │             ▼
┌──────────────────────────┐  │    ┌─────────────────┐     ┌─────────────────┐
│     RULE_DEFINITIONS     │──┤    │                 │     │                 │
│  (conditions, actions)   │  │    │   Generators    │────►│    Filters      │
└──────────────────────────┘  │    │                 │     │                 │
                              │    └─────────────────┘     └────────┬────────┘
┌──────────────────────────┐  │                                     │
│      QUERY_LOG           │──┤                                     ▼
│  (queries, clicks, dwell)│  │                           ┌─────────────────┐
└──────────────────────────┘  │                           │                 │
                              │                           │    Writers      │
┌──────────────────────────┐  │                           │                 │
│     OBJECT_STORE         │──┘                           └────────┬────────┘
│  (documents, content)    │                                       │
└──────────────────────────┘                                       ▼
                                                          ┌─────────────────┐
                                                          │  TRAINING_DATA  │
                                                          │    (bucket)     │
                                                          ├─────────────────┤
                                                          │ • pairs.jsonl   │
                                                          │ • train.jsonl   │
                                                          │ • valid.jsonl   │
                                                          │ • metadata.json │
                                                          └─────────────────┘
```

## Configuration

### Full Configuration Schema

```yaml
training:
  # Component identification
  component_id: "training-processor"
  
  # Source configuration - which extractors to enable
  sources:
    # Tier 0: Structural (deterministic, no dependencies)
    entities:
      enabled: true
      min_description_length: 20        # Skip entities with terse descriptions
      include_labels: true              # Generate "What is X?" from labels
      include_aliases: true             # Include alias variations
      
    relationships:
      enabled: true
      min_relationships: 1              # Entity must have relationships
      bidirectional: true               # Generate both directions
      max_hops: 2                        # Multi-hop relationship questions
      
    rules:
      enabled: true
      include_triggers: true            # Include historical trigger examples
      include_conditions: true          # "When does X happen?"
      include_actions: true             # "What happens when X?"
      
    # Tier 1: Statistical (deterministic, computed)
    communities:
      enabled: true
      min_community_size: 3             # Skip tiny communities
      include_membership: true          # "What group is X in?"
      include_neighbors: true           # "What's related to X?"
      
    queries:
      enabled: true
      min_confidence: 0.7               # User signal confidence threshold
      require_click: true               # Must have click signal
      min_dwell_seconds: 30             # Minimum time on result
      include_refinements: false        # Include query refinement chains
      
    # Tier 2: Semantic (requires LLM)
    documents:
      enabled: false                    # Disabled by default (requires LLM)
      synthetic_qa: true                # Generate QA from document content
      chunk_size: 512                   # Document chunk size for QA gen
      questions_per_chunk: 3            # QA pairs per chunk
      
    # Human-in-loop (requires human feedback)
    feedback:
      enabled: true
      require_explicit: false           # Require thumbs up/down
      implicit_positive_threshold: 0.8  # Click + dwell = implicit positive
      include_curated: true             # Include manually curated pairs
      
  # Generator configuration
  generators:
    # Template-based generation (no LLM)
    templates:
      enabled: true
      variations: 3                     # Paraphrase variations per template
      include_negatives: true           # Generate "What is NOT X?" pairs
      
    # LLM-based generation (requires LLM)
    llm:
      enabled: false                    # Disabled by default
      model: "llama-3.3-70b"           # Model for generation
      temperature: 0.7                  # Generation temperature
      batch_size: 10                    # Batch size for efficiency
      
    # Augmentation (optional, improves diversity)
    augmentation:
      enabled: true
      synonym_replacement: true         # Replace with synonyms
      entity_swap: true                 # Swap similar entities
      paraphrase: false                 # Requires LLM
      
  # Filter configuration
  filters:
    # Deduplication (uses semembed)
    deduplication:
      enabled: true
      similarity_threshold: 0.95        # Cosine similarity for dedup
      prefer_human_validated: true      # Keep human-validated over generated
      
    # Quality filtering
    quality:
      enabled: true
      min_question_length: 10           # Characters
      max_question_length: 500
      min_answer_length: 20
      max_answer_length: 2000
      require_complete_sentences: true
      
    # Cluster-based sampling (uses semembed)
    clustering:
      enabled: true
      target_clusters: 100              # Number of semantic clusters
      samples_per_cluster: 50           # Max samples per cluster
      ensure_coverage: true             # At least 1 from each cluster
      
    # Human validation queue
    human_review:
      enabled: false                    # Disabled for autonomous operation
      queue_low_confidence: true        # Queue uncertain pairs for review
      confidence_threshold: 0.6         # Below this goes to review
      
  # Output configuration
  output:
    format: "jsonl"                     # Output format
    bucket: "TRAINING_DATA"             # NATS KV bucket
    
    splits:
      train: 0.85                       # Training split
      validation: 0.10                  # Validation split
      test: 0.05                        # Held-out test split
      
    metadata:
      include_source: true              # Track where pair came from
      include_confidence: true          # Include confidence score
      include_timestamp: true           # When pair was generated
      
  # Schedule configuration
  schedule:
    mode: "triggered"                   # "triggered" | "scheduled" | "continuous"
    trigger_subject: "training.export.request"
    
    # For scheduled mode
    cron: "0 2 * * *"                  # 2 AM daily
    
    # Conditions to actually run
    conditions:
      min_new_pairs: 100                # Minimum new pairs since last run
      min_hours_since_last: 24          # Minimum time between runs
      max_system_load: 0.5              # Don't run if system busy
      
  # Embedding integration (via semembed)
  embedding:
    enabled: true
    component: "semembed"               # Use existing semembed component
    cache_embeddings: true              # Cache for reuse
    batch_size: 100                     # Embedding batch size
```

### Minimal Configuration (Deterministic Only)

```yaml
training:
  sources:
    entities:
      enabled: true
    relationships:
      enabled: true
    rules:
      enabled: true
    communities:
      enabled: true
    queries:
      enabled: true
      require_click: true
    documents:
      enabled: false                    # No LLM available
    feedback:
      enabled: true
      require_explicit: false
      
  generators:
    templates:
      enabled: true
    llm:
      enabled: false                    # No LLM available
      
  filters:
    deduplication:
      enabled: true
    quality:
      enabled: true
    clustering:
      enabled: true
    human_review:
      enabled: false                    # No human available
      
  embedding:
    enabled: true                       # Use statistical mode if neural unavailable
```

### Vessel Configuration (Autonomous with LLM)

```yaml
training:
  sources:
    entities:
      enabled: true
    relationships:
      enabled: true
    rules:
      enabled: true
    communities:
      enabled: true
    queries:
      enabled: true
    documents:
      enabled: true                     # LLM available for synthetic QA
    feedback:
      enabled: true
      require_explicit: false           # Use implicit signals
      
  generators:
    templates:
      enabled: true
    llm:
      enabled: true                     # LLM available
      model: "llama-3.3-70b"
      
  filters:
    deduplication:
      enabled: true
    quality:
      enabled: true
    clustering:
      enabled: true
    human_review:
      enabled: false                    # No human at sea
      
  schedule:
    mode: "scheduled"
    cron: "0 2 * * 0"                  # Weekly, Sunday 2 AM
    conditions:
      min_new_pairs: 500
      min_hours_since_last: 168         # Weekly
```

### Shore Configuration (Full Human-in-Loop)

```yaml
training:
  sources:
    entities:
      enabled: true
    relationships:
      enabled: true
    rules:
      enabled: true
    communities:
      enabled: true
    queries:
      enabled: true
    documents:
      enabled: true
    feedback:
      enabled: true
      require_explicit: true            # Prefer explicit feedback
      include_curated: true
      
  generators:
    templates:
      enabled: true
    llm:
      enabled: true
    augmentation:
      enabled: true
      paraphrase: true                  # LLM paraphrasing
      
  filters:
    deduplication:
      enabled: true
    quality:
      enabled: true
    clustering:
      enabled: true
    human_review:
      enabled: true                     # Human available
      queue_low_confidence: true
      
  schedule:
    mode: "triggered"                   # On-demand
```

## Extractors

### EntityExtractor

Generates factual QA pairs from entity descriptions and labels.

```go
type EntityExtractor struct {
    store      EntityStore
    config     EntityExtractorConfig
}

type EntityExtractorConfig struct {
    MinDescriptionLength int
    IncludeLabels        bool
    IncludeAliases       bool
}

// Extract generates QA pairs from entities
func (e *EntityExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    pairs := []RawPair{}
    
    entities, err := e.store.ListModifiedSince(ctx, since)
    if err != nil {
        return nil, err
    }
    
    for _, ent := range entities {
        // Skip if description too short
        if len(ent.Description) < e.config.MinDescriptionLength {
            continue
        }
        
        // Primary QA from description
        pairs = append(pairs, RawPair{
            Question:   fmt.Sprintf("What is %s?", ent.Label),
            Answer:     ent.Description,
            Source:     "entity",
            SourceID:   ent.ID,
            Confidence: 1.0, // Deterministic
        })
        
        // Alias variations
        if e.config.IncludeAliases {
            for _, alias := range ent.Aliases {
                pairs = append(pairs, RawPair{
                    Question:   fmt.Sprintf("What is %s?", alias),
                    Answer:     ent.Description,
                    Source:     "entity_alias",
                    SourceID:   ent.ID,
                    Confidence: 1.0,
                })
            }
        }
    }
    
    return pairs, nil
}
```

### RelationshipExtractor

Generates relational QA from graph structure.

```go
type RelationshipExtractor struct {
    store      EntityStore
    config     RelationshipExtractorConfig
}

type RelationshipExtractorConfig struct {
    MinRelationships int
    Bidirectional    bool
    MaxHops          int
}

func (r *RelationshipExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    pairs := []RawPair{}
    
    // Get entities with relationships modified since timestamp
    entities, err := r.store.ListWithRelationships(ctx, since)
    if err != nil {
        return nil, err
    }
    
    for _, ent := range entities {
        outgoing := r.store.GetOutgoing(ctx, ent.ID)
        if len(outgoing) < r.config.MinRelationships {
            continue
        }
        
        // Group by predicate
        byPredicate := groupByPredicate(outgoing)
        
        for pred, rels := range byPredicate {
            objects := extractLabels(rels)
            
            // Forward question: "What does X <predicate>?"
            pairs = append(pairs, RawPair{
                Question:   fmt.Sprintf("What does %s %s?", ent.Label, predicateToQuestion(pred)),
                Answer:     strings.Join(objects, ", "),
                Source:     "relationship",
                SourceID:   ent.ID,
                Confidence: 1.0,
            })
            
            // Reverse question: "What <predicate> X?"
            if r.config.Bidirectional {
                for _, rel := range rels {
                    pairs = append(pairs, RawPair{
                        Question:   fmt.Sprintf("What %s %s?", predicateToQuestion(pred), rel.Object.Label),
                        Answer:     ent.Label,
                        Source:     "relationship_reverse",
                        SourceID:   rel.Object.ID,
                        Confidence: 1.0,
                    })
                }
            }
        }
        
        // Multi-hop if configured
        if r.config.MaxHops > 1 {
            pairs = append(pairs, r.extractMultiHop(ctx, ent, r.config.MaxHops)...)
        }
    }
    
    return pairs, nil
}

func predicateToQuestion(pred string) string {
    // Convert predicate to natural question form
    // "deployed_at" -> "is deployed at"
    // "monitors" -> "monitor"
    // "part_of" -> "is part of"
    mapping := map[string]string{
        "deployed_at":  "is deployed at",
        "monitors":     "monitor",
        "part_of":      "is part of",
        "connected_to": "is connected to",
        "operates":     "operate",
        "measures":     "measure",
    }
    if q, ok := mapping[pred]; ok {
        return q
    }
    return strings.ReplaceAll(pred, "_", " ")
}
```

### RuleExtractor

Generates procedural QA from rule definitions and trigger history.

```go
type RuleExtractor struct {
    engine     RuleEngine
    config     RuleExtractorConfig
}

type RuleExtractorConfig struct {
    IncludeTriggers   bool
    IncludeConditions bool
    IncludeActions    bool
}

func (r *RuleExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    pairs := []RawPair{}
    
    rules := r.engine.ListRules(ctx)
    
    for _, rule := range rules {
        // Condition -> Action QA
        if r.config.IncludeConditions && r.config.IncludeActions {
            pairs = append(pairs, RawPair{
                Question:   fmt.Sprintf("What should happen when %s?", rule.ConditionDescription),
                Answer:     rule.ActionDescription,
                Source:     "rule",
                SourceID:   rule.ID,
                Confidence: 1.0,
            })
            
            // Reverse: "When does X action occur?"
            pairs = append(pairs, RawPair{
                Question:   fmt.Sprintf("When should %s?", rule.ActionDescription),
                Answer:     fmt.Sprintf("When %s", rule.ConditionDescription),
                Source:     "rule_reverse",
                SourceID:   rule.ID,
                Confidence: 1.0,
            })
        }
        
        // Include trigger history as examples
        if r.config.IncludeTriggers {
            triggers, _ := r.engine.GetTriggerHistory(ctx, rule.ID, since)
            for _, trigger := range triggers {
                pairs = append(pairs, RawPair{
                    Question:   fmt.Sprintf("What happened at %s when %s?", 
                                           trigger.Timestamp.Format("15:04"), 
                                           trigger.ConditionValues),
                    Answer:     fmt.Sprintf("%s was triggered, resulting in: %s", 
                                           rule.Name, trigger.ActionTaken),
                    Source:     "rule_trigger",
                    SourceID:   trigger.ID,
                    Confidence: 0.9, // Slightly lower - historical data
                })
            }
        }
    }
    
    return pairs, nil
}
```

### QueryExtractor

Generates QA from user queries with positive signals.

```go
type QueryExtractor struct {
    queryLog   QueryLog
    config     QueryExtractorConfig
}

type QueryExtractorConfig struct {
    MinConfidence       float64
    RequireClick        bool
    MinDwellSeconds     int
    IncludeRefinements  bool
}

func (q *QueryExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    pairs := []RawPair{}
    
    queries, err := q.queryLog.GetSince(ctx, since)
    if err != nil {
        return nil, err
    }
    
    for _, query := range queries {
        // Calculate confidence from signals
        confidence := q.calculateConfidence(query)
        if confidence < q.config.MinConfidence {
            continue
        }
        
        // Must have click if required
        if q.config.RequireClick && !query.HasClick {
            continue
        }
        
        // Check dwell time
        if query.DwellSeconds < q.config.MinDwellSeconds {
            continue
        }
        
        // Use the clicked/selected result as the answer
        if query.SelectedResult != nil {
            pairs = append(pairs, RawPair{
                Question:   query.QueryText,
                Answer:     query.SelectedResult.Summary,
                Source:     "query_log",
                SourceID:   query.ID,
                Confidence: confidence,
            })
        }
        
        // Include query refinements as chain
        if q.config.IncludeRefinements && len(query.Refinements) > 0 {
            for i, ref := range query.Refinements {
                if ref.SelectedResult != nil {
                    pairs = append(pairs, RawPair{
                        Question:   ref.QueryText,
                        Answer:     ref.SelectedResult.Summary,
                        Source:     "query_refinement",
                        SourceID:   fmt.Sprintf("%s.%d", query.ID, i),
                        Confidence: confidence * 0.9, // Slightly lower for refinements
                    })
                }
            }
        }
    }
    
    return pairs, nil
}

func (q *QueryExtractor) calculateConfidence(query QueryLogEntry) float64 {
    confidence := 0.5 // Base
    
    if query.HasClick {
        confidence += 0.2
    }
    if query.DwellSeconds > 60 {
        confidence += 0.1
    }
    if query.DwellSeconds > 120 {
        confidence += 0.1
    }
    if query.ExplicitFeedback == "positive" {
        confidence = 1.0 // Override to max
    }
    if query.ExplicitFeedback == "negative" {
        confidence = 0.0 // Exclude
    }
    
    return confidence
}
```

### DocumentExtractor

Generates synthetic QA from documents. **Requires LLM.**

```go
type DocumentExtractor struct {
    store      ObjectStore
    llm        LLMClient
    config     DocumentExtractorConfig
}

type DocumentExtractorConfig struct {
    SyntheticQA       bool
    ChunkSize         int
    QuestionsPerChunk int
}

func (d *DocumentExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    if d.llm == nil {
        return nil, ErrLLMRequired
    }
    
    pairs := []RawPair{}
    
    docs, err := d.store.ListDocuments(ctx, since)
    if err != nil {
        return nil, err
    }
    
    for _, doc := range docs {
        chunks := chunkDocument(doc.Content, d.config.ChunkSize)
        
        for i, chunk := range chunks {
            // Use LLM to generate QA pairs from chunk
            generated, err := d.generateQAFromChunk(ctx, chunk, d.config.QuestionsPerChunk)
            if err != nil {
                continue // Skip on error, don't fail entire extraction
            }
            
            for _, qa := range generated {
                pairs = append(pairs, RawPair{
                    Question:   qa.Question,
                    Answer:     qa.Answer,
                    Source:     "document_synthetic",
                    SourceID:   fmt.Sprintf("%s.chunk.%d", doc.ID, i),
                    Confidence: 0.8, // LLM-generated, slightly lower
                })
            }
        }
    }
    
    return pairs, nil
}

func (d *DocumentExtractor) generateQAFromChunk(ctx context.Context, chunk string, count int) ([]QAPair, error) {
    prompt := fmt.Sprintf(`Given this text, generate %d question-answer pairs that test understanding of the content.

Text:
%s

Generate exactly %d QA pairs in this format:
Q1: [question]
A1: [answer]
Q2: [question]
A2: [answer]
...

Questions should be specific and answerable from the text. Answers should be complete sentences.`, count, chunk, count)

    response, err := d.llm.Complete(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    return parseQAPairs(response)
}
```

### CommunityExtractor

Generates grouping QA from community membership.

```go
type CommunityExtractor struct {
    index      CommunityIndex
    store      EntityStore
    config     CommunityExtractorConfig
}

type CommunityExtractorConfig struct {
    MinCommunitySize  int
    IncludeMembership bool
    IncludeNeighbors  bool
}

func (c *CommunityExtractor) Extract(ctx context.Context, since time.Time) ([]RawPair, error) {
    pairs := []RawPair{}
    
    communities, err := c.index.ListCommunities(ctx)
    if err != nil {
        return nil, err
    }
    
    for _, comm := range communities {
        members := c.index.GetMembers(ctx, comm.ID)
        if len(members) < c.config.MinCommunitySize {
            continue
        }
        
        memberLabels := c.getLabels(ctx, members)
        
        // Community membership: "What's in the X group?"
        if c.config.IncludeMembership && comm.Label != "" {
            pairs = append(pairs, RawPair{
                Question:   fmt.Sprintf("What equipment/entities are in the %s group?", comm.Label),
                Answer:     strings.Join(memberLabels, ", "),
                Source:     "community",
                SourceID:   comm.ID,
                Confidence: 0.9, // Algorithmic, high confidence
            })
        }
        
        // Neighbor queries: "What's related to X?"
        if c.config.IncludeNeighbors {
            for _, member := range members {
                others := removeElement(memberLabels, member.Label)
                if len(others) > 0 {
                    pairs = append(pairs, RawPair{
                        Question:   fmt.Sprintf("What is related to %s?", member.Label),
                        Answer:     fmt.Sprintf("%s is grouped with: %s", member.Label, strings.Join(others, ", ")),
                        Source:     "community_neighbor",
                        SourceID:   member.ID,
                        Confidence: 0.85,
                    })
                }
            }
        }
    }
    
    return pairs, nil
}
```

## Generators

### TemplateGenerator

Generates paraphrase variations without LLM.

```go
type TemplateGenerator struct {
    templates  []QuestionTemplate
    config     TemplateGeneratorConfig
}

type QuestionTemplate struct {
    Pattern     string   // "What is {entity}?"
    Variations  []string // ["Describe {entity}", "Tell me about {entity}"]
    AnswerForm  string   // "definition" | "list" | "explanation"
}

type TemplateGeneratorConfig struct {
    Variations       int
    IncludeNegatives bool
}

func (t *TemplateGenerator) Generate(pairs []RawPair) []RawPair {
    result := []RawPair{}
    
    for _, pair := range pairs {
        // Keep original
        result = append(result, pair)
        
        // Find matching template
        template := t.findTemplate(pair.Question)
        if template == nil {
            continue
        }
        
        // Generate variations
        entity := t.extractEntity(pair.Question, template.Pattern)
        for i, variation := range template.Variations {
            if i >= t.config.Variations {
                break
            }
            result = append(result, RawPair{
                Question:   strings.Replace(variation, "{entity}", entity, 1),
                Answer:     pair.Answer,
                Source:     pair.Source + "_variation",
                SourceID:   pair.SourceID,
                Confidence: pair.Confidence * 0.95, // Slightly lower
            })
        }
        
        // Generate negative if configured
        if t.config.IncludeNegatives {
            result = append(result, t.generateNegative(pair)...)
        }
    }
    
    return result
}
```

### LLMGenerator

Generates higher-quality variations and augmentations. **Requires LLM.**

```go
type LLMGenerator struct {
    llm        LLMClient
    config     LLMGeneratorConfig
}

type LLMGeneratorConfig struct {
    Model       string
    Temperature float64
    BatchSize   int
}

func (l *LLMGenerator) Generate(ctx context.Context, pairs []RawPair) ([]RawPair, error) {
    if l.llm == nil {
        return pairs, nil // Pass through if no LLM
    }
    
    result := make([]RawPair, 0, len(pairs)*2)
    result = append(result, pairs...) // Keep originals
    
    // Process in batches
    for i := 0; i < len(pairs); i += l.config.BatchSize {
        batch := pairs[i:min(i+l.config.BatchSize, len(pairs))]
        
        augmented, err := l.augmentBatch(ctx, batch)
        if err != nil {
            continue // Don't fail, just skip batch
        }
        
        result = append(result, augmented...)
    }
    
    return result, nil
}

func (l *LLMGenerator) augmentBatch(ctx context.Context, batch []RawPair) ([]RawPair, error) {
    // Build prompt for paraphrasing
    prompt := `Paraphrase each question while keeping the same meaning. 
Keep answers unchanged. Output in same format.

`
    for i, pair := range batch {
        prompt += fmt.Sprintf("Q%d: %s\nA%d: %s\n\n", i+1, pair.Question, i+1, pair.Answer)
    }
    
    prompt += "Paraphrased versions:\n"
    
    response, err := l.llm.Complete(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    return parseQAPairs(response)
}
```

## Filters

### DeduplicationFilter

Uses semembed to remove semantic duplicates.

```go
type DeduplicationFilter struct {
    embedder   Embedder  // semembed component
    config     DedupConfig
}

type DedupConfig struct {
    SimilarityThreshold   float64
    PreferHumanValidated  bool
}

func (d *DeduplicationFilter) Filter(ctx context.Context, pairs []RawPair) ([]RawPair, error) {
    if len(pairs) == 0 {
        return pairs, nil
    }
    
    // Embed all questions
    questions := make([]string, len(pairs))
    for i, p := range pairs {
        questions[i] = p.Question
    }
    
    embeddings, err := d.embedder.EmbedBatch(ctx, questions)
    if err != nil {
        // Fallback to exact string matching if embedding fails
        return d.exactDedup(pairs), nil
    }
    
    // Find duplicates by cosine similarity
    kept := []RawPair{}
    keptEmbeddings := [][]float32{}
    
    for i, pair := range pairs {
        isDuplicate := false
        
        for j, keptEmb := range keptEmbeddings {
            similarity := cosineSimilarity(embeddings[i], keptEmb)
            if similarity > d.config.SimilarityThreshold {
                // It's a duplicate - decide which to keep
                if d.config.PreferHumanValidated {
                    if pair.Source == "feedback" && kept[j].Source != "feedback" {
                        // Replace with human-validated version
                        kept[j] = pair
                        keptEmbeddings[j] = embeddings[i]
                    }
                }
                isDuplicate = true
                break
            }
        }
        
        if !isDuplicate {
            kept = append(kept, pair)
            keptEmbeddings = append(keptEmbeddings, embeddings[i])
        }
    }
    
    return kept, nil
}
```

### ClusterSamplingFilter

Uses semembed to ensure training diversity.

```go
type ClusterSamplingFilter struct {
    embedder   Embedder
    config     ClusterConfig
}

type ClusterConfig struct {
    TargetClusters    int
    SamplesPerCluster int
    EnsureCoverage    bool
}

func (c *ClusterSamplingFilter) Filter(ctx context.Context, pairs []RawPair) ([]RawPair, error) {
    if len(pairs) <= c.config.TargetClusters * c.config.SamplesPerCluster {
        return pairs, nil // Not enough to need sampling
    }
    
    // Embed all questions
    questions := make([]string, len(pairs))
    for i, p := range pairs {
        questions[i] = p.Question
    }
    
    embeddings, err := c.embedder.EmbedBatch(ctx, questions)
    if err != nil {
        return pairs, nil // Return all if embedding fails
    }
    
    // K-means clustering
    clusters := kmeans(embeddings, c.config.TargetClusters)
    
    // Sample from each cluster
    result := []RawPair{}
    for clusterID, memberIndices := range clusters {
        // Sort by confidence within cluster
        sort.Slice(memberIndices, func(i, j int) bool {
            return pairs[memberIndices[i]].Confidence > pairs[memberIndices[j]].Confidence
        })
        
        // Take top N from cluster
        count := min(len(memberIndices), c.config.SamplesPerCluster)
        if c.config.EnsureCoverage && count == 0 && len(memberIndices) > 0 {
            count = 1 // At least one from each cluster
        }
        
        for i := 0; i < count; i++ {
            result = append(result, pairs[memberIndices[i]])
        }
    }
    
    return result, nil
}
```

### QualityFilter

Enforces basic quality constraints.

```go
type QualityFilter struct {
    config QualityConfig
}

type QualityConfig struct {
    MinQuestionLength       int
    MaxQuestionLength       int
    MinAnswerLength         int
    MaxAnswerLength         int
    RequireCompleteSentences bool
}

func (q *QualityFilter) Filter(pairs []RawPair) []RawPair {
    result := []RawPair{}
    
    for _, pair := range pairs {
        // Length checks
        if len(pair.Question) < q.config.MinQuestionLength ||
           len(pair.Question) > q.config.MaxQuestionLength {
            continue
        }
        if len(pair.Answer) < q.config.MinAnswerLength ||
           len(pair.Answer) > q.config.MaxAnswerLength {
            continue
        }
        
        // Sentence completion check
        if q.config.RequireCompleteSentences {
            if !endsWithPunctuation(pair.Answer) {
                continue
            }
        }
        
        result = append(result, pair)
    }
    
    return result
}
```

## Output Format

### Training Pair Structure

```go
type TrainingPair struct {
    // Core fields (required for training)
    Instruction string `json:"instruction"`
    Response    string `json:"response"`
    
    // Metadata (for tracking and debugging)
    Source      string    `json:"source"`       // "entity", "relationship", etc.
    SourceID    string    `json:"source_id"`    // Original record ID
    Confidence  float64   `json:"confidence"`   // 0.0-1.0
    GeneratedAt time.Time `json:"generated_at"`
    
    // Optional context (for retrieval-augmented training)
    Context     string    `json:"context,omitempty"` // Retrieved context if any
}
```

### Output Files

```text
TRAINING_DATA/
├── pairs.jsonl           # All generated pairs
├── train.jsonl           # Training split (85%)
├── valid.jsonl           # Validation split (10%)
├── test.jsonl            # Held-out test split (5%)
├── metadata.json         # Export metadata
└── embeddings.npy        # Cached embeddings (optional)
```

### metadata.json

```json
{
  "export_id": "export-20250122-143052",
  "exported_at": "2025-01-22T14:30:52Z",
  "source_counts": {
    "entity": 1250,
    "relationship": 3420,
    "rule": 89,
    "community": 456,
    "query_log": 2103,
    "document_synthetic": 890
  },
  "total_pairs": 8208,
  "splits": {
    "train": 6977,
    "valid": 821,
    "test": 410
  },
  "filters_applied": {
    "deduplication": {
      "removed": 423
    },
    "quality": {
      "removed": 156
    },
    "clustering": {
      "removed": 0
    }
  },
  "config_hash": "sha256:abc123...",
  "previous_export": "export-20250115-020000"
}
```

## NATS Integration

### Subjects

| Subject | Purpose |
|---------|---------|
| `training.export.request` | Trigger export |
| `training.export.status` | Progress updates |
| `training.export.complete` | Export finished |
| `training.pairs.new` | New pairs generated (streaming) |
| `training.feedback.positive` | Explicit positive feedback |
| `training.feedback.negative` | Explicit negative feedback |

### Buckets

| Bucket | Purpose |
|--------|---------|
| `TRAINING_DATA` | Generated training pairs |
| `TRAINING_STATE` | Export state, last run timestamp |
| `TRAINING_FEEDBACK` | Human feedback queue |

## API

### Trigger Export

```bash
# Request export
nats pub training.export.request '{
  "include_synthetic": true,
  "min_confidence": 0.7,
  "format": "jsonl"
}'

# Stream: training.export.status
# {"phase": "extracting", "source": "entities", "count": 1250}
# {"phase": "extracting", "source": "relationships", "count": 3420}
# {"phase": "generating", "variations": 2100}
# {"phase": "filtering", "dedup_removed": 423}
# {"phase": "writing", "total": 8208}

# Complete: training.export.complete
# {"export_id": "export-20250122-143052", "total_pairs": 8208}
```

### Query Training Data

```bash
# Get export metadata
nats kv get TRAINING_STATE latest

# Get specific pair
nats kv get TRAINING_DATA pairs/train/00001.json

# Stream training pairs (for external trainer)
nats kv watch TRAINING_DATA "pairs/train/*"
```

### Submit Feedback

```bash
# Positive feedback on a query result
nats pub training.feedback.positive '{
  "query_id": "q-12345",
  "response_id": "r-67890"
}'

# Negative feedback
nats pub training.feedback.negative '{
  "query_id": "q-12345",
  "reason": "incorrect_entity"
}'

# Submit curated pair directly
nats pub training.pairs.curated '{
  "instruction": "What is the standard sampling depth for CTD profiles?",
  "response": "Standard CTD profiles are conducted to 500m depth, with bottle samples at 10, 25, 50, 100, 200, 300, and 500m."
}'
```

## Integration with slm-trainer

The `training-processor` outputs data; `slm-trainer` consumes it for fine-tuning.

```yaml
# slm-trainer configuration
trainer:
  input:
    bucket: "TRAINING_DATA"
    train_file: "train.jsonl"
    valid_file: "valid.jsonl"
    
  model:
    base: "meta-llama/Llama-3.2-8B"
    adapter: "qlora"
    
  qlora:
    r: 64
    alpha: 16
    dropout: 0.1
    target_modules: ["q_proj", "v_proj", "k_proj", "o_proj"]
    
  training:
    epochs: 3
    batch_size: 4
    gradient_accumulation: 4
    learning_rate: 2e-4
    warmup_ratio: 0.03
    
  output:
    adapter_path: "/models/adapters/ocean-v{version}"
    push_to_bucket: "MODEL_ADAPTERS"
```

## Metrics

| Metric | Description |
|--------|-------------|
| `training_pairs_generated_total` | Total pairs by source |
| `training_pairs_filtered_total` | Pairs removed by filter |
| `training_export_duration_seconds` | Export time |
| `training_embedding_latency_seconds` | Embedding generation time |
| `training_llm_tokens_used` | LLM tokens for synthetic generation |
| `training_feedback_positive_total` | Positive feedback count |
| `training_feedback_negative_total` | Negative feedback count |

## Error Handling

| Error | Handling |
|-------|----------|
| LLM unavailable | Skip LLM-dependent extractors, continue with deterministic |
| Embedding unavailable | Fall back to exact string dedup, skip clustering |
| Insufficient pairs | Abort export if below minimum threshold |
| Export in progress | Queue request, don't run concurrent exports |
| Storage full | Alert, pause export, await manual intervention |

## Summary

The training processor provides:

1. **Three autonomy levels:** Fully autonomous (Tier 0/1), LLM-assisted (Tier 2), Human-in-loop
2. **Graceful degradation:** Works without LLM or embeddings, just with lower quality
3. **Embedding integration:** Uses semembed for quality (dedup, clustering), not generation
4. **Incremental processing:** Only processes new data since last export
5. **Research-ready:** Outputs include splits, metadata, and confidence scores for experiments
