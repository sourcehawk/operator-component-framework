// Package resources provides resource implementations for the pv primitive example.
package resources

import (
	"github.com/sourcehawk/operator-component-framework/examples/pv-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewPVResource constructs a PersistentVolume primitive resource with all the features.
func NewPVResource(owner *sharedapp.ExampleApp) (*pv.Resource, error) {
	// 1. Create the base PersistentVolume object.
	//    PVs are cluster-scoped — no namespace is set.
	base := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: owner.Name + "-data",
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/mnt/data",
				},
			},
			StorageClassName: "standard",
		},
	}

	// 2. Initialize the PV builder.
	builder := pv.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.RetainPolicyMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	builder.WithMutation(features.MountOptionsMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Build the final resource.
	return builder.Build()
}
