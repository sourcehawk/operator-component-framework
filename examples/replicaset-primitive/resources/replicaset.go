// Package resources provides resource implementations for the replicaset primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/replicaset-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/replicaset-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/replicaset"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewReplicaSetResource constructs a replicaset primitive resource with all the features.
func NewReplicaSetResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base replicaset object.
	base := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-replicaset",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: appsv1.ReplicaSetSpec{
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
		},
	}

	// 2. Initialize the replicaset builder.
	builder := replicaset.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsFeature(owner.Spec.EnableMetrics, 9090))

	// 4. Configure flavors (e.g., preserve labels/annotations if they were modified externally).
	builder.WithFieldApplicationFlavor(features.PreserveLabelsFlavor())
	builder.WithFieldApplicationFlavor(features.PreserveAnnotationsFlavor())

	// 5. Data extraction (optional).
	builder.WithDataExtractor(func(rs appsv1.ReplicaSet) error {
		fmt.Printf("Reconciling replicaset: %s, ready replicas: %d\n", rs.Name, rs.Status.ReadyReplicas)

		// Print the complete replicaset resource object as yaml
		y, err := yaml.Marshal(rs)
		if err != nil {
			return fmt.Errorf("failed to marshal replicaset to yaml: %w", err)
		}
		fmt.Printf("Complete ReplicaSet Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
