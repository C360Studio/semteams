# Tiered-Statistical Scenario Review

**Reviewed**: 2025-12-20  
**File**: `test/e2e/scenarios/tiered.go`  
**Status**: Moderate Coverage with Significant Gaps

---

## Scenario Overview

The `tiered --variant statistical` scenario validates BM25 lexical embeddings and community detection without external ML services. This is Tier 1 - adds search capabilities on top of structural tier.

**Duration**: ~60 seconds  
**Tier**: Statistical (Tier 1)  
**Dependencies**: NATS only (no GPU, no external ML)

---

## What's Tested

### Stages for Statistical Variant

| Stage | Purpose | Assertion Strength |
|-------|---------|-------------------|
| `verify-components` | Check required components exist | Strong |
| `send-mixed-data` | Send telemetry + regular messages | Medium |
| `validate-processing` | Verify graph processor running | Medium |
| `verify-entity-count` | Count entities in ENTITY_STATES | Medium |
| `verify-entity-retrieval` | Retrieve specific known entities | Medium |
| `validate-entity-structure` | Validate entity data structure | Medium |
| `verify-index-population` | Check 7 indexes populated | Strong |
| `test-semantic-search` | Test semantic search endpoint | Medium |
| `verify-search-quality` | Validate search result quality | Medium |
| `test-http-gateway` | Test HTTP gateway search | Medium |
| `test-embedding-fallback` | Verify BM25 fallback works | Medium |
| `compare-statistical-semantic` | Capture comparison data | Weak (data capture only) |
| `validate-rules` | Verify rules evaluated/triggered | Medium |
| `validate-metrics` | Check Prometheus metrics exposed | Strong |
| `verify-outputs` | Verify output components exist | Medium |

---

## Correctness Assessment

### Correct

1. **Semantic search testing** (`tiered.go:656-746`)
   - Sends test messages with descriptions
   - Waits for embeddings using event-driven polling
   - Verifies embedding metrics present

2. **Search quality verification** (`tiered.go:1526-1650`)
   - Tests 4 natural language queries
   - Validates minimum score thresholds (0.3)
   - Validates minimum hit counts
   - Calculates overall average score

3. **HTTP gateway testing** (`tiered.go:748-829`)
   - Tests POST to `/api-gateway/search/semantic`
   - Validates 200 OK response
   - Parses and counts hits

4. **BM25 fallback verification** (`tiered.go:831-898`)
   - Checks if semembed unavailable
   - Verifies graph processor healthy regardless
   - Confirms BM25 fallback mode active

### Issues Found

| Issue | Severity | Description |
|-------|----------|-------------|
| GraphRAG not tested | **High** | GraphRAG is Tier 1 feature but has no test |
| Community structure not validated | **High** | Communities detected but structure not verified |
| BM25 embedding generation not validated | **Medium** | Just checks metrics exist, not that embeddings work |
| Search quality thresholds very lenient | **Medium** | min_score=0.3 is low, avg threshold=0.5 |
| No community query tests | **Medium** | Community detection runs but communities not queried |
| Comparison stage just captures data | **Low** | `compare-statistical-semantic` doesn't compare, just saves |

---

## Gap Analysis

### Critical Gap: GraphRAG Not Tested

GraphRAG is documented as a Tier 1+ feature:

From `docs/concepts/09-graphrag-pattern.md`:
> "Requires: Communities (Tier 1+)"
> "Global Search: Searches across all communities by topic"
> "Local Search: Starts from a known entity and explores its community"

