package trustgraph

// TGTriple is TrustGraph's compact triple representation.
// This is the wire format used in TrustGraph REST APIs.
type TGTriple struct {
	S TGValue `json:"s"`
	P TGValue `json:"p"`
	O TGValue `json:"o"`
}

// TGValue is a compact value with entity flag.
// When E is true, V contains a URI (entity reference).
// When E is false, V contains a literal value.
type TGValue struct {
	V string `json:"v"` // Value (URI or literal)
	E bool   `json:"e"` // Is entity (true = URI, false = literal)
}

// NewEntityValue creates a TGValue representing an entity (URI).
func NewEntityValue(uri string) TGValue {
	return TGValue{V: uri, E: true}
}

// NewLiteralValue creates a TGValue representing a literal.
func NewLiteralValue(value string) TGValue {
	return TGValue{V: value, E: false}
}
