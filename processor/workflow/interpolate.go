package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// interpolationPattern matches ${path.to.value} patterns
var interpolationPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Interpolator handles variable interpolation in workflow data
type Interpolator struct {
	execution *Execution
}

// NewInterpolator creates a new interpolator for an execution
func NewInterpolator(exec *Execution) *Interpolator {
	return &Interpolator{execution: exec}
}

// InterpolateString interpolates variables in a string
func (i *Interpolator) InterpolateString(input string) (string, error) {
	return i.interpolate(input)
}

// InterpolateJSON interpolates variables in a JSON structure
func (i *Interpolator) InterpolateJSON(input json.RawMessage) (json.RawMessage, error) {
	if input == nil {
		return nil, nil
	}

	str := string(input)
	result, err := i.interpolate(str)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(result), nil
}

// interpolate replaces ${...} patterns with their values
func (i *Interpolator) interpolate(input string) (string, error) {
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
func (i *Interpolator) resolvePath(path string) (any, error) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	switch parts[0] {
	case "execution":
		return i.resolveExecutionPath(parts[1:])
	case "trigger":
		return i.resolveTriggerPath(parts[1:])
	case "steps":
		return i.resolveStepsPath(parts[1:])
	default:
		return nil, fmt.Errorf("unknown path root: %s", parts[0])
	}
}

// resolveExecutionPath resolves paths under execution.*
func (i *Interpolator) resolveExecutionPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("execution path requires field")
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
		return nil, fmt.Errorf("unknown execution field: %s", parts[0])
	}
}

// resolveTriggerPath resolves paths under trigger.*
func (i *Interpolator) resolveTriggerPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("trigger path requires field")
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
		return nil, fmt.Errorf("unknown trigger field: %s", parts[0])
	}
}

// resolveTriggerPayloadPath resolves paths into the trigger payload JSON
func (i *Interpolator) resolveTriggerPayloadPath(parts []string) (any, error) {
	if i.execution.Trigger.Payload == nil {
		return nil, fmt.Errorf("trigger payload is empty")
	}

	var payload map[string]any
	if err := json.Unmarshal(i.execution.Trigger.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse trigger payload: %w", err)
	}

	return resolveMapPath(payload, parts)
}

// resolveStepsPath resolves paths under steps.*
func (i *Interpolator) resolveStepsPath(parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("steps path requires step name")
	}

	stepName := parts[0]
	result, ok := i.execution.StepResults[stepName]
	if !ok {
		return nil, fmt.Errorf("step not found: %s", stepName)
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
		return nil, fmt.Errorf("unknown step field: %s", parts[1])
	}
}

// resolveStepOutputPath resolves paths into a step's output JSON
func (i *Interpolator) resolveStepOutputPath(result StepResult, parts []string) (any, error) {
	if result.Output == nil {
		return nil, fmt.Errorf("step output is empty")
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		// Try as a simple value
		var simpleOutput any
		if err2 := json.Unmarshal(result.Output, &simpleOutput); err2 == nil && len(parts) == 0 {
			return simpleOutput, nil
		}
		return nil, fmt.Errorf("failed to parse step output: %w", err)
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
		return nil, fmt.Errorf("key not found: %s", key)
	}

	if len(parts) == 1 {
		return value, nil
	}

	// Handle array indexing
	if idx, err := strconv.Atoi(parts[1]); err == nil {
		arr, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("expected array for index: %s", parts[1])
		}
		if idx < 0 || idx >= len(arr) {
			return nil, fmt.Errorf("array index out of bounds: %d", idx)
		}
		if len(parts) == 2 {
			return arr[idx], nil
		}
		if nestedMap, ok := arr[idx].(map[string]any); ok {
			return resolveMapPath(nestedMap, parts[2:])
		}
		return nil, fmt.Errorf("cannot traverse into non-object array element")
	}

	// Handle nested object
	nestedMap, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cannot traverse into non-object: %s", key)
	}

	return resolveMapPath(nestedMap, parts[1:])
}

// valueToString converts a value to its string representation
func (i *Interpolator) valueToString(value any) string {
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

// EvaluateCondition evaluates a condition against the current execution state
func (i *Interpolator) EvaluateCondition(cond *ConditionDef) (bool, error) {
	if cond == nil {
		return true, nil
	}

	value, err := i.resolvePath(cond.Field)
	if err != nil {
		// exists/not_exists operators handle missing values
		if cond.Operator == "not_exists" {
			return true, nil
		}
		if cond.Operator == "exists" {
			return false, nil
		}
		return false, fmt.Errorf("failed to resolve field %s: %w", cond.Field, err)
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
	default:
		return false, fmt.Errorf("unknown operator: %s", cond.Operator)
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
