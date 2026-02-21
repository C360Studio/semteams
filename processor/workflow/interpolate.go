package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/c360studio/semstreams/pkg/errs"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
)

// interpolationPattern matches ${path.to.value} patterns
var interpolationPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// pureInterpolationPattern matches strings that are exactly one ${...} pattern (no surrounding text)
var pureInterpolationPattern = regexp.MustCompile(`^\$\{([^}]+)\}$`)

// interpolator handles variable interpolation in workflow data
type interpolator struct {
	execution *Execution
}

// newInterpolator creates a new interpolator for an execution
func newInterpolator(exec *Execution) *interpolator {
	return &interpolator{execution: exec}
}

// InterpolateString interpolates variables in a string
func (i *interpolator) InterpolateString(input string) (string, error) {
	return i.interpolate(input)
}

// InterpolateJSON interpolates variables in a JSON structure with type preservation.
// For "pure" interpolations where a string value is exactly "${path}", the resolved
// value's type is preserved (arrays stay arrays, objects stay objects).
// For embedded interpolations like "prefix ${path} suffix", string interpolation is used.
func (i *interpolator) InterpolateJSON(input json.RawMessage) (json.RawMessage, error) {
	if input == nil {
		return nil, nil
	}

	// Parse JSON into a generic structure
	var raw any
	if err := json.Unmarshal(input, &raw); err != nil {
		// If not valid JSON, fall back to string-based interpolation
		str := string(input)
		result, interpolateErr := i.interpolate(str)
		if interpolateErr != nil {
			return input, interpolateErr
		}
		return json.RawMessage(result), nil
	}

	// Walk and interpolate with type preservation
	result, err := i.walkAndInterpolate(raw)
	if err != nil {
		return nil, err
	}

	// Marshal back to JSON
	return json.Marshal(result)
}

// interpolateStringWithFallback interpolates a string, returning the original on error
func (i *interpolator) interpolateStringWithFallback(input string) string {
	if input == "" {
		return input
	}
	result, err := i.interpolate(input)
	if err != nil {
		return input
	}
	return result
}

// interpolateJSONWithFallback interpolates JSON, returning the original on error
func (i *interpolator) interpolateJSONWithFallback(input json.RawMessage) json.RawMessage {
	if input == nil {
		return nil
	}
	result, err := i.InterpolateJSON(input)
	if err != nil {
		return input
	}
	return result
}

// InterpolateActionDef returns a copy of the ActionDef with all fields interpolated.
// On interpolation errors, the original field value is preserved.
func (i *interpolator) InterpolateActionDef(action wfschema.ActionDef) wfschema.ActionDef {
	return wfschema.ActionDef{
		Type:    action.Type,
		Subject: i.interpolateStringWithFallback(action.Subject),
		Payload: i.interpolateJSONWithFallback(action.Payload),
		Entity:  i.interpolateStringWithFallback(action.Entity),
		State:   i.interpolateJSONWithFallback(action.State),
		Timeout: action.Timeout, // Timeout is not interpolated (it's a duration)
		Role:    i.interpolateStringWithFallback(action.Role),
		Model:   i.interpolateStringWithFallback(action.Model),
		Prompt:  i.interpolateStringWithFallback(action.Prompt),
		TaskID:  i.interpolateStringWithFallback(action.TaskID),
	}
}

// interpolate replaces ${...} patterns with their values
func (i *interpolator) interpolate(input string) (string, error) {
	var lastErr error

	result := interpolationPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the path from ${path}
		path := match[2 : len(match)-1]

		value, err := i.resolvePath(path)
		if err != nil {
			lastErr = err
			return match // Keep original on error
		}

		// Convert value to string
		return i.valueToString(value)
	})

	return result, lastErr
}

// resolvePath resolves a dot-notation path to a value
func (i *interpolator) resolvePath(path string) (any, error) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, errs.WrapInvalid(fmt.Errorf("empty path"), "interpolator", "resolvePath", "parse path")
	}

	switch parts[0] {
	case "execution":
		return i.resolveExecutionPath(parts[1:])
	case "trigger":
		return i.resolveTriggerPath(parts[1:])
	case "steps":
		return i.resolveStepsPath(parts[1:])
	default:
		return nil, errs.WrapInvalid(fmt.Errorf("unknown path root: %s", parts[0]), "interpolator", "resolvePath", "resolve path root")
	}
}

// resolveExecutionPath resolves paths under execution.*
func (i *interpolator) resolveExecutionPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, errs.WrapInvalid(fmt.Errorf("execution path requires field"), "interpolator", "resolveExecutionPath", "validate path")
	}

	switch parts[0] {
	case "id":
		return i.execution.ID, nil
	case "workflow_id":
		return i.execution.WorkflowID, nil
	case "workflow_name":
		return i.execution.WorkflowName, nil
	case "state":
		return string(i.execution.State), nil
	case "iteration":
		return i.execution.Iteration, nil
	case "current_step":
		return i.execution.CurrentStep, nil
	case "current_name":
		return i.execution.CurrentName, nil
	default:
		return nil, errs.WrapInvalid(fmt.Errorf("unknown execution field: %s", parts[0]), "interpolator", "resolveExecutionPath", "resolve field")
	}
}

