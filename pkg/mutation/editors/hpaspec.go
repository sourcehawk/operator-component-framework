package editors

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

// HPASpecEditor provides a typed API for mutating a Kubernetes HorizontalPodAutoscalerSpec.
type HPASpecEditor struct {
	spec *autoscalingv2.HorizontalPodAutoscalerSpec
}

// NewHPASpecEditor creates a new HPASpecEditor for the given HorizontalPodAutoscalerSpec.
func NewHPASpecEditor(spec *autoscalingv2.HorizontalPodAutoscalerSpec) *HPASpecEditor {
	return &HPASpecEditor{spec: spec}
}

// Raw returns the underlying *autoscalingv2.HorizontalPodAutoscalerSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *HPASpecEditor) Raw() *autoscalingv2.HorizontalPodAutoscalerSpec {
	return e.spec
}

// SetScaleTargetRef sets the reference to the resource being scaled.
func (e *HPASpecEditor) SetScaleTargetRef(ref autoscalingv2.CrossVersionObjectReference) {
	e.spec.ScaleTargetRef = ref
}

// SetMinReplicas sets the lower bound for the number of replicas.
// Passing nil removes the lower bound (Kubernetes defaults to 1).
func (e *HPASpecEditor) SetMinReplicas(n *int32) {
	e.spec.MinReplicas = n
}

// SetMaxReplicas sets the upper bound for the number of replicas.
func (e *HPASpecEditor) SetMaxReplicas(n int32) {
	e.spec.MaxReplicas = n
}

// EnsureMetric upserts a metric in the spec's Metrics slice.
//
// Matching is performed by MetricSpec.Type. Within that type, matching is refined
// by the metric name where applicable:
//   - Resource: matched by Resource.Name
//   - Pods: matched by Pods.Metric.Name
//   - Object: matched by Object.Metric.Name
//   - ContainerResource: matched by ContainerResource.Name + ContainerResource.Container
//   - External: matched by External.Metric.Name
//
// If a matching entry exists it is replaced; otherwise the metric is appended.
func (e *HPASpecEditor) EnsureMetric(metric autoscalingv2.MetricSpec) {
	for i, existing := range e.spec.Metrics {
		if metricsMatch(existing, metric) {
			e.spec.Metrics[i] = metric
			return
		}
	}
	e.spec.Metrics = append(e.spec.Metrics, metric)
}

// RemoveMetric removes a metric matching the given type and name.
//
// For Resource metrics, name corresponds to the resource name (e.g. "cpu").
// For Pods, Object, and External metrics, name corresponds to the metric name.
// For ContainerResource metrics, name corresponds to the resource name; all
// container variants of that resource are removed.
//
// If no matching metric is found, this is a no-op.
func (e *HPASpecEditor) RemoveMetric(metricType autoscalingv2.MetricSourceType, name string) {
	filtered := e.spec.Metrics[:0]
	for _, m := range e.spec.Metrics {
		if m.Type == metricType && metricName(m) == name {
			continue
		}
		filtered = append(filtered, m)
	}
	e.spec.Metrics = filtered
}

// SetBehavior sets the autoscaling behavior configuration.
// Passing nil removes custom behavior (Kubernetes uses defaults).
func (e *HPASpecEditor) SetBehavior(behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) {
	e.spec.Behavior = behavior
}

// metricsMatch reports whether two MetricSpec values target the same metric.
func metricsMatch(a, b autoscalingv2.MetricSpec) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if a.Resource == nil || b.Resource == nil {
			return false
		}
		return a.Resource.Name == b.Resource.Name
	case autoscalingv2.PodsMetricSourceType:
		if a.Pods == nil || b.Pods == nil {
			return false
		}
		return a.Pods.Metric.Name == b.Pods.Metric.Name
	case autoscalingv2.ObjectMetricSourceType:
		if a.Object == nil || b.Object == nil {
			return false
		}
		return a.Object.Metric.Name == b.Object.Metric.Name
	case autoscalingv2.ContainerResourceMetricSourceType:
		if a.ContainerResource == nil || b.ContainerResource == nil {
			return false
		}
		return a.ContainerResource.Name == b.ContainerResource.Name &&
			a.ContainerResource.Container == b.ContainerResource.Container
	case autoscalingv2.ExternalMetricSourceType:
		if a.External == nil || b.External == nil {
			return false
		}
		return a.External.Metric.Name == b.External.Metric.Name
	default:
		return false
	}
}

// metricName extracts the identifying name from a MetricSpec.
func metricName(m autoscalingv2.MetricSpec) string {
	switch m.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if m.Resource != nil {
			return string(m.Resource.Name)
		}
	case autoscalingv2.PodsMetricSourceType:
		if m.Pods != nil {
			return m.Pods.Metric.Name
		}
	case autoscalingv2.ObjectMetricSourceType:
		if m.Object != nil {
			return m.Object.Metric.Name
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if m.ContainerResource != nil {
			return string(m.ContainerResource.Name)
		}
	case autoscalingv2.ExternalMetricSourceType:
		if m.External != nil {
			return m.External.Metric.Name
		}
	}
	return ""
}
