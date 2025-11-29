# Quickstart: Graph Package Consolidation Migration

**Feature**: 005-graph-package-consolidation
**Date**: 2025-11-29

## Overview

This guide covers the import path and API changes required by the graph package consolidation. Follow this guide to update your code after merging this feature.

## Migration Summary

| Before | After |
|--------|-------|
| `import gtypes "github.com/c360/semstreams/types/graph"` | `import gtypes "github.com/c360/semstreams/graph"` |
| `message.Graphable` | `graph.Graphable` |
| `pkg/graphclustering` | `processor/graph/clustering` |
| `pkg/embedding` | `processor/graph/embedding` |
| `comm.GetID()` | `comm.ID` |
| `graph.FederatedEntity` | Use `message.EntityID` fields |

## Step-by-Step Migration

### 1. Update types/graph Imports

**Files affected**: 9 files in `processor/` directory

```diff
// Before
-import gtypes "github.com/c360/semstreams/types/graph"

// After
+import gtypes "github.com/c360/semstreams/graph"
```

**No code changes needed** - the `gtypes` alias maintains compatibility.

### 2. Update Graphable Interface Usage

**Files affected**: Any file implementing or using `message.Graphable`

```diff
// Before
-import "github.com/c360/semstreams/message"
-
-func processGraphable(g message.Graphable) {
-    // ...
-}

// After
+import "github.com/c360/semstreams/graph"
+
+func processGraphable(g graph.Graphable) {
+    // ...
+}
```

**Interface assertion example**:
```diff
// Before
-if graphable, ok := msg.(message.Graphable); ok {
-    // ...
-}

// After
+if graphable, ok := msg.(graph.Graphable); ok {
+    // ...
+}
```

### 3. Update graphclustering Imports

**Files affected**: Any file importing `pkg/graphclustering`

```diff
// Before
-import "github.com/c360/semstreams/pkg/graphclustering"

// After
+import "github.com/c360/semstreams/processor/graph/clustering"
```

**Package name change**:
```diff
// Before
-graphclustering.NewLPA(...)

// After
+clustering.NewLPA(...)
```

### 4. Replace Getter Methods with Direct Field Access

**Files affected**: Any file using Community getter methods

```diff
// Before
-id := community.GetID()
-level := community.GetLevel()
-members := community.GetMembers()
-keywords := community.GetKeywords()
-summary := community.GetStatisticalSummary()

// After
+id := community.ID
+level := community.Level
+members := community.Members
+keywords := community.Keywords
+summary := community.StatisticalSummary
```

**Full getter→field mapping**:
| Getter Method | Direct Field |
|---------------|--------------|
| `GetID()` | `.ID` |
| `GetLevel()` | `.Level` |
| `GetMembers()` | `.Members` |
| `GetParentID()` | `.ParentID` |
| `GetKeywords()` | `.Keywords` |
| `GetRepEntities()` | `.RepEntities` |
| `GetStatisticalSummary()` | `.StatisticalSummary` |
| `GetLLMSummary()` | `.LLMSummary` |
| `GetSummaryStatus()` | `.SummaryStatus` |
| `GetMetadata()` | `.Metadata` |

### 5. Remove graphinterfaces Usage

**Files affected**: Any file importing `pkg/graphinterfaces`

The `graphinterfaces.Community` interface is deleted. Use the concrete `clustering.Community` struct directly:

```diff
// Before
-import "github.com/c360/semstreams/pkg/graphinterfaces"
-
-func processCommunity(comm graphinterfaces.Community) {
-    id := comm.GetID()
-}

// After
+import "github.com/c360/semstreams/processor/graph/clustering"
+
+func processCommunity(comm *clustering.Community) {
+    id := comm.ID
+}
```

### 6. Update embedding Imports

**Files affected**: Any file importing `pkg/embedding` (2 files in indexmanager)

```diff
// Before
-import "github.com/c360/semstreams/pkg/embedding"

// After
+import "github.com/c360/semstreams/processor/graph/embedding"
```

**Note**: Package name remains `embedding`, so no code changes needed beyond the import path.

### 7. Replace FederatedEntity with EntityID

**Files affected**: Any file using `graph.FederatedEntity`

Federation information is now extracted from the 6-part EntityID:

```diff
// Before
-fed := graph.BuildFederatedEntity(localID, msg)
-globalID := fed.GlobalID
-platformID := fed.PlatformID

// After
+eid, _ := message.ParseEntityID(entityID)
+// Federation info is embedded in the ID:
+// org.platform.domain.system.type.instance
+platformID := eid.Platform
+org := eid.Org
```

## Verification

After migration, verify your changes:

```bash
# Check for compilation errors
go build ./...

# Run tests with race detection
go test -race ./...

# Check for any remaining old imports
grep -r "types/graph" --include="*.go" .
grep -r "message.Graphable" --include="*.go" .
grep -r "pkg/graphclustering" --include="*.go" .
grep -r "pkg/graphinterfaces" --include="*.go" .
grep -r "pkg/embedding" --include="*.go" .
grep -r "\.Get[A-Z][a-zA-Z]*\(\)" --include="*.go" . | grep -i community
```

## Common Issues

### Import Cycle After Migration

If you see an import cycle error after updating imports:
1. Check that you're not importing `graph` from within the `message` package
2. The `graph` package imports `message` for `Triple` type - this is the intended direction

### Missing Community Methods

If code fails with "undefined: GetID" or similar:
1. Replace getter calls with direct field access
2. See the getter→field mapping table above

### Type Assertion Failures

If `msg.(graph.Graphable)` fails at runtime:
1. Ensure the implementing type's imports were also updated
2. Rebuild all packages with `go build ./...`

## Package Ownership Reference

After consolidation:

| Package | Owns |
|---------|------|
| `message/` | Transport primitives: EntityID, Triple, FederationMeta |
| `graph/` | Graph contracts and storage: Graphable, EntityState, helpers |
| `processor/graph/clustering/` | Community detection algorithms and types |
| `processor/graph/embedding/` | Vector embedding interfaces and implementations |