**No e2e test validates GraphRAG queries.** This means:
- Community-based search is untested
- Local search (entity → community context) is untested
- Global search (topic → communities) is untested
- LLM context assembly is untested (though that's Tier 2)

### Critical Gap: Community Structure Not Validated

The test verifies community detection *runs* but doesn't verify:
- Communities actually formed
- Community membership makes sense
- Community summaries generated
- PageRank representatives identified
- Hierarchical levels (0, 1, 2) work

### Features Documented but Not Tested

| Feature | Documentation Source | Gap |
|---------|---------------------|-----|
| **GraphRAG local search** | `docs/concepts/09-graphrag-pattern.md` | No test exists |
| **GraphRAG global search** | `docs/concepts/09-graphrag-pattern.md` | No test exists |
| Community membership query | Architecture docs | Not tested |
| Community summary content | Should have TF-IDF keywords | Not validated |
| PageRank representatives | Community detection identifies hubs | Not validated |
| Hierarchical community levels | Level 0, 1, 2 documented | Only level 0 |
| BM25 embedding correctness | Lexical matching should work | Only metrics checked |
| Spatial index queries | SPATIAL_INDEX populated | Not queried |
| Temporal index queries | TEMPORAL_INDEX populated | Not queried |

---

## Search Quality Analysis

The test runs 4 search queries:

```go
{"What documents mention forklift safety?", "forklift", 0.3, 1}
{"Are there safety observations related to temperature?", "temperature", 0.3, 1}
{"What maintenance was done on cold storage equipment?", "cold", 0.3, 1}
{"Find all sensors in zone-a", "zone-a", 0.3, 1}
```

**Issues**:
1. **Pattern matching is loose** - Just checks if expected pattern appears in ANY result entity ID
2. **Score threshold too low** - 0.3 is very lenient for relevance
3. **Hit count threshold too low** - minHits=1 means almost anything passes
4. **No ranking validation** - Doesn't verify best match is first
5. **No negative cases** - Doesn't verify unrelated queries return low scores

---

## Recommendations

### Priority: High (Must Fix)

1. **Add GraphRAG e2e test**
   
   Create tests for:
   - Local search: Start from known entity, verify community context returned
   - Global search: Topic query, verify relevant communities found
   
   ```go
   // Example test structure
   graphRAGTests := []struct {
       searchType      string  // "local" or "global"
       query           string
       startEntity     string  // For local only
       expectedCommunity string
       minMembers      int
   }{
       {
           searchType:  "local",
           startEntity: "c360.logistics.sensor.document.temperature.sensor-temp-001",
           expectedCommunity: "sensor-temperature-*",
           minMembers: 3,
       },
       {
           searchType: "global",
           query:      "temperature monitoring",
           expectedCommunity: "sensor-*",
           minMembers: 5,
       },
   }
   ```

2. **Validate community structure**
   
   Add stage to query COMMUNITY_INDEX and verify:
   - Communities exist (count > 0)
   - Non-singleton communities exist (members > 1)
   - Summaries have keywords
   - Representative entities identified

### Priority: Medium (Should Fix)

3. **Increase search quality thresholds**
   - Raise min_score from 0.3 to 0.5 for meaningful relevance
   - Raise min_hits to at least 2 for pattern-based queries
   - Add ranking validation (top result should match pattern)

4. **Test spatial and temporal indexes**
   - Execute spatial query (bounding box)
   - Execute temporal query (time range)
   - Verify results match expected criteria

5. **Validate BM25 embedding content**
   - Don't just check metric exists
   - Query an entity's embedding
   - Verify it's a valid BM25 vector

### Priority: Low (Nice to Have)

6. **Add negative search cases**
   - Query for something not in test data
   - Verify low/zero scores returned

7. **Test hierarchical community levels**
   - Query level 0 communities (fine-grained)
   - Query level 1 communities (coarser)
   - Verify hierarchy makes sense

---

## Feature Coverage Matrix

| Documented Feature | Tested? | Notes |
|--------------------|---------|-------|
| BM25 embeddings | Partial | Metrics checked, content not verified |
| Community detection | Partial | Runs, but structure not validated |
| Semantic search | Yes | 4 queries with lenient thresholds |
| Search quality | Partial | Basic checks, thresholds too low |
| HTTP gateway search | Yes | Status and hit count checked |
| BM25 fallback | Yes | Mode detection verified |
| **GraphRAG local search** | **No** | Critical gap |
| **GraphRAG global search** | **No** | Critical gap |
| **Community queries** | **No** | COMMUNITY_INDEX not queried |
| Community summaries | **No** | TF-IDF keywords not verified |
| PageRank representatives | **No** | Not validated |
| Hierarchical levels | **No** | Only level 0 |
| Spatial index queries | **No** | Index populated but not queried |
| Temporal index queries | **No** | Index populated but not queried |
| PathRAG | **No** | Should work at Tier 1 too |

---

## Test Configuration

```go
// Statistical tier uses default config
ValidationTimeout:    30 * time.Second,
PollInterval:         100 * time.Millisecond,
MinProcessed:         10,
MinExpectedEntities:  50,
```

---

## Comparison Stage Analysis

The `compare-statistical-semantic` stage captures query results for later analysis:

```go
// From tiered.go:1680-1706
func (s *TieredScenario) executeCompareStatisticalSemantic(ctx context.Context, result *Result) error {
    info := s.detectVariantAndProvider(result)
    queryResults := s.runComparisonQueries(ctx)
    comparisonFile := s.persistComparisonResults(info, queryResults.searchResults, result)
    // ...
}
```

**Issue**: This stage only *captures* data, it doesn't *compare* anything. The comparison is meant to be done externally by comparing JSON files from statistical vs semantic runs.

**Recommendation**: Either rename to `capture-comparison-data` or implement actual comparison logic.

---

## Conclusion

**Overall Assessment**: The `tiered --variant statistical` scenario provides **basic validation** of BM25 search functionality but has **significant gaps** in testing GraphRAG patterns and community structure. The search quality thresholds are lenient, potentially allowing weak search implementations to pass.

**Critical Gaps**:
1. GraphRAG (local and global search) not tested
2. Community structure not validated
3. Spatial/temporal index queries not tested

**Risk**: GraphRAG could be broken, community detection could produce meaningless clusters, and spatial/temporal queries could fail - all without this test detecting the issues.

**Recommendations**:
1. **High priority**: Add GraphRAG e2e tests
2. **High priority**: Validate community structure
3. **Medium priority**: Increase search quality thresholds
4. **Medium priority**: Test spatial/temporal index queries
