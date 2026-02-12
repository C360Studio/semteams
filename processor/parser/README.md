# Parser Component

Data format parsing utilities for converting raw bytes into structured maps.

## Purpose

The parser component provides lightweight format parsers that transform raw byte data into `map[string]any`
structures for StreamKit message processing. Each parser implements validation, parsing, and format
identification for specific data formats. Currently includes production-ready JSON parsing; CSV parser
is a placeholder implementation.

## Key Types and Interfaces

### JSONParser

Parses JSON data into structured maps with full validation.

```go
type JSONParser struct{}

// Parse converts JSON bytes to map[string]any
func (p *JSONParser) Parse(data []byte) (map[string]any, error)

// Validate checks if data is valid JSON
func (p *JSONParser) Validate(data []byte) error

// Format returns "json"
func (p *JSONParser) Format() string
```

### CSVParser

Placeholder implementation that returns unparsed data.

```go
type CSVParser struct{}

// Parse returns placeholder map with parsed: false
func (p *CSVParser) Parse(data []byte) (map[string]any, error)

// Validate performs basic empty check
func (p *CSVParser) Validate(data []byte) error

// Format returns "csv"
func (p *CSVParser) Format() string
```

### Error Types

```go
var (
    ErrInvalidFormat = errors.New("invalid data format")
    ErrEmptyData     = errors.New("empty data")
    ErrParsingFailed = errors.New("parsing failed")
)
```

## Example Usage

### JSON Parsing

```go
import "github.com/c360studio/semstreams/processor/parser"

// Create parser
p := parser.NewJSONParser()

// Validate before parsing
data := []byte(`{"sensor": "temp-001", "value": 23.5}`)
if err := p.Validate(data); err != nil {
    return fmt.Errorf("validation failed: %w", err)
}

// Parse to structured map
result, err := p.Parse(data)
if err != nil {
    return fmt.Errorf("parse failed: %w", err)
}

// Access parsed data
sensorID := result["sensor"].(string)    // "temp-001"
value := result["value"].(float64)       // 23.5
```

### Nested JSON

```go
data := []byte(`{
  "sensor": "temp-001",
  "reading": {
    "value": 23.5,
    "unit": "celsius"
  }
}`)

result, err := p.Parse(data)
if err != nil {
    return err
}

// Navigate nested structure
reading := result["reading"].(map[string]any)
value := reading["value"].(float64)      // 23.5
unit := reading["unit"].(string)         // "celsius"
```

### JSON Arrays

```go
data := []byte(`{
  "sensor": "multi-001",
  "readings": [21.5, 22.0, 23.5]
}`)

result, err := p.Parse(data)
if err != nil {
    return err
}

readings := result["readings"].([]any)
first := readings[0].(float64)           // 21.5
```

### Integration with GenericJSON

```go
import (
    "github.com/c360studio/semstreams/message"
    "github.com/c360studio/semstreams/processor/parser"
)

// Parse raw JSON
p := parser.NewJSONParser()
data := []byte(`{"sensor": "temp-001", "value": 23.5}`)
parsed, err := p.Parse(data)
if err != nil {
    return err
}

// Wrap in GenericJSON message
payload := message.NewGenericJSON(parsed)
// Ready for StreamKit processing
```

### CSV Parser Warning

The CSV parser is NOT production-ready. It returns a placeholder map:

```go
p := parser.NewCSVParser()
data := []byte("header1,header2\nval1,val2")
result, err := p.Parse(data)
// result: map[string]any{
//   "format": "csv",
//   "raw": "header1,header2\nval1,val2",
//   "parsed": false,  // Indicates placeholder
// }
```

Do not use CSV parser in production deployments.

## Type Mapping

JSON types map to Go types as follows:

| JSON Type | Go Type |
|-----------|---------|
| string | `string` |
| number | `float64` |
| boolean | `bool` |
| null | `nil` |
| object | `map[string]any` |
| array | `[]any` |

## Performance

- Uses `encoding/json` from standard library
- Processes ~100,000 small objects/second
- Minimal allocations for simple objects
- Suitable for high-throughput message processing

## Testing

Run parser tests:

```bash
go test ./processor/parser -v
```

JSON parser has 100% test coverage including validation, parsing, error conditions, and type preservation.
