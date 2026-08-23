package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// runtimeMetrics defines the controller-runtime, workqueue, REST client and
// leader election series an operator exposes, with the same names, label sets
// and bucket layouts as the real ones. The real vectors live in
// controller-runtime's internal packages; runtime_test.go guards this copy
// against drift.
type runtimeMetrics struct {
	reconcileTotal      *prometheus.CounterVec
	reconcileErrors     *prometheus.CounterVec
	reconcilePanics     *prometheus.CounterVec
	reconcileTime       *prometheus.HistogramVec
	maxConcurrent       *prometheus.GaugeVec
	activeWorkers       *prometheus.GaugeVec
	queueDepth          *prometheus.GaugeVec
	queueAdds           *prometheus.CounterVec
	queueDuration       *prometheus.HistogramVec
	workDuration        *prometheus.HistogramVec
	queueUnfinished     *prometheus.GaugeVec
	queueLongestRunning *prometheus.GaugeVec
	queueRetries        *prometheus.CounterVec
	restRequests        *prometheus.CounterVec
	leader              *prometheus.GaugeVec
}

// Label names, the workqueue subsystem and the reconcile result value the
// lookalike series share with controller-runtime.
const (
	labelController    = "controller"
	labelName          = "name"
	subsystemWorkqueue = "workqueue"
	resultError        = "error"
)

// reconcileTimeBuckets mirrors controller-runtime's ReconcileTime histogram.
var reconcileTimeBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
	1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60}

func newRuntimeMetrics(reg prometheus.Registerer) *runtimeMetrics {
	m := &runtimeMetrics{
		reconcileTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "controller_runtime_reconcile_total",
			Help: "Total number of reconciliations per controller",
		}, []string{labelController, "result"}),
		reconcileErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "controller_runtime_reconcile_errors_total",
			Help: "Total number of reconciliation errors per controller",
		}, []string{labelController}),
		reconcilePanics: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "controller_runtime_reconcile_panics_total",
			Help: "Total number of reconciliation panics per controller",
		}, []string{labelController}),
		reconcileTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "controller_runtime_reconcile_time_seconds",
			Help:    "Length of time per reconciliation per controller",
			Buckets: reconcileTimeBuckets,
		}, []string{labelController}),
		maxConcurrent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "controller_runtime_max_concurrent_reconciles",
			Help: "Maximum number of concurrent reconciles per controller",
		}, []string{labelController}),
		activeWorkers: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "controller_runtime_active_workers",
			Help: "Number of currently used workers per controller",
		}, []string{labelController}),
		queueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: subsystemWorkqueue, Name: "depth",
			Help: "Current depth of workqueue by workqueue and priority",
		}, []string{labelName, labelController, "priority"}),
		queueAdds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: subsystemWorkqueue, Name: "adds_total",
			Help: "Total number of adds handled by workqueue",
		}, []string{labelName, labelController}),
		queueDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: subsystemWorkqueue, Name: "queue_duration_seconds",
			Help:    "How long in seconds an item stays in workqueue before being requested",
			Buckets: prometheus.ExponentialBuckets(10e-9, 10, 12),
		}, []string{labelName, labelController}),
		workDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: subsystemWorkqueue, Name: "work_duration_seconds",
			Help:    "How long in seconds processing an item from workqueue takes.",
			Buckets: prometheus.ExponentialBuckets(10e-9, 10, 12),
		}, []string{labelName, labelController}),
		queueUnfinished: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: subsystemWorkqueue, Name: "unfinished_work_seconds",
			Help: "How many seconds of work has been done that " +
				"is in progress and hasn't been observed by work_duration. Large " +
				"values indicate stuck threads. One can deduce the number of stuck " +
				"threads by observing the rate at which this increases.",
		}, []string{labelName, labelController}),
		queueLongestRunning: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: subsystemWorkqueue, Name: "longest_running_processor_seconds",
			Help: "How many seconds has the longest running " +
				"processor for workqueue been running.",
		}, []string{labelName, labelController}),
		queueRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: subsystemWorkqueue, Name: "retries_total",
			Help: "Total number of items added to the workqueue with a non-zero delay (rate-limited requeues, explicit RequeueAfter or AddAfter calls)",
		}, []string{labelName, labelController}),
		restRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		}, []string{"code", "method", "host"}),
		leader: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "leader_election_master_status",
			Help: "Gauge of if the reporting system is master of the relevant lease, 0 indicates backup, 1 indicates master. 'name' is the string used to identify the lease. Please make sure to group by name.",
		}, []string{labelName}),
	}
	reg.MustRegister(
		m.reconcileTotal, m.reconcileErrors, m.reconcilePanics, m.reconcileTime, m.maxConcurrent, m.activeWorkers,
		m.queueDepth, m.queueAdds, m.queueDuration, m.workDuration, m.queueUnfinished, m.queueLongestRunning, m.queueRetries,
		m.restRequests, m.leader,
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)
	return m
}

// initController creates the zero-valued series controller-runtime creates
// when a controller starts, so panels show a flat line instead of no data.
func (m *runtimeMetrics) initController(name string, maxConcurrent int) {
	for _, result := range []string{resultError, "requeue_after", "requeue", "success"} {
		m.reconcileTotal.WithLabelValues(name, result).Add(0)
	}
	m.reconcileErrors.WithLabelValues(name).Add(0)
	m.reconcilePanics.WithLabelValues(name).Add(0)
	m.reconcileTime.WithLabelValues(name)
	m.maxConcurrent.WithLabelValues(name).Set(float64(maxConcurrent))
	m.activeWorkers.WithLabelValues(name).Set(0)
	m.queueDepth.WithLabelValues(name, name, "0").Set(0)
	m.queueAdds.WithLabelValues(name, name).Add(0)
	m.queueDuration.WithLabelValues(name, name)
	m.workDuration.WithLabelValues(name, name)
	m.queueUnfinished.WithLabelValues(name, name).Set(0)
	m.queueLongestRunning.WithLabelValues(name, name).Set(0)
	m.queueRetries.WithLabelValues(name, name).Add(0)
}
