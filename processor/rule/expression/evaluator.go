// Package expression - Expression evaluator implementation
package expression

import (
	"fmt"
	"math"
	"strings"

	gtypes "github.com/c360/semstreams/types/graph"
)

// NewExpressionEvaluator creates a new expression evaluator with all supported operators
func NewExpressionEvaluator() *Evaluator {
	evaluator := &Evaluator{
		operators:    make(map[string]OperatorFunc),
		typeDetector: &defaultTypeDetector{},
	}

	// Register numeric operators
	evaluator.operators[OpEqual] = operatorEqual
	evaluator.operators[OpNotEqual] = operatorNotEqual
	evaluator.operators[OpLessThan] = operatorLessThan
	evaluator.operators[OpLessThanEqual] = operatorLessThanEqual
	evaluator.operators[OpGreaterThan] = operatorGreaterThan
	evaluator.operators[OpGreaterThanEqual] = operatorGreaterThanEqual

	// Register string operators
	evaluator.operators[OpContains] = operatorContains
	evaluator.operators[OpStartsWith] = operatorStartsWith
	evaluator.operators[OpEndsWith] = operatorEndsWith
	evaluator.operators[OpRegexMatch] = operatorRegex

	return evaluator
}

// Evaluate evaluates a logical expression against an entity state
func (e *Evaluator) Evaluate(entityState *gtypes.EntityState, expr LogicalExpression) (bool, error) {
	if len(expr.Conditions) == 0 {
		return true, nil // Empty condition list passes
	}

	results := make([]bool, len(expr.Conditions))

	// Evaluate each condition
	for i, condition := range expr.Conditions {
		result, err := e.evaluateCondition(entityState, condition)
		if err != nil {
			return false, err
		}
		results[i] = result
	}

	// Apply logic operator
	switch expr.Logic {
	case LogicOr, "": // Default to OR if not specified
		for _, result := range results {
			if result {
				return true, nil
			}
		}
		return false, nil

	case LogicAnd:
		for _, result := range results {
			if !result {
				return false, nil
			}
		}
		return true, nil

	default:
		return false, &EvaluationError{
			Message: fmt.Sprintf("unsupported logic operator: %s", expr.Logic),
		}
	}
}

// evaluateCondition evaluates a single condition against entity state
func (e *Evaluator) evaluateCondition(entityState *gtypes.EntityState, condition ConditionExpression) (bool, error) {
	// Get field value from entity state
	fieldValue, exists, err := e.typeDetector.GetFieldValue(entityState, condition.Field)
	if err != nil {
		return false, &EvaluationError{
			Field:   condition.Field,
			Message: "failed to get field value",
			Err:     err,
		}
	}

	// Handle missing fields based on Required flag
	if !exists {
		if condition.Required {
			// Required field missing - fail fast as requested
			return false, &EvaluationError{
				Field:   condition.Field,
				Message: "required field not found",
			}
		}
		// Optional field missing - condition fails (conservative approach)
		return false, nil
	}

	// Get operator function
	opFunc, exists := e.operators[condition.Operator]
	if !exists {
		return false, &EvaluationError{
			Field:    condition.Field,
			Operator: condition.Operator,
			Message:  "unsupported operator",
		}
	}

	// Execute operator
	result, err := opFunc(fieldValue, condition.Value)
	if err != nil {
		return false, &EvaluationError{
			Field:    condition.Field,
			Operator: condition.Operator,
			Message:  "operator execution failed",
			Err:      err,
		}
	}

	return result, nil
}

// defaultTypeDetector implements TypeDetector using existing triple access functions
type defaultTypeDetector struct{}

// GetFieldValue extracts a field value from entity state using existing helper functions
func (d *defaultTypeDetector) GetFieldValue(entityState *gtypes.EntityState, field string) (interface{}, bool, error) {
	// Import existing functions from entity_state_rule.go
	// We'll need to make them accessible or recreate the logic here
	for _, triple := range entityState.Triples {
		if triple.Predicate == field {
			return triple.Object, true, nil
		}
	}
	return nil, false, nil
}

// DetectFieldType determines the Go type of a field value
func (d *defaultTypeDetector) DetectFieldType(value interface{}) FieldType {
	switch value.(type) {
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return FieldTypeFloat64
	case string:
		return FieldTypeString
	case bool:
		return FieldTypeBool
	case []interface{}:
		return FieldTypeArray
	default:
		return FieldTypeUnknown
	}
}

// Operator implementations

func operatorEqual(fieldValue, compareValue interface{}) (bool, error) {
	return compareValues(fieldValue, compareValue) == 0, nil
}

func operatorNotEqual(fieldValue, compareValue interface{}) (bool, error) {
	return compareValues(fieldValue, compareValue) != 0, nil
}