// resolveTriggerPath resolves paths under trigger.*
func (i *interpolator) resolveTriggerPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, errs.WrapInvalid(fmt.Errorf("trigger path requires field"), "interpolator", "resolveTriggerPath", "validate path")
	}

	switch parts[0] {
	case "subject":
		return i.execution.Trigger.Subject, nil
	case "payload":
		return i.resolveTriggerPayloadPath(parts[1:])
	case "timestamp":
		return i.execution.Trigger.Timestamp.Format("2006-01-02T15:04:05Z"), nil
	case "headers":
		if len(parts) > 1 {
			return i.execution.Trigger.Headers[parts[1]], nil
		}
		return i.execution.Trigger.Headers, nil
	default:
		return nil, errs.WrapInvalid(fmt.Errorf("unknown trigger field: %s", parts[0]), "interpolator", "resolveTriggerPath", "resolve field")
	}
}

// resolveTriggerPayloadPath resolves paths into the trigger payload JSON
func (i *interpolator) resolveTriggerPayloadPath(parts []string) (any, error) {
	if i.execution.Trigger.Payload == nil {
		return nil, errs.WrapInvalid(fmt.Errorf("trigger payload is empty"), "interpolator", "resolveTriggerPayloadPath", "access payload")
	}

	var payload map[string]any
	if err := json.Unmarshal(i.execution.Trigger.Payload, &payload); err != nil {
		return nil, errs.WrapInvalid(err, "interpolator", "resolveTriggerPayloadPath", "parse trigger payload")
	}

	return resolveMapPath(payload, parts)
}

// resolveStepsPath resolves paths under steps.*
func (i *interpolator) resolveStepsPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, errs.WrapInvalid(fmt.Errorf("steps path requires step name"), "interpolator", "resolveStepsPath", "validate path")
	}

	stepName := parts[0]
	result, ok := i.execution.StepResults[stepName]
	if !ok {
		return nil, errs.WrapInvalid(fmt.Errorf("step not found: %s", stepName), "interpolator", "resolveStepsPath", "find step")
	}

	if len(parts) == 1 {
		return result, nil
	}

	switch parts[1] {
	case "status":
		return result.Status, nil
	case "error":
		return result.Error, nil
	case "duration":
		return result.Duration.String(), nil
	case "iteration":
		return result.Iteration, nil
	case "output":
		return i.resolveStepOutputPath(result, parts[2:])
	default:
		return nil, errs.WrapInvalid(fmt.Errorf("unknown step field: %s", parts[1]), "interpolator", "resolveStepsPath", "resolve field")
	}
}

// resolveStepOutputPath resolves paths into a step's output JSON
func (i *interpolator) resolveStepOutputPath(result StepResult, parts []string) (any, error) {
	if result.Output == nil {
		return nil, errs.WrapInvalid(fmt.Errorf("step output is empty"), "interpolator", "resolveStepOutputPath", "access output")
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		// Try as a simple value
		var simpleOutput any
		if err2 := json.Unmarshal(result.Output, &simpleOutput); err2 == nil && len(parts) == 0 {
			return simpleOutput, nil
		}
		return nil, errs.WrapInvalid(err, "interpolator", "resolveStepOutputPath", "parse step output")
	}

	if len(parts) == 0 {
		return output, nil
	}

	return resolveMapPath(output, parts)
}

// resolveMapPath resolves a dot-notation path within a map
func resolveMapPath(data map[string]any, parts []string) (any, error) {
	if len(parts) == 0 {
		return data, nil
	}

	key := parts[0]
	value, ok := data[key]
	if !ok {
		return nil, errs.WrapInvalid(fmt.Errorf("key not found: %s", key), "interpolator", "resolveMapPath", "find key")
	}

	if len(parts) == 1 {
		return value, nil
	}

	// Handle array indexing
	if idx, err := strconv.Atoi(parts[1]); err == nil {
		arr, ok := value.([]any)
		if !ok {
			return nil, errs.WrapInvalid(fmt.Errorf("expected array for index: %s", parts[1]), "interpolator", "resolveMapPath", "access array")
		}
		if idx < 0 || idx >= len(arr) {
			return nil, errs.WrapInvalid(fmt.Errorf("array index out of bounds: %d", idx), "interpolator", "resolveMapPath", "validate index")
		}
		if len(parts) == 2 {
			return arr[idx], nil
		}
		if nestedMap, ok := arr[idx].(map[string]any); ok {
			return resolveMapPath(nestedMap, parts[2:])
		}
		return nil, errs.WrapInvalid(fmt.Errorf("cannot traverse into non-object array element"), "interpolator", "resolveMapPath", "traverse array")
	}

	// Handle nested object
	nestedMap, ok := value.(map[string]any)
	if !ok {
		return nil, errs.WrapInvalid(fmt.Errorf("cannot traverse into non-object: %s", key), "interpolator", "resolveMapPath", "traverse object")
	}

	return resolveMapPath(nestedMap, parts[1:])
}

