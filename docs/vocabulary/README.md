# Vocabulary & Ontology Standards

SemStreams uses a layered ontology architecture that maps internal dotted-notation predicates to established semantic web standards at API boundaries.

## Ontology Layer Architecture

```text
┌─────────────────────────────────────────────────┐
│  Domain Standards                               │
│  PROV-O · SSN/SOSA · S-Agent-Comm · Schema.org  │
├─────────────────────────────────────────────────┤
│  CCO (Common Core Ontologies)                   │
│  Agents · Actions · Information · Artifacts      │
├─────────────────────────────────────────────────┤
│  BFO 2.0 (Basic Formal Ontology)               │
│  Entity · Continuant · Occurrent                │
└─────────────────────────────────────────────────┘
```

- **BFO** provides the most general categories (what kinds of things exist)
- **CCO** adds reusable mid-level concepts (agents, plans, information)
- **Domain standards** provide specific vocabularies for provenance, sensors, agents, and more

## Subpackages

### `vocabulary/bfo` - Basic Formal Ontology

IRI constants for BFO 2.0 (ISO 21838-2), the upper-level ontology that classifies all entities into continuants (things that persist) and occurrents (things that happen).

```go
import "github.com/c360studio/semstreams/vocabulary/bfo"

bfo.Object          // Material objects (drones, sensors)
bfo.Process         // Activities that unfold in time
bfo.Role            // Roles borne by entities
bfo.Quality         // Measurable properties
```

### `vocabulary/cco` - Common Core Ontologies

IRI constants for CCO, a mid-level ontology built on BFO for modeling agents, actions, information entities, and artifacts.

```go
import "github.com/c360studio/semstreams/vocabulary/cco"

cco.Agent                       // Entities that can act
cco.IntelligentSoftwareAgent    // AI/software agents
cco.PlanSpecification           // Mission plans, workflows
cco.InformationContentEntity    // Documents, data records
cco.Capability                  // Agent abilities
```

### `vocabulary/agentic` - W3C S-Agent-Comm

Predicates and IRI constants for AI agent interoperability, aligned with the W3C Semantic Agent Communication Community Group ontology.

```go
import "github.com/c360studio/semstreams/vocabulary/agentic"

agentic.IntentGoal          // Agent goals and objectives
agentic.CapabilityName      // Agent skills and abilities
agentic.DelegationFrom      // Authority delegation chains
agentic.AccountabilityActor // Responsibility attribution
```

See [Agentic Vocabulary](agentic.md) for detailed predicate tables and usage patterns.

### `vocabulary/export` - RDF Serialization

Serializes `[]message.Triple` to standard RDF formats, using the vocabulary registry for predicate IRI resolution and `message.ParseEntityID` for subject IRIs.

```go
import "github.com/c360studio/semstreams/vocabulary/export"

export.Serialize(w, triples, export.Turtle)    // Turtle with prefix declarations
export.Serialize(w, triples, export.NTriples)  // One triple per line
export.Serialize(w, triples, export.JSONLD)    // JSON with @context
```

Options: `export.WithBaseIRI(...)`, `export.WithSubjectIRIFunc(...)`

## Standards in `standards.go`

The root `vocabulary` package provides IRI constants for widely used semantic web standards:

| Standard | Purpose | Example Constant |
|----------|---------|-----------------|
| OWL | Equivalence, property types | `vocabulary.OwlSameAs` |
| SKOS | Labels, concept hierarchies | `vocabulary.SkosPrefLabel` |
| Dublin Core | Metadata terms, relations | `vocabulary.DcIdentifier` |
| PROV-O | Provenance (entities, activities, agents) | `vocabulary.ProvWasDerivedFrom` |
| SSN/SOSA | Sensor observations, deployments | `vocabulary.SosaObserves` |
| Schema.org | Common web vocabulary | `vocabulary.SchemaName` |
| FOAF | People and accounts | `vocabulary.FoafAccountName` |

## How It Works

Internally, SemStreams always uses dotted-notation predicates for NATS compatibility:

```go
// Internal: dotted notation
triple := message.Triple{
    Subject:   entityID,
    Predicate: "dc.terms.title",  // dotted notation
    Object:    "Mission Report",
}
```

At API boundaries (RDF export, SPARQL queries), predicates translate to standard IRIs:

```go
// External: IRI for standards compliance
meta := vocabulary.GetPredicateMetadata("dc.terms.title")
// meta.StandardIRI == "http://purl.org/dc/terms/title"
```

## Documentation

- [Vocabulary Guide](../basics/04-vocabulary.md) - Predicate design, registration, alias resolution
- [Agentic Vocabulary](agentic.md) - W3C S-Agent-Comm predicates for agent interoperability
- [Vocabulary Package API](../../vocabulary/README.md) - Registry API, functional options, complete reference
