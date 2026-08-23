// Package metrics provides the framework's Prometheus implementation of
// [component.MetricsRecorder].
//
// It records two things: the status condition metrics of
// [go-crd-condition-metrics], and the framework's own resource-level apply
// counters. Wire one [Recorder] per controller and share a single [Collectors]
// across the process:
//
//	var (
//	    conditions = ocm.NewOperatorConditionsGauge("myoperator")
//	    collectors = metrics.NewCollectors()
//	)
//
//	func init() {
//	    ctrlmetrics.Registry.MustRegister(conditions, collectors)
//	}
//
//	rec := component.ReconcileContext{
//	    // ...
//	    Metrics: metrics.NewRecorder("webapp", conditions, collectors),
//	}
//
// The controller name must match the controller-runtime controller name (the
// lower-cased kind by default) so the dashboards and alerts shipped under
// observability/ correlate the framework's series with controller-runtime's.
//
// [go-crd-condition-metrics]: https://github.com/sourcehawk/go-crd-condition-metrics
package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// Label names of the resource-level metrics, in the order the counters declare
// them. The `controller` label separates operators, and controllers within an
// operator, that share one registry.
var (
	applyLabels = []string{"controller", "owner_kind", "component", "resource", "kind", "operation"}
	errorLabels = []string{"controller", "owner_kind", "component", "resource", "kind"}
)

// Collectors holds the framework's resource-level Prometheus collectors:
// ocf_resource_apply_total and ocf_resource_apply_errors_total.
//
// Construct one per process and register it once, before any Recorder that
// uses it starts reconciling:
//
//	collectors := metrics.NewCollectors()
//	ctrlmetrics.Registry.MustRegister(collectors)
//
// Every controller in the process then shares it, told apart by the
// `controller` label their Recorder supplies.
//
// The series are keyed only by the operator's static topology: controller,
// owner kind, component, resource identifier, kind and operation. No owner name
// or namespace appears, so the same handful of series covers three owners or
// three thousand, and no series needs removing when an owner is deleted. That
// holds only while every resource identifier stays low-cardinality, which is
// the contract WithMetricsIdentifier documents on the resource builders.
type Collectors struct {
	applies *prometheus.CounterVec
	errors  *prometheus.CounterVec
}

// NewCollectors creates the framework's resource-level collectors. Register the
// result with a Prometheus registry, typically controller-runtime's.
func NewCollectors() *Collectors {
	return &Collectors{
		applies: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ocf_resource_apply_total",
			Help: "Framework applies of a managed resource, by outcome.",
		}, applyLabels),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ocf_resource_apply_errors_total",
			Help: "Failed framework applies of a managed resource.",
		}, errorLabels),
	}
}

// Describe implements prometheus.Collector.
func (c *Collectors) Describe(ch chan<- *prometheus.Desc) {
	c.applies.Describe(ch)
	c.errors.Describe(ch)
}

// Collect implements prometheus.Collector.
func (c *Collectors) Collect(ch chan<- prometheus.Metric) {
	c.applies.Collect(ch)
	c.errors.Collect(ch)
}

var _ prometheus.Collector = (*Collectors)(nil)

// Recorder is the framework's Prometheus implementation of
// [component.MetricsRecorder]. Create one per controller with [NewRecorder] and
// assign it to [component.ReconcileContext.Metrics].
//
// Both halves are independently optional: a Recorder built without a conditions
// gauge records only resource metrics, and one built without collectors records
// only condition metrics.
type Recorder struct {
	controller string
	conditions *ocm.ConditionMetricRecorder
	collectors *Collectors
}

var _ component.MetricsRecorder = (*Recorder)(nil)

// NewRecorder creates a Recorder for the named controller.
//
// controller is the value of the `controller` label on every series the
// recorder emits, condition metrics included, so it must match the name
// controller-runtime uses for that controller (the lower-cased kind passed to
// For, unless Named overrides it); the shipped dashboards and alerts filter
// both families with one controller label. conditions and collectors are the
// shared, registered collectors; passing nil for either disables that half of
// the recording rather than panicking at reconcile time.
func NewRecorder(
	controller string, conditions *ocm.OperatorConditionsGauge, collectors *Collectors,
) *Recorder {
	r := &Recorder{controller: controller, collectors: collectors}
	if conditions != nil {
		r.conditions = &ocm.ConditionMetricRecorder{
			Controller:              controller,
			OperatorConditionsGauge: conditions,
		}
	}
	return r
}

// RecordConditionFor records a condition change for the given object and kind.
// It is a no-op when the recorder was built without a conditions gauge.
func (r *Recorder) RecordConditionFor(
	kind string, object ocm.ObjectLike,
	conditionType, conditionStatus, conditionReason string, lastTransitionTime time.Time,
	extraLabelValues ...string,
) {
	if r.conditions == nil {
		return
	}
	r.conditions.RecordConditionFor(
		kind, object, conditionType, conditionStatus, conditionReason, lastTransitionTime,
		extraLabelValues...,
	)
}

// RemoveConditionsFor deletes every condition metric for the given object,
// returning the number of time series removed. Call it when the object is
// deleted, so its condition series do not outlive it.
//
// There is deliberately no counterpart for the resource-level metrics: those
// series carry no owner identity, so they do not accumulate per object, and
// deleting a counter mid-flight reads downstream as a counter reset.
//
// It returns zero when the recorder was built without a conditions gauge.
func (r *Recorder) RemoveConditionsFor(kind string, object ocm.ObjectLike) int {
	if r.conditions == nil {
		return 0
	}
	return r.conditions.RemoveConditionsFor(kind, object)
}

// RecordResourceApply records one framework apply of a managed resource. It is
// a no-op when the recorder was built without collectors.
func (r *Recorder) RecordResourceApply(
	labels component.ResourceMetricLabels, operation concepts.ConvergingOperation,
) {
	if r.collectors == nil {
		return
	}
	r.collectors.applies.WithLabelValues(
		r.controller, labels.OwnerKind, labels.Component, labels.Identifier, labels.Kind,
		strings.ToLower(string(operation)),
	).Inc()
}

// RecordResourceApplyError records one failed framework apply of a managed
// resource. It is a no-op when the recorder was built without collectors.
func (r *Recorder) RecordResourceApplyError(labels component.ResourceMetricLabels) {
	if r.collectors == nil {
		return
	}
	r.collectors.errors.WithLabelValues(
		r.controller, labels.OwnerKind, labels.Component, labels.Identifier, labels.Kind,
	).Inc()
}