func operatorLessThan(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp < 0, nil
}

func operatorLessThanEqual(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp <= 0, nil
}

func operatorGreaterThan(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func operatorGreaterThanEqual(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp >= 0, nil
}

func operatorContains(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.Contains(fieldStr, compareStr), nil
}

func operatorStartsWith(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.HasPrefix(fieldStr, compareStr), nil
}

func operatorEndsWith(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.HasSuffix(fieldStr, compareStr), nil
}

func operatorRegex(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	pattern, ok := compareValue.(string)
	if !ok {
		return false, fmt.Errorf("regex pattern must be a string")
	}

	// Use cached regex compilation for better performance
	re, err := compileRegex(pattern)
	if err != nil {
		return false, err
	}

	return re.MatchString(fieldStr), nil
}

// Helper functions for value comparison

func compareValues(a, b interface{}) int {
	result, _ := compareValuesWithError(a, b)
	return result
}

func compareValuesWithError(a, b interface{}) (int, error) {
	// Try numeric comparison first
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)

	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1, nil
		} else if aNum > bNum {
			return 1, nil
		}
		return 0, nil
	}

	// Fallback to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	if aStr < bStr {
		return -1, nil
	} else if aStr > bStr {
		return 1, nil
	}
	return 0, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// Expression helper functions for rule conditions

// HasTriple checks if an entity has a triple with the given predicate.
// Returns false if entity is nil or predicate not found.
func (e *Evaluator) HasTriple(entity *gtypes.EntityState, predicate string) bool {
	if entity == nil {
		return false
	}

	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			return true
		}
	}

	return false
}

// GetOutgoing returns a list of entity IDs for outgoing relationships with the given predicate.
// Only returns valid entity IDs (4-part dotted notation), not literal values.
// Returns empty slice if entity is nil or no relationships found.
func (e *Evaluator) GetOutgoing(entity *gtypes.EntityState, predicate string) []string {
	// Always return non-nil slice for consistency
	outgoing := []string{}

	if entity == nil {
		return outgoing
	}

	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			// Check if Object is a valid entity ID (relationship)
			if objStr, ok := triple.Object.(string); ok {
				// Use message.IsValidEntityID to check for valid entity reference
				if isValidEntityID(objStr) {
					outgoing = append(outgoing, objStr)
				}
			}
		}
	}

	return outgoing
}

// Distance calculates the great-circle distance between two entities in meters
// using the Haversine formula. Returns error if either entity lacks position data.
// Position is now extracted from geo.location.* triples (single source of truth).
func (e *Evaluator) Distance(entity1, entity2 *gtypes.EntityState) (float64, error) {
	if entity1 == nil || entity2 == nil {
		return 0, fmt.Errorf("both entities must be non-nil")
	}

	// Extract position from triples
	lat1, lon1 := extractLatLonFromTriples(entity1)
	lat2, lon2 := extractLatLonFromTriples(entity2)

	if lat1 == 0 && lon1 == 0 {
		return 0, fmt.Errorf("entity1 must have position data in triples")
	}
	if lat2 == 0 && lon2 == 0 {
		return 0, fmt.Errorf("entity2 must have position data in triples")
	}

	return haversineDistance(lat1, lon1, lat2, lon2), nil
}

// extractLatLonFromTriples extracts latitude and longitude from entity triples
func extractLatLonFromTriples(entity *gtypes.EntityState) (float64, float64) {
	var lat, lon float64
	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case "geo.location.latitude", "latitude":
			if v, ok := triple.Object.(float64); ok {
				lat = v
			}
		case "geo.location.longitude", "longitude":
			if v, ok := triple.Object.(float64); ok {
				lon = v
			}
		}
	}
	return lat, lon
}

// haversineDistance calculates the great-circle distance between two points
// on Earth given their latitude and longitude in degrees.
// Returns distance in meters.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000 // Earth's radius in meters

	// Convert degrees to radians
	lat1Rad := lat1 * 3.14159265359 / 180
	lat2Rad := lat2 * 3.14159265359 / 180
	deltaLatRad := (lat2 - lat1) * 3.14159265359 / 180
	deltaLonRad := (lon2 - lon1) * 3.14159265359 / 180

	// Haversine formula
	a := math.Sin(deltaLatRad/2)*math.Sin(deltaLatRad/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLonRad/2)*math.Sin(deltaLonRad/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}

// isValidEntityID checks if a string is a valid 6-part entity ID
// This is a local copy to avoid circular dependency with message package
func isValidEntityID(s string) bool {
	if s == "" {
		return false
	}

	parts := strings.Split(s, ".")
	if len(parts) != 6 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
	}

	return true
}
