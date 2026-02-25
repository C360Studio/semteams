package reactive

import (
	"sync"

	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// EngineMetrics holds Prometheus metrics for the reactive workflow engine.
// It implements MetricsRecorder interface.
type EngineMetrics struct {
	// Execution lifecycle
	executionsCreated   *prometheus.CounterVec
	executionsCompleted *prometheus.CounterVec
	executionsFailed    *prometheus.CounterVec
	executionsTimedOut  *prometheus.CounterVec
	executionsEscalated *prometheus.CounterVec
	activeExecutions    *prometheus.GaugeVec

	// Rule evaluation
	ruleEvaluationsTotal *prometheus.CounterVec
	ruleFiringsTotal     *prometheus.CounterVec

	// Actions
	actionsDispatchedTotal *prometheus.CounterVec

	// Duration
	executionDuration *prometheus.HistogramVec
	ruleDuration      *prometheus.HistogramVec

	// Callbacks
	callbacksReceivedTotal *prometheus.CounterVec
	pendingCallbacks       prometheus.Gauge
	callbackLatency        *prometheus.HistogramVec
}

// Package-level metrics (registered once to avoid duplicate registration errors)
var (
	metricsOnce     sync.Once
	metricsInstance *EngineMetrics
)

// GetMetrics returns the singleton metrics instance, creating and registering it if needed.
func GetMetrics(registry *metric.MetricsRegistry) *EngineMetrics {
	metricsOnce.Do(func() {
		metricsInstance = &EngineMetrics{
			executionsCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "executions_created_total",
				Help:      "Total number of workflow executions created",
			}, []string{"workflow_id"}),

			executionsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "executions_completed_total",
				Help:      "Total number of workflow executions completed successfully",
			}, []string{"workflow_id"}),

			executionsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "executions_failed_total",
				Help:      "Total number of workflow executions that failed",
			}, []string{"workflow_id", "reason"}),

			executionsTimedOut: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "executions_timed_out_total",
				Help:      "Total number of workflow executions that timed out",
			}, []string{"workflow_id"}),

			executionsEscalated: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "executions_escalated_total",
				Help:      "Total number of workflow executions that were escalated",
			}, []string{"workflow_id", "reason"}),

			activeExecutions: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "active_executions",
				Help:      "Number of currently active workflow executions",
			}, []string{"workflow_id"}),

			ruleEvaluationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "rule_evaluations_total",
				Help:      "Total number of rule evaluations",
			}, []string{"workflow_id", "rule_id"}),

			ruleFiringsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "rule_firings_total",
				Help:      "Total number of rules that fired (conditions met)",
			}, []string{"workflow_id", "rule_id"}),

			actionsDispatchedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "actions_dispatched_total",
				Help:      "Total number of actions dispatched",
			}, []string{"workflow_id", "rule_id", "action_type"}),

			executionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "execution_duration_seconds",
				Help:      "Duration of workflow executions in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15), // 0.1s to ~3276s (~55min)
			}, []string{"workflow_id", "status"}),

			ruleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "rule_duration_seconds",
				Help:      "Duration of rule evaluation and execution in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
			}, []string{"workflow_id", "rule_id"}),

			callbacksReceivedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "callbacks_received_total",
				Help:      "Total number of async callbacks received",
			}, []string{"workflow_id", "rule_id", "status"}),

			pendingCallbacks: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "pending_callbacks",
				Help:      "Number of callbacks currently pending",
			}),

			callbackLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "semstreams",
				Subsystem: "reactive_workflow",
				Name:      "callback_latency_seconds",
				Help:      "Time between async action dispatch and callback receipt",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 12), // 100ms to ~410s
			}, []string{"workflow_id", "rule_id"}),
		}

		// Register metrics with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounterVec("reactive_workflow", "executions_created_total", metricsInstance.executionsCreated)
			_ = registry.RegisterCounterVec("reactive_workflow", "executions_completed_total", metricsInstance.executionsCompleted)
			_ = registry.RegisterCounterVec("reactive_workflow", "executions_failed_total", metricsInstance.executionsFailed)
			_ = registry.RegisterCounterVec("reactive_workflow", "executions_timed_out_total", metricsInstance.executionsTimedOut)
			_ = registry.RegisterCounterVec("reactive_workflow", "executions_escalated_total", metricsInstance.executionsEscalated)
			_ = registry.RegisterGaugeVec("reactive_workflow", "active_executions", metricsInstance.activeExecutions)
			_ = registry.RegisterCounterVec("reactive_workflow", "rule_evaluations_total", metricsInstance.ruleEvaluationsTotal)
			_ = registry.RegisterCounterVec("reactive_workflow", "rule_firings_total", metricsInstance.ruleFiringsTotal)
			_ = registry.RegisterCounterVec("reactive_workflow", "actions_dispatched_total", metricsInstance.actionsDispatchedTotal)
			_ = registry.RegisterHistogramVec("reactive_workflow", "execution_duration_seconds", metricsInstance.executionDuration)
			_ = registry.RegisterHistogramVec("reactive_workflow", "rule_duration_seconds", metricsInstance.ruleDuration)
			_ = registry.RegisterCounterVec("reactive_workflow", "callbacks_received_total", metricsInstance.callbacksReceivedTotal)
			_ = registry.RegisterGauge("reactive_workflow", "pending_callbacks", metricsInstance.pendingCallbacks)
			_ = registry.RegisterHistogramVec("reactive_workflow", "callback_latency_seconds", metricsInstance.callbackLatency)
		} else {
			// Fallback to default prometheus registry for testing
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionsCreated)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionsCompleted)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionsFailed)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionsTimedOut)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionsEscalated)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.activeExecutions)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.ruleEvaluationsTotal)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.ruleFiringsTotal)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.actionsDispatchedTotal)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.executionDuration)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.ruleDuration)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.callbacksReceivedTotal)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.pendingCallbacks)
			_ = prometheus.DefaultRegisterer.Register(metricsInstance.callbackLatency)
		}
	})
	return metricsInstance
}

