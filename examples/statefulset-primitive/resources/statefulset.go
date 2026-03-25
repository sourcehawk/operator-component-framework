// Package resources provides resource implementations for the statefulset primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/statefulset-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/statefulset-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewStatefulSetResource constructs a statefulset primitive resource with all the features.
func NewStatefulSetResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base statefulset object.
	base := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-statefulset",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: owner.Name + "-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": owner.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": owner.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "my-app:latest", // Will be overwritten by VersionFeature
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("10Gi"),
							},
						},
					},
				},
			},
		},
	}

	// 2. Initialize the statefulset builder.
	builder := statefulset.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsFeature(owner.Spec.EnableMetrics, 9090))

	// 4. Configure custom status handlers.
	builder.WithCustomConvergeStatus(features.CustomConvergeStatus())
	builder.WithCustomGraceStatus(features.CustomGraceStatus())

	// 5. Configure custom suspension logic.
	builder.WithCustomSuspendMutation(features.CustomSuspendMutation())

	// 6. Data extraction (optional).
	builder.WithDataExtractor(func(s appsv1.StatefulSet) error {
		fmt.Printf("Reconciling statefulset: %s, ready replicas: %d\n", s.Name, s.Status.ReadyReplicas)

		y, err := yaml.Marshal(s)
		if err != nil {
			return fmt.Errorf("failed to marshal statefulset to yaml: %w", err)
		}
		fmt.Printf("Complete StatefulSet Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 7. Build the final resource.
	return builder.Build()
}
