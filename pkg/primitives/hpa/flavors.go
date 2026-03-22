package hpa

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

// FieldApplicationFlavor defines a function signature for applying "flavors" to an HPA resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*autoscalingv2.HorizontalPodAutoscaler]

// PreserveCurrentLabels ensures that any labels present on the current live
// HPA but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *autoscalingv2.HorizontalPodAutoscaler) error {
	return flavors.PreserveCurrentLabels[*autoscalingv2.HorizontalPodAutoscaler]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live HPA but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *autoscalingv2.HorizontalPodAutoscaler) error {
	return flavors.PreserveCurrentAnnotations[*autoscalingv2.HorizontalPodAutoscaler]()(applied, current, desired)
}
