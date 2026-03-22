package job

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	"github.com/sourcehawk/operator-component-framework/pkg/flavors/utils"
	batchv1 "k8s.io/api/batch/v1"
)

// FieldApplicationFlavor defines a function signature for applying "flavors" to a resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*batchv1.Job]

// PreserveCurrentLabels ensures that any labels present on the current live
// Job but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *batchv1.Job) error {
	return flavors.PreserveCurrentLabels[*batchv1.Job]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live Job but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *batchv1.Job) error {
	return flavors.PreserveCurrentAnnotations[*batchv1.Job]()(applied, current, desired)
}

// PreserveCurrentPodTemplateLabels ensures that any labels present on the
// current live Job's pod template but missing from the applied
// (desired) object's pod template are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentPodTemplateLabels(applied, current, _ *batchv1.Job) error {
	applied.Spec.Template.Labels = utils.PreserveMap(applied.Spec.Template.Labels, current.Spec.Template.Labels)
	return nil
}

// PreserveCurrentPodTemplateAnnotations ensures that any annotations present
// on the current live Job's pod template but missing from the applied
// (desired) object's pod template are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentPodTemplateAnnotations(applied, current, _ *batchv1.Job) error {
	applied.Spec.Template.Annotations = utils.PreserveMap(applied.Spec.Template.Annotations, current.Spec.Template.Annotations)
	return nil
}
