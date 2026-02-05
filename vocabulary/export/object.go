package export

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/c360studio/semstreams/message"
)

// XSD datatype IRIs
const (
	xsdString   = "http://www.w3.org/2001/XMLSchema#string"
	xsdInteger  = "http://www.w3.org/2001/XMLSchema#integer"
	xsdDouble   = "http://www.w3.org/2001/XMLSchema#double"
	xsdBoolean  = "http://www.w3.org/2001/XMLSchema#boolean"
	xsdDateTime = "http://www.w3.org/2001/XMLSchema#dateTime"
)

// objectKind classifies how an object value should be serialized.
type objectKind int

const (
	objectLiteral  objectKind = iota // Quoted literal with optional datatype
	objectResource                   // IRI reference (entity or external)
	objectInvalid                    // Cannot be serialized
)

// classifiedObject holds the result of classifying a Triple's Object.
type classifiedObject struct {
	kind     objectKind
	iri      string // For objectResource: the resolved IRI
	lexical  string // For objectLiteral: the lexical form (unescaped)
	datatype string // For objectLiteral: XSD datatype IRI (empty for plain string)
	bare     bool   // True if Turtle can emit without quotes (int, bool)
}

// classifyObject inspects a Triple's Object and Datatype fields and returns
// a classified representation suitable for serialization.
func classifyObject(t message.Triple, opts *options) classifiedObject {
	if t.Object == nil {
		return classifiedObject{kind: objectInvalid}
	}

	// If Triple.Datatype is set, it overrides type inference
	if t.Datatype != "" {
		return classifyWithExplicitDatatype(t, opts)
	}

	return classifyByGoType(t.Object, opts)
}

// classifyWithExplicitDatatype handles triples where Datatype is explicitly set.
// When the user sets Datatype, we respect it unconditionally — even for strings
// that look like entity IDs. The explicit Datatype declaration takes precedence.
func classifyWithExplicitDatatype(t message.Triple, _ *options) classifiedObject {
	return classifiedObject{
		kind:     objectLiteral,
		lexical:  fmt.Sprintf("%v", t.Object),
		datatype: expandDatatypePrefix(t.Datatype),
	}
}

// classifyByGoType infers the serialization from the Go type of the object.
func classifyByGoType(obj any, opts *options) classifiedObject {
	switch v := obj.(type) {
	case string:
		return classifyString(v, opts)
	case bool:
		return classifiedObject{
			kind:     objectLiteral,
			lexical:  fmt.Sprintf("%t", v),
			datatype: xsdBoolean,
			bare:     true,
		}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return integerLiteral(v)
	case float32:
		return classifyFloat(float64(v))
	case float64:
		return classifyFloat(v)
	case time.Time:
		return classifiedObject{
			kind:     objectLiteral,
			lexical:  v.Format(time.RFC3339),
			datatype: xsdDateTime,
		}
	default:
		return classifiedObject{
			kind:    objectLiteral,
			lexical: fmt.Sprintf("%v", v),
		}
	}
}

func integerLiteral(v any) classifiedObject {
	return classifiedObject{
		kind:     objectLiteral,
		lexical:  fmt.Sprintf("%d", v),
		datatype: xsdInteger,
		bare:     true,
	}
}

func classifyFloat(f float64) classifiedObject {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return classifiedObject{kind: objectInvalid}
	}
	return classifiedObject{
		kind:     objectLiteral,
		lexical:  formatFloat(f),
		datatype: xsdDouble,
	}
}

// classifyString determines if a string is an entity reference or a literal.
func classifyString(s string, opts *options) classifiedObject {
	if message.IsValidEntityID(s) {
		return classifiedObject{
			kind: objectResource,
			iri:  resolveSubjectIRI(s, opts),
		}
	}
	return classifiedObject{
		kind:    objectLiteral,
		lexical: s,
		// xsd:string is the default and omitted in output
	}
}

// formatFloat renders a float64 for RDF output.
// It avoids scientific notation for common values and ensures a decimal point.
func formatFloat(f float64) string {
	s := fmt.Sprintf("%g", f)
	// Ensure it has a decimal point for clarity
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

// escapeTurtleString escapes a string for Turtle/N-Triples string literals.
func escapeTurtleString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// expandDatatypePrefix expands common xsd: prefixed datatypes to full IRIs.
func expandDatatypePrefix(dt string) string {
	if strings.HasPrefix(dt, "xsd:") {
		return "http://www.w3.org/2001/XMLSchema#" + dt[4:]
	}
	// Already a full IRI or other prefix
	if strings.Contains(dt, "://") {
		return dt
	}
	// Unknown prefix — return as-is for now
	return dt
}
