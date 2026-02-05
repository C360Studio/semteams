# RDF Export

Serializes `[]message.Triple` to standard RDF formats using the vocabulary registry for IRI resolution.

## Supported Formats

| Format | Extension | Description |
|--------|-----------|-------------|
| `Turtle` | `.ttl` | Compact, human-readable with prefix declarations and subject grouping |
| `NTriples` | `.nt` | Line-based, one triple per line, fully expanded IRIs |
| `JSONLD` | `.jsonld` | JSON with `@context` and `@graph` for web APIs |

## Usage

```go
import "github.com/c360studio/semstreams/vocabulary/export"

triples := []message.Triple{
    {Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
    {Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.flight.armed", Object: true},
}

// Write Turtle to a writer
err := export.Serialize(os.Stdout, triples, export.Turtle)

// Get N-Triples as a string
s, err := export.SerializeToString(triples, export.NTriples)
```

## IRI Resolution

The package resolves dotted-notation identifiers to IRIs automatically:

**Subjects** are converted based on entity ID structure:

- Valid 6-part entity IDs: `acme.ops.robotics.gcs.drone.001` becomes `{base}/entities/acme/ops/robotics/gcs/drone/001`
- Other subjects: dots are converted to slashes under `{base}/subjects/`

**Predicates** resolve through the vocabulary registry:

- Registered predicates use their `StandardIRI` (e.g., `robotics.battery.level` with a registered IRI maps to that IRI)
- Unregistered predicates generate `{base}/predicates/domain/category/property`

**Objects** are typed by Go type inference:

| Go Type | RDF Datatype | Output |
|---------|-------------|--------|
| `string` (entity ID) | Resource | `<iri>` |
| `string` (other) | `xsd:string` | `"text"` |
| `int`, `int64`, etc. | `xsd:integer` | `42` |
| `float64` | `xsd:double` | `"85.5"^^xsd:double` |
| `bool` | `xsd:boolean` | `true` |
| `time.Time` | `xsd:dateTime` | `"2024-01-15T10:30:00Z"^^xsd:dateTime` |

## Options

### `WithBaseIRI`

Override the default base IRI used for generated subject and predicate URIs:

```go
err := export.Serialize(w, triples, export.Turtle,
    export.WithBaseIRI("https://example.org"))
```

### `WithSubjectIRIFunc`

Provide a custom function to map entity ID strings to IRIs. When set, this replaces the default subject IRI generation entirely (`WithBaseIRI` has no effect on subjects):

```go
err := export.Serialize(w, triples, export.JSONLD,
    export.WithSubjectIRIFunc(func(subject string) string {
        return "https://example.org/entities/" + subject
    }))
```

## Prefix Management

Turtle and JSON-LD output automatically compact IRIs using well-known prefixes (OWL, SKOS, Dublin Core, PROV-O, Schema.org, FOAF, SSN/SOSA, XSD). Only prefixes that appear in the output are declared.

## API

| Function | Description |
|----------|-------------|
| `Serialize(w, triples, format, ...opts)` | Write triples to an `io.Writer` |
| `SerializeToString(triples, format, ...opts)` | Return serialized output as a string |

## Related Documentation

- [Vocabulary Package](../README.md) - Registry API, predicate registration, IRI mappings
- [Vocabulary Guide](../../docs/basics/04-vocabulary.md) - Predicate design and naming conventions
