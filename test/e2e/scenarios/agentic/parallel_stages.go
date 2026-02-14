package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/workflow"
	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// registerParallelWorkflow registers the parallel agents test workflow to NATS KV.
func (s *Scenario) registerParallelWorkflow(ctx context.Context, result *scenarios.Result) error {
	data, err := json.Marshal(TestParallelWorkflow)
	if err != nil {
		return fmt.Errorf("failed to marshal parallel workflow: %w", err)
	}

	if err := s.nats.PutKV(ctx, client.BucketWorkflowDefinitions, TestParallelWorkflowID, data); err != nil {
		return fmt.Errorf("failed to register parallel workflow: %w", err)
	}

	result.Details["parallel_workflow_id"] = TestParallelWorkflowID
	result.Details["parallel_workflow_registered"] = true
	result.Details["parallel_step_count"] = len(TestParallelWorkflow.Steps[0].Steps)

	// Give workflow processor time to pick up the new definition
	time.Sleep(500 * time.Millisecond)

	return nil
}

// testParallelAgents triggers the parallel workflow and validates the results.
// It verifies:
// - 3 agent loops are spawned and complete
// - Results are aggregated using the union aggregator
// - Workflow completes successfully
func (s *Scenario) testParallelAgents(ctx context.Context, result *scenarios.Result) error {
	// Capture baseline loop count before triggering
	baselineLoops := 0.0
	currentLoops, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
	if err == nil {
		baselineLoops = currentLoops
	}

	// Trigger the parallel workflow
	requestID := fmt.Sprintf("e2e-parallel-%d", time.Now().UnixNano())

	triggerPayload := &workflow.TriggerPayload{
		WorkflowID: TestParallelWorkflowID,
		RequestID:  requestID,
		Data:       json.RawMessage(`{"content": "E2E test for parallel agent execution"}`),
	}

	baseMsg := message.NewBaseMessage(triggerPayload.Schema(), triggerPayload, "e2e-test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal parallel trigger: %w", err)
	}

	result.Details["parallel_trigger_request_id"] = requestID

	if err := s.nats.Publish(ctx, "workflow.trigger.e2e-parallel", data); err != nil {
		return fmt.Errorf("failed to publish parallel workflow trigger: %w", err)
	}

	// Wait for completion: 3 agent loops + workflow completion
	timeout := 60 * time.Second
	deadline := time.Now().Add(timeout)

	var loopsCompleted float64
	var workflowCompleted bool
	var lastExec *client.WorkflowExecution
	expectedLoops := baselineLoops + 3 // 3 parallel reviewers

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Check agent loop completion via metrics
			loops, err := s.metrics.SumMetricsByName(ctx, "semstreams_agentic_loop_loops_completed_total")
			if err == nil {
				loopsCompleted = loops
			}

			// Check workflow completion via KV state
			exec, err := s.nats.WaitForWorkflowState(ctx, TestParallelWorkflowID, []string{"completed", "failed"}, 100*time.Millisecond)
			if err == nil && exec != nil {
				lastExec = exec
				if exec.State == "completed" {
					workflowCompleted = true
				} else if exec.State == "failed" {
					result.Details["parallel_workflow_execution"] = exec
					return fmt.Errorf("parallel workflow failed: %s", exec.Error)
				}
			}

			// Success: all 3 loops completed AND workflow completed
			if loopsCompleted >= expectedLoops && workflowCompleted {
				break
			}
		}

		if loopsCompleted >= expectedLoops && workflowCompleted {
			break
		}
	}

	// Validate results
	result.Metrics["parallel_loops_completed"] = loopsCompleted - baselineLoops
	result.Details["parallel_workflow_completed"] = workflowCompleted

	if !workflowCompleted {
		return fmt.Errorf("parallel workflow did not complete (loops=%v, expected=%v)",
			loopsCompleted-baselineLoops, 3)
	}

	if lastExec == nil {
		return fmt.Errorf("could not retrieve parallel workflow execution")
	}

	result.Details["parallel_workflow_execution"] = lastExec

	// Validate the parallel step result contains aggregated output
	parallelResult, hasResult := lastExec.StepResults["parallel_review"]
	if !hasResult {
		return fmt.Errorf("parallel_review step result not found")
	}

	if parallelResult.Status != "success" {
		return fmt.Errorf("parallel_review step failed: %s", parallelResult.Error)
	}

	// Verify aggregated output is an array (union aggregator)
	var aggregatedOutput []json.RawMessage
	if err := json.Unmarshal(parallelResult.Output, &aggregatedOutput); err != nil {
		return fmt.Errorf("expected aggregated output to be an array (union aggregator): %w", err)
	}

	// Should have 3 results from 3 reviewers
	if len(aggregatedOutput) != 3 {
		return fmt.Errorf("expected 3 aggregated results, got %d", len(aggregatedOutput))
	}

	result.Details["parallel_aggregated_results"] = len(aggregatedOutput)
	result.Metrics["parallel_reviewers_completed"] = float64(len(aggregatedOutput))

	return nil
}