// valueToString converts a value to its string representation
func (i *interpolator) valueToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		// For complex types, marshal to JSON
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

// walkAndInterpolate recursively walks a JSON value and performs type-aware interpolation.
// For strings, it detects pure vs embedded interpolations and handles them appropriately.
func (i *interpolator) walkAndInterpolate(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		return i.walkMap(v)
	case []any:
		return i.walkArray(v)
	case string:
		return i.interpolateStringValue(v)
	default:
		// Numbers, bools, null - pass through unchanged
		return v, nil
	}
}

// walkMap processes each entry in a map, interpolating values recursively.
func (i *interpolator) walkMap(m map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for k, v := range m {
		interpolated, err := i.walkAndInterpolate(v)
		if err != nil {
			return nil, err
		}
		result[k] = interpolated
	}
	return result, nil
}

// walkArray processes each element in an array, interpolating values recursively.
func (i *interpolator) walkArray(arr []any) ([]any, error) {
	result := make([]any, len(arr))
	for idx, v := range arr {
		interpolated, err := i.walkAndInterpolate(v)
		if err != nil {
			return nil, err
		}
		result[idx] = interpolated
	}
	return result, nil
}

// interpolateStringValue handles string interpolation with type preservation.
// For "pure" interpolations (entire string is one ${...}), the resolved value's
// native type is preserved. For embedded interpolations, string replacement is used.
func (i *interpolator) interpolateStringValue(s string) (any, error) {
	// Check for pure interpolation (entire string is exactly one ${...})
	if match := pureInterpolationPattern.FindStringSubmatch(s); match != nil {
		path := match[1]
		value, err := i.resolvePath(path)
		if err != nil {
			// Return nil and error - let caller decide how to handle
			return nil, err
		}
		// Return the resolved value with its native type
		return value, nil
	}

	// Check if string contains any interpolation patterns
	if !interpolationPattern.MatchString(s) {
		// No interpolation needed, return as-is
		return s, nil
	}

	// Embedded interpolation - use existing string replacement
	result, err := i.interpolate(s)
	if err != nil {
		// Return empty string and error - let caller decide how to handle
		return "", err
	}
	return result, nil
}

// EvaluateCondition evaluates a condition against the current execution state
func (i *interpolator) EvaluateCondition(cond *wfschema.ConditionDef) (bool, error) {
	if cond == nil {
		return true, nil
	}

	// Strip ${...} wrapper if present for consistent UX
	field := cond.Field
	if strings.HasPrefix(field, "${") && strings.HasSuffix(field, "}") {
		field = field[2 : len(field)-1]
	}

	value, err := i.resolvePath(field)
	if err != nil {
		// exists/not_exists operators handle missing values
		if cond.Operator == "not_exists" {
			return true, nil
		}
		if cond.Operator == "exists" {
			return false, nil
		}
		return false, errs.WrapInvalid(err, "interpolator", "EvaluateCondition", fmt.Sprintf("resolve field %s", cond.Field))
	}

	switch cond.Operator {
	case "exists":
		return value != nil, nil
	case "not_exists":
		return value == nil, nil
	case "eq":
		return compareEqual(value, cond.Value), nil
	case "ne":
		return !compareEqual(value, cond.Value), nil
	case "gt":
		return compareGreater(value, cond.Value), nil
	case "lt":
		return compareLess(value, cond.Value), nil
	case "gte":
		return compareEqual(value, cond.Value) || compareGreater(value, cond.Value), nil
	case "lte":
		return compareEqual(value, cond.Value) || compareLess(value, cond.Value), nil
	case "in":
		arr, ok := cond.Value.([]any)
		if !ok {
			return false, errs.WrapInvalid(fmt.Errorf("in operator requires array value"), "interpolator", "EvaluateCondition", "validate in operator")
		}
		for _, item := range arr {
			if compareEqual(value, item) {
				return true, nil
			}
		}
		return false, nil
	case "not_in":
		arr, ok := cond.Value.([]any)
		if !ok {
			return false, errs.WrapInvalid(fmt.Errorf("not_in operator requires array value"), "interpolator", "EvaluateCondition", "validate not_in operator")
		}
		for _, item := range arr {
			if compareEqual(value, item) {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, errs.WrapInvalid(fmt.Errorf("unknown operator: %s", cond.Operator), "interpolator", "EvaluateCondition", "validate operator")
	}
}

// compareEqual compares two values for equality
func compareEqual(a, b any) bool {
	// Handle numeric comparisons
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum == bNum
	}

	// String comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// compareGreater compares if a > b
func compareGreater(a, b any) bool {
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum > bNum
	}

	// String comparison
	return fmt.Sprintf("%v", a) > fmt.Sprintf("%v", b)
}

// compareLess compares if a < b
func compareLess(a, b any) bool {
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum < bNum
	}

	// String comparison
	return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
}

// toFloat64 converts a value to float64 if possible
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
