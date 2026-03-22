// Package resources provides resource implementations for the PVC primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/pvc-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/pvc-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pvc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewPVCResource constructs a PVC primitive resource with all the features.
func NewPVCResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base PVC object.
	base := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-data",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	// 2. Initialize the PVC builder.
	builder := pvc.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.StorageAnnotationMutation(owner.Spec.Version, "standard"))
	builder.WithMutation(features.LargeStorageMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Preserve labels and annotations added by external controllers.
	builder.WithFieldApplicationFlavor(pvc.PreserveCurrentLabels)
	builder.WithFieldApplicationFlavor(pvc.PreserveCurrentAnnotations)

	// 5. Extract data from the reconciled PVC.
	builder.WithDataExtractor(func(p corev1.PersistentVolumeClaim) error {
		fmt.Printf("Reconciled PVC: %s\n", p.Name)
		fmt.Printf("  Phase: %s\n", p.Status.Phase)
		if storage, ok := p.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			fmt.Printf("  Storage Request: %s\n", storage.String())
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
