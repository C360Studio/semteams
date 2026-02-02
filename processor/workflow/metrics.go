package workflow

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// workflowMetrics holds Prometheus metrics for the workflow processor
type workflowMetrics struct {
	// Workflow lifecycle
	workflowsStarted   *prometheus.CounterVec
	workflowsCompleted *prometheus.CounterVec
	workflowsFailed    *prometheus.CounterVec
	workflowsTimeout   *prometheus.CounterVec
	activeWorkflows    prometheus.Gauge

	// Iterations
	iterationsTotal       *prometheus.CounterVec
	loopMaxIterations     *prometheus.CounterVec
	iterationsPerWorkflow *prometheus.HistogramVec

	// Duration
	workflowDuration *prometheus.HistogramVec

	// Steps
	stepsStarted   *prometheus.CounterVec
	stepsCompleted *prometheus.CounterVec
	stepsFailed    *prometheus.CounterVec
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce     sync.Once
	metricsInstance *workflowMetrics
)

// getMetrics returns the singleton metrics instance, creating and registering it if needed
func getMetrics(registry *metric.MetricsRegistry) *workflowMetrics {
	metricsOnce.Do(func() {
		metricsInstance = &workflowMetrics{
			workflowsStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "workflows_started_total",
				Help:      "Total number of workflows started",
			}, []string{"workflow_id"}),

			workflowsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "workflows_completed_total",
				Help:      "Total number of workflows completed successfully",
			}, []string{"workflow_id"}),

			workflowsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "workflows_failed_total",
				Help:      "Total number of workflows that failed",
			}, []string{"workflow_id", "reason"}),

			workflowsTimeout: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "workflows_timeout_total",
				Help:      "Total number of workflows that timed out",
			}, []string{"workflow_id"}),

			activeWorkflows: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "active_workflows",
				Help:      "Number of currently active workflows",
			}),

			iterationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "iterations_total",
				Help:      "Total number of loop iterations",
			}, []string{"workflow_id"}),

			loopMaxIterations: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "loop_max_iterations_total",
				Help:      "Total number of times a workflow reached max iterations",
			}, []string{"workflow_id"}),

			iterationsPerWorkflow: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "iterations_per_workflow",
				Help:      "Distribution of iterations per workflow execution",
				Buckets:   []float64{1, 2, 3, 5, 10, 15, 20, 50, 100},
			}, []string{"workflow_id"}),

			workflowDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "duration_seconds",
				Help:      "Duration of workflow executions in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15), // 0.1s to ~3276s (~55min)
			}, []string{"workflow_id", "status"}),

			stepsStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "steps_started_total",
				Help:      "Total number of steps started",
			}, []string{"step_name"}),

			stepsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "steps_completed_total",
				Help:      "Total number of steps completed successfully",
			}, []string{"step_name"}),

			stepsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "workflow",
				Name:      "steps_failed_total",
				Help:      "Total number of steps that failed",
			}, []string{"step_name"}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounterVec("workflow", "workflows_started_total", metricsInstance.workflowsStarted)
			_ = registry.RegisterCounterVec("workflow", "workflows_completed_total", metricsInstance.workflowsCompleted)
			_ = registry.RegisterCounterVec("workflow", "workflows_failed_total", metricsInstance.workflowsFailed)
			_ = registry.RegisterCounterVec("workflow", "workflows_timeout_total", metricsInstance.workflowsTimeout)
			_ = registry.RegisterGauge("workflow", "active_workflows", metricsInstance.activeWorkflows)
			_ = registry.RegisterCounterVec("workflow", "iterations_total", metricsInstance.iterationsTotal)
			_ = registry.RegisterCounterVec("workflow", "loop_max_iterations_total", metricsInstance.loopMaxIterations)
			_ = registry.RegisterHistogramVec("workflow", "iterations_per_workflow", metricsInstance.iterationsPerWorkflow)
			_ = registry.RegisterHistogramVec("workflow", "duration_seconds", metricsInstance.workflowDuration)
			_ = registry.RegisterCounterVec("workflow", "steps_started_total", metricsInstance.stepsStarted)
			_ = registry.RegisterCounterVec("workflow", "steps_completed_total", metricsInstance.stepsCompleted)
			_ = registry.RegisterCounterVec("workflow", "steps_failed_total", metricsInstance.stepsFailed)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.workflowsStarted)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.workflowsCompleted)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.workflowsFailed)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.workflowsTimeout)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.activeWorkflows)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.iterationsTotal)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.loopMaxIterations)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.iterationsPerWorkflow)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.workflowDuration)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.stepsStarted)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.stepsCompleted)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.stepsFailed)
		}
	})
	return metricsInstance
}

// recordWorkflowStarted increments the workflows started counter
func (m *workflowMetrics) recordWorkflowStarted(workflowID string) {
	m.workflowsStarted.WithLabelValues(workflowID).Inc()
	m.activeWorkflows.Inc()
}

// recordWorkflowCompleted records a successful workflow completion
func (m *workflowMetrics) recordWorkflowCompleted(workflowID string, iterations int, durationSeconds float64) {
	m.workflowsCompleted.WithLabelValues(workflowID).Inc()
	m.activeWorkflows.Dec()
	m.iterationsPerWorkflow.WithLabelValues(workflowID).Observe(float64(iterations))
	m.workflowDuration.WithLabelValues(workflowID, "completed").Observe(durationSeconds)
}

// recordWorkflowFailed records a failed workflow
func (m *workflowMetrics) recordWorkflowFailed(workflowID, reason string, iterations int, durationSeconds float64) {
	m.workflowsFailed.WithLabelValues(workflowID, reason).Inc()
	m.activeWorkflows.Dec()
	m.iterationsPerWorkflow.WithLabelValues(workflowID).Observe(float64(iterations))
	m.workflowDuration.WithLabelValues(workflowID, "failed").Observe(durationSeconds)
}

// recordWorkflowTimeout records a workflow timeout
func (m *workflowMetrics) recordWorkflowTimeout(workflowID string, iterations int, durationSeconds float64) {
	m.workflowsTimeout.WithLabelValues(workflowID).Inc()
	m.activeWorkflows.Dec()
	m.iterationsPerWorkflow.WithLabelValues(workflowID).Observe(float64(iterations))
	m.workflowDuration.WithLabelValues(workflowID, "timeout").Observe(durationSeconds)
}

// recordIteration increments the iterations counter
func (m *workflowMetrics) recordIteration(workflowID string) {
	m.iterationsTotal.WithLabelValues(workflowID).Inc()
}

// recordLoopMaxIterations records when a workflow hits max iterations
func (m *workflowMetrics) recordLoopMaxIterations(workflowID string) {
	m.loopMaxIterations.WithLabelValues(workflowID).Inc()
}

// recordStepStarted records a step starting
func (m *workflowMetrics) recordStepStarted(stepName string) {
	m.stepsStarted.WithLabelValues(stepName).Inc()
}

// recordStepCompleted records a step completing successfully
func (m *workflowMetrics) recordStepCompleted(stepName string) {
	m.stepsCompleted.WithLabelValues(stepName).Inc()
}

// recordStepFailed records a step failing
func (m *workflowMetrics) recordStepFailed(stepName string) {
	m.stepsFailed.WithLabelValues(stepName).Inc()
}
