package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func int32Ptr(v int32) *int32 { return &v }

func resourcePtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func TestHPASpecEditor_SetScaleTargetRef(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	ref := autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "web",
	}
	e.SetScaleTargetRef(ref)

	assert.Equal(t, ref, spec.ScaleTargetRef)
}

func TestHPASpecEditor_SetMinReplicas(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	e.SetMinReplicas(int32Ptr(2))
	require.NotNil(t, spec.MinReplicas)
	assert.Equal(t, int32(2), *spec.MinReplicas)

	e.SetMinReplicas(nil)
	assert.Nil(t, spec.MinReplicas)
}

func TestHPASpecEditor_SetMaxReplicas(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	e.SetMaxReplicas(10)
	assert.Equal(t, int32(10), spec.MaxReplicas)
}

func TestHPASpecEditor_EnsureMetric_Resource(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	cpuMetric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: int32Ptr(80),
			},
		},
	}

	// Add CPU metric
	e.EnsureMetric(cpuMetric)
	require.Len(t, spec.Metrics, 1)
	assert.Equal(t, int32(80), *spec.Metrics[0].Resource.Target.AverageUtilization)

	// Update CPU metric (upsert by resource name)
	updatedCPU := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: int32Ptr(50),
			},
		},
	}
	e.EnsureMetric(updatedCPU)
	require.Len(t, spec.Metrics, 1)
	assert.Equal(t, int32(50), *spec.Metrics[0].Resource.Target.AverageUtilization)

	// Add memory metric (different resource name)
	memMetric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceMemory,
			Target: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: resourcePtr("500Mi"),
			},
		},
	}
	e.EnsureMetric(memMetric)
	assert.Len(t, spec.Metrics, 2)
}

func TestHPASpecEditor_EnsureMetric_Pods(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	metric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.PodsMetricSourceType,
		Pods: &autoscalingv2.PodsMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "requests_per_second"},
			Target: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: resourcePtr("100"),
			},
		},
	}
	e.EnsureMetric(metric)
	require.Len(t, spec.Metrics, 1)

	// Upsert same metric name
	updated := autoscalingv2.MetricSpec{
		Type: autoscalingv2.PodsMetricSourceType,
		Pods: &autoscalingv2.PodsMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "requests_per_second"},
			Target: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: resourcePtr("200"),
			},
		},
	}
	e.EnsureMetric(updated)
	assert.Len(t, spec.Metrics, 1)
}

func TestHPASpecEditor_EnsureMetric_Object(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	metric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ObjectMetricSourceType,
		Object: &autoscalingv2.ObjectMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "queue_length"},
			DescribedObject: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "worker",
			},
			Target: autoscalingv2.MetricTarget{
				Type:  autoscalingv2.ValueMetricType,
				Value: resourcePtr("30"),
			},
		},
	}
	e.EnsureMetric(metric)
	require.Len(t, spec.Metrics, 1)
	assert.Equal(t, "queue_length", spec.Metrics[0].Object.Metric.Name)
}

func TestHPASpecEditor_EnsureMetric_External(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	metric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "pubsub_undelivered"},
			Target: autoscalingv2.MetricTarget{
				Type:  autoscalingv2.ValueMetricType,
				Value: resourcePtr("100"),
			},
		},
	}
	e.EnsureMetric(metric)
	require.Len(t, spec.Metrics, 1)
	assert.Equal(t, "pubsub_undelivered", spec.Metrics[0].External.Metric.Name)
}

func TestHPASpecEditor_EnsureMetric_ContainerResource(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	metric := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ContainerResourceMetricSourceType,
		ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
			Name:      corev1.ResourceCPU,
			Container: "app",
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: int32Ptr(70),
			},
		},
	}
	e.EnsureMetric(metric)
	require.Len(t, spec.Metrics, 1)

	// Different container same resource -> separate entry
	metric2 := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ContainerResourceMetricSourceType,
		ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
			Name:      corev1.ResourceCPU,
			Container: "sidecar",
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: int32Ptr(50),
			},
		},
	}
	e.EnsureMetric(metric2)
	assert.Len(t, spec.Metrics, 2)
}

func TestHPASpecEditor_RemoveMetric(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{
		Metrics: []autoscalingv2.MetricSpec{
			{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32Ptr(80),
					},
				},
			},
			{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceMemory,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32Ptr(70),
					},
				},
			},
		},
	}
	e := NewHPASpecEditor(spec)

	e.RemoveMetric(autoscalingv2.ResourceMetricSourceType, "cpu")
	require.Len(t, spec.Metrics, 1)
	assert.Equal(t, corev1.ResourceMemory, spec.Metrics[0].Resource.Name)
}

func TestHPASpecEditor_RemoveMetric_NoMatch(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{
		Metrics: []autoscalingv2.MetricSpec{
			{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
				},
			},
		},
	}
	e := NewHPASpecEditor(spec)

	e.RemoveMetric(autoscalingv2.ResourceMetricSourceType, "memory")
	assert.Len(t, spec.Metrics, 1)
}

func TestHPASpecEditor_SetBehavior(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{}
	e := NewHPASpecEditor(spec)

	stabilization := int32(300)
	behavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{
		ScaleDown: &autoscalingv2.HPAScalingRules{
			StabilizationWindowSeconds: &stabilization,
		},
	}
	e.SetBehavior(behavior)
	require.NotNil(t, spec.Behavior)
	assert.Equal(t, int32(300), *spec.Behavior.ScaleDown.StabilizationWindowSeconds)

	e.SetBehavior(nil)
	assert.Nil(t, spec.Behavior)
}

func TestHPASpecEditor_Raw(t *testing.T) {
	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{
		MaxReplicas: 5,
	}
	e := NewHPASpecEditor(spec)

	raw := e.Raw()
	assert.Equal(t, int32(5), raw.MaxReplicas)

	raw.MaxReplicas = 10
	assert.Equal(t, int32(10), spec.MaxReplicas)
}
