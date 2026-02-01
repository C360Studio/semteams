// Package workflow provides a workflow processor for orchestrating multi-step
// agentic patterns that require loops, limits, and timeouts beyond what the
// rules engine can handle.
//
// # Overview
//
// The workflow processor handles patterns like:
//
//   - reviewer -> fixer -> reviewer (max 3x)
//   - plan -> implement -> test (with timeout)
//   - multi-agent coordination with step tracking
//
// Unlike the rules engine which handles simple condition-action patterns, the
// workflow processor manages complex orchestration with state tracking, variable
// interpolation, and execution lifecycle management.
//
// # Architecture
//
// The workflow processor coordinates between triggers, steps, and agentic tasks:
//
//	                  ┌─────────────────┐
//	workflow.trigger ▶│                 │ ──▶ agent.task.*
//	                  │    workflow     │
//	workflow.step.   ▶│   (this pkg)    │ ──▶ workflow.events
//	complete.>        │                 │
//	                  │                 │
//	agent.complete.> ▶│                 │
//	                  └────────┬────────┘
//	                           │
//	                  ┌────────┴────────┐
//	                  │   NATS KV       │
//	                  │ WORKFLOW_DEFS   │
//	                  │ WORKFLOW_EXEC   │
//	                  └─────────────────┘
//
// # Key Features
//
//   - Workflow definition loading from KV bucket
//   - Step tracking and sequencing
//   - Loop limits (max_iterations)
//   - Workflow timeout enforcement
//   - Variable interpolation (${trigger.*}, ${steps.*}, ${execution.id})
//   - Conditional step execution
//   - Multiple action types
//
// # Quick Start
//
// Create and start the processor:
//
//	config := workflow.DefaultConfig()
//
//	rawConfig, _ := json.Marshal(config)
//	comp, err := workflow.NewComponent(rawConfig, deps)
//
//	lc := comp.(component.LifecycleComponent)
//	lc.Initialize()
//	lc.Start(ctx)
//	defer lc.Stop(5 * time.Second)
//
// Define a workflow:
//
//	definition := workflow.Definition{
//	    ID:      "review-fix-loop",
//	    Name:    "Review and Fix Loop",
//	    Enabled: true,
//	    Trigger: workflow.TriggerDef{
//	        Subject: "code.review.requested",
//	    },
//	    Steps: []workflow.StepDef{
//	        {
//	            Name: "review",
//	            Action: workflow.ActionDef{
//	                Type:    "publish_agent",
//	                Subject: "agent.task.review",
//	                Payload: json.RawMessage(`{"role":"reviewer","prompt":"Review..."}`),
//	            },
//	            OnSuccess: "fix",
//	            OnFail:    "fail",
//	        },
//	        {
//	            Name: "fix",
//	            Action: workflow.ActionDef{
//	                Type:    "publish_agent",
//	                Subject: "agent.task.fix",
//	                Payload: json.RawMessage(`{"role":"editor","prompt":"Fix: ${steps.review.output}"}`),
//	            },
//	            OnSuccess: "review",
//	            OnFail:    "fail",
//	        },
//	    },
//	    MaxIterations: 5,
//	    Timeout:       "30m",
//	}
//
// # Action Types
//
// The processor supports four action types:
//
// **call** - NATS request/response with timeout:
//
//	action := workflow.ActionDef{
//	    Type:    "call",
//	    Subject: "service.request",
//	    Payload: json.RawMessage(`{"key":"value"}`),
//	    Timeout: "30s",
//	}
//
// **publish** - Fire-and-forget NATS publish:
//
//	action := workflow.ActionDef{
//	    Type:    "publish",
//	    Subject: "events.notification",
//	    Payload: json.RawMessage(`{"message":"done"}`),
//	}
//
// **publish_agent** - Spawn an agentic task:
//
//	action := workflow.ActionDef{
//	    Type:    "publish_agent",
//	    Subject: "agent.task.analyze",
//	    Payload: json.RawMessage(`{
//	        "task_id": "${execution.id}",
//	        "role": "general",
//	        "model": "gpt-4",
//	        "prompt": "Analyze: ${trigger.payload.content}"
//	    }`),
//	}
//
// **set_state** - Mutate entity state via graph processor:
//
//	action := workflow.ActionDef{
//	    Type:   "set_state",
//	    Entity: "entity:${trigger.payload.entity_id}",
//	    State:  json.RawMessage(`{"status":"processed"}`),
//	}
//
// # Variable Interpolation
//
// Variables use ${path.to.value} syntax. The Interpolator resolves paths from
// three roots:
//
// **execution.*** - Current execution state:
//
//   - ${execution.id} - Execution ID
//   - ${execution.workflow_id} - Workflow ID
//   - ${execution.workflow_name} - Workflow name
//   - ${execution.state} - Current state (pending, running, etc.)
//   - ${execution.iteration} - Current iteration number
//   - ${execution.current_step} - Current step index
//   - ${execution.current_name} - Current step name
//
// **trigger.*** - Trigger event data:
//
//   - ${trigger.subject} - NATS subject that triggered
//   - ${trigger.payload.*} - Fields from trigger payload
//   - ${trigger.timestamp} - When trigger was received
//   - ${trigger.headers.*} - NATS headers
//
// **steps.*** - Results from completed steps:
//
//   - ${steps.{name}.status} - Status (success, failed, skipped)
//   - ${steps.{name}.output} - Full output object
//   - ${steps.{name}.output.*} - Nested output fields
//   - ${steps.{name}.error} - Error message if failed
//   - ${steps.{name}.duration} - Execution duration
//   - ${steps.{name}.iteration} - Which iteration the step ran
//
// Example usage:
//
//	interpolator := workflow.NewInterpolator(execution)
//
//	// Interpolate a string
//	result, err := interpolator.InterpolateString("Process: ${trigger.payload.name}")
//
//	// Interpolate JSON payload
//	result, err := interpolator.InterpolateJSON(payload)
//
//	// Evaluate a condition
//	matches, err := interpolator.EvaluateCondition(&condition)
//
// # Execution Lifecycle
//
// Executions progress through states:
//
//	pending → running → completed
//	                 ↘ failed
//	                 ↘ timed_out
//
// Execution state is persisted to WORKFLOW_EXECUTIONS bucket:
//
//	exec := workflow.NewExecution(workflowID, workflowName, trigger, timeout)
//	exec.MarkRunning()
//	exec.SetCurrentStep(0, "review")
//	exec.RecordStepResult("review", result)
//	exec.MarkCompleted()
//
// # Condition Evaluation
//
// Steps can have conditions that gate execution:
//
//	condition := workflow.ConditionDef{
//	    Field:    "steps.review.output.issues_count",
//	    Operator: "gt",
//	    Value:    0,
//	}
//
// Supported operators:
//
//   - eq: Equal
//   - ne: Not equal
//   - gt: Greater than
//   - lt: Less than
//   - gte: Greater than or equal
//   - lte: Less than or equal
//   - exists: Field exists
//   - not_exists: Field does not exist
//
// # Configuration
//
// Full configuration schema:
//
//	{
//	    "definitions_bucket": "WORKFLOW_DEFINITIONS",
//	    "executions_bucket": "WORKFLOW_EXECUTIONS",
//	    "stream_name": "WORKFLOW",
//	    "consumer_name_suffix": "",
//	    "default_timeout": "10m",
//	    "default_max_iterations": 10,
//	    "request_timeout": "30s",
//	    "ports": {
//	        "inputs": [...],
//	        "outputs": [...]
//	    }
//	}
//
// # NATS Subjects
//
// The processor uses these subject patterns:
//
//   - workflow.trigger.{id}: Start workflow execution
//   - workflow.step.complete.{exec_id}: Step completed signal
//   - workflow.events: Execution lifecycle events
//   - agent.task.*: Agent task dispatch (for publish_agent)
//
// # KV Buckets
//
// **WORKFLOW_DEFINITIONS**: Stores workflow definitions (JSON)
//
//	{
//	    "id": "review-fix-loop",
//	    "name": "Review and Fix Loop",
//	    "enabled": true,
//	    "steps": [...],
//	    ...
//	}
//
// **WORKFLOW_EXECUTIONS**: Stores execution state (JSON, 7d TTL)
//
//	{
//	    "id": "exec_123456_abc",
//	    "workflow_id": "review-fix-loop",
//	    "state": "running",
//	    "current_step": 1,
//	    "iteration": 2,
//	    "step_results": {...},
//	    "deadline": "2024-01-15T11:00:00Z"
//	}
//
// # Step Transitions
//
// Steps define transitions via on_success and on_fail:
//
//   - Step name: Transition to named step
//   - "complete": Mark workflow completed
//   - "fail": Mark workflow failed
//
// Loop detection: When on_success points to an earlier step, the execution
// increments iteration and checks against max_iterations.
//
// # Error Handling
//
// Errors are handled at multiple levels:
//
//   - Validation errors: Returned from Validate() methods
//   - Execution errors: Mark step failed, follow on_fail transition
//   - Timeout errors: Mark execution timed_out
//   - KV errors: Logged, execution may be inconsistent
//
// # Thread Safety
//
// The Execution type uses RWMutex for concurrent access:
//
//	exec.MarkRunning()           // Write lock
//	state := exec.GetState()     // Read lock
//	clone := exec.Clone()        // Read lock, returns copy
//
// # Testing
//
// Use ConsumerNameSuffix for test isolation:
//
//	config := workflow.Config{
//	    StreamName:         "WORKFLOW",
//	    ConsumerNameSuffix: "test-" + t.Name(),
//	}
//
// # Limitations
//
// Current limitations:
//
//   - No parallel step execution (sequential only)
//   - No sub-workflow invocation
//   - No manual intervention/approval steps
//   - Step output limited by KV value size (1MB)
//
// # See Also
//
// Related packages:
//
//   - processor/agentic-loop: Loop orchestration for agents
//   - processor/agentic-dispatch: User message routing
//   - processor/rules: Simple condition-action patterns
//   - agentic: Shared types
package workflow
