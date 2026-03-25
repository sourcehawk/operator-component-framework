// Package features provides sample features for the HPA primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/hpa"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// CPUMetricFeature configures CPU-based autoscaling with the given utilization target.
func CPUMetricFeature(version string, targetUtilization int32) hpa.Mutation {
	return hpa.Mutation{
		Name:    "CPUMetric",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *hpa.Mutator) error {
			m.EditHPASpec(func(e *editors.HPASpecEditor) error {
				e.EnsureMetric(autoscalingv2.MetricSpec{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(targetUtilization),
						},
					},
				})
				return nil
			})

			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})

			return nil
		},
	}
}

// MemoryMetricFeature adds memory-based autoscaling when enabled.
func MemoryMetricFeature(enabled bool, targetUtilization int32) hpa.Mutation {
	return hpa.Mutation{
		Name:    "MemoryMetric",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *hpa.Mutator) error {
			m.EditHPASpec(func(e *editors.HPASpecEditor) error {
				e.EnsureMetric(autoscalingv2.MetricSpec{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To(targetUtilization),
						},
					},
				})
				return nil
			})
			return nil
		},
	}
}

// ScaleBehaviorFeature configures conservative scale-down behavior.
func ScaleBehaviorFeature() hpa.Mutation {
	return hpa.Mutation{
		Name: "ScaleBehavior",
		Mutate: func(m *hpa.Mutator) error {
			m.EditHPASpec(func(e *editors.HPASpecEditor) error {
				e.SetBehavior(&autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: ptr.To(int32(300)),
					},
				})
				return nil
			})
			return nil
		},
	}
}