// RecordRuleEvaluation records a rule evaluation (implements MetricsRecorder).
func (m *EngineMetrics) RecordRuleEvaluation(workflowID, ruleID string, fired bool) {
	m.ruleEvaluationsTotal.WithLabelValues(workflowID, ruleID).Inc()
	if fired {
		m.ruleFiringsTotal.WithLabelValues(workflowID, ruleID).Inc()
	}
}

// RecordActionDispatch records an action dispatch (implements MetricsRecorder).
func (m *EngineMetrics) RecordActionDispatch(workflowID, ruleID, actionType string) {
	m.actionsDispatchedTotal.WithLabelValues(workflowID, ruleID, actionType).Inc()
}

// RecordExecutionCreated records a new execution being created (implements MetricsRecorder).
func (m *EngineMetrics) RecordExecutionCreated(workflowID string) {
	m.executionsCreated.WithLabelValues(workflowID).Inc()
	m.activeExecutions.WithLabelValues(workflowID).Inc()
}

// RecordExecutionCompleted records a successful execution completion.
func (m *EngineMetrics) RecordExecutionCompleted(workflowID string, durationSeconds float64) {
	m.executionsCompleted.WithLabelValues(workflowID).Inc()
	m.activeExecutions.WithLabelValues(workflowID).Dec()
	m.executionDuration.WithLabelValues(workflowID, "completed").Observe(durationSeconds)
}

// RecordExecutionFailed records a failed execution.
func (m *EngineMetrics) RecordExecutionFailed(workflowID, reason string, durationSeconds float64) {
	m.executionsFailed.WithLabelValues(workflowID, reason).Inc()
	m.activeExecutions.WithLabelValues(workflowID).Dec()
	m.executionDuration.WithLabelValues(workflowID, "failed").Observe(durationSeconds)
}

// RecordExecutionTimedOut records an execution that timed out.
func (m *EngineMetrics) RecordExecutionTimedOut(workflowID string, durationSeconds float64) {
	m.executionsTimedOut.WithLabelValues(workflowID).Inc()
	m.activeExecutions.WithLabelValues(workflowID).Dec()
	m.executionDuration.WithLabelValues(workflowID, "timed_out").Observe(durationSeconds)
}

// RecordExecutionEscalated records an execution that was escalated.
func (m *EngineMetrics) RecordExecutionEscalated(workflowID, reason string, durationSeconds float64) {
	m.executionsEscalated.WithLabelValues(workflowID, reason).Inc()
	m.activeExecutions.WithLabelValues(workflowID).Dec()
	m.executionDuration.WithLabelValues(workflowID, "escalated").Observe(durationSeconds)
}

// RecordRuleDuration records the duration of a rule evaluation and execution.
func (m *EngineMetrics) RecordRuleDuration(workflowID, ruleID string, durationSeconds float64) {
	m.ruleDuration.WithLabelValues(workflowID, ruleID).Observe(durationSeconds)
}

// RecordCallbackReceived records receiving an async callback.
func (m *EngineMetrics) RecordCallbackReceived(workflowID, ruleID, status string, latencySeconds float64) {
	m.callbacksReceivedTotal.WithLabelValues(workflowID, ruleID, status).Inc()
	m.callbackLatency.WithLabelValues(workflowID, ruleID).Observe(latencySeconds)
}

// SetPendingCallbacks sets the current number of pending callbacks.
func (m *EngineMetrics) SetPendingCallbacks(count int) {
	m.pendingCallbacks.Set(float64(count))
}

// IncrementPendingCallbacks increments the pending callbacks counter.
func (m *EngineMetrics) IncrementPendingCallbacks() {
	m.pendingCallbacks.Inc()
}

// DecrementPendingCallbacks decrements the pending callbacks counter.
func (m *EngineMetrics) DecrementPendingCallbacks() {
	m.pendingCallbacks.Dec()
}
