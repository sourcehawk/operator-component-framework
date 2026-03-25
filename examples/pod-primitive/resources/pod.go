// Package resources provides resource implementations for the pod primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/pod-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/pod-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewPodResource constructs a pod primitive resource with all the features.
func NewPodResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base pod object.
	base := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-pod",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "my-app:latest", // Base image for the app container
				},
			},
		},
	}

	// 2. Initialize the pod builder.
	builder := pod.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))

	// 4. Data extraction (optional).
	builder.WithDataExtractor(func(p corev1.Pod) error {
		fmt.Printf("Reconciling pod: %s, phase: %s\n", p.Name, p.Status.Phase)

		// Print the complete pod resource object as yaml
		y, err := yaml.Marshal(p)
		if err != nil {
			return fmt.Errorf("failed to marshal pod to yaml: %w", err)
		}
		fmt.Printf("Complete Pod Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
