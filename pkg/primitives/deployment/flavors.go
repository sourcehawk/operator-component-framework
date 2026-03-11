package deployment

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	"github.com/sourcehawk/operator-component-framework/pkg/flavors/utils"
	appsv1 "k8s.io/api/apps/v1"
)

// FieldApplicationFlavor defines a function signature for applying "flavors" to a resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*appsv1.Deployment]

// PreserveCurrentLabels ensures that any labels present on the current live
// Deployment but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *appsv1.Deployment) error {
	return flavors.PreserveCurrentLabels[*appsv1.Deployment]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live Deployment but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *appsv1.Deployment) error {
	return flavors.PreserveCurrentAnnotations[*appsv1.Deployment]()(applied, current, desired)
}

// PreserveCurrentPodTemplateLabels ensures that any labels present on the
// current live Deployment's pod template but missing from the applied
// (desired) object's pod template are preserved.
// If a label exists in both, the applied value wins.
//
// Note: pod template metadata changes may affect the rollout hash of the Deployment.
func PreserveCurrentPodTemplateLabels(applied, current, _ *appsv1.Deployment) error {
	applied.Spec.Template.Labels = utils.PreserveMap(applied.Spec.Template.Labels, current.Spec.Template.Labels)
	return nil
}

// PreserveCurrentPodTemplateAnnotations ensures that any annotations present
// on the current live Deployment's pod template but missing from the applied
// (desired) object's pod template are preserved.
// If an annotation exists in both, the applied value wins.
//
// Note: pod template metadata changes may affect the rollout hash of the Deployment.
func PreserveCurrentPodTemplateAnnotations(applied, current, _ *appsv1.Deployment) error {
	applied.Spec.Template.Annotations = utils.PreserveMap(applied.Spec.Template.Annotations, current.Spec.Template.Annotations)
	return nil
}
