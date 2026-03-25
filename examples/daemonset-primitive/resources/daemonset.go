// Package resources provides resource implementations for the daemonset primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/daemonset-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/daemonset-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/daemonset"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewDaemonSetResource constructs a daemonset primitive resource with all the features.
func NewDaemonSetResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base daemonset object.
	base := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-daemonset",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
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
							Name:  "agent",
							Image: "my-agent:latest", // Will be overwritten by VersionFeature
						},
					},
				},
			},
		},
	}

	// 2. Initialize the daemonset builder.
	builder := daemonset.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsFeature(owner.Spec.EnableMetrics, 9090))

	// 4. Data extraction (optional).
	builder.WithDataExtractor(func(d appsv1.DaemonSet) error {
		fmt.Printf("Reconciling desired DaemonSet object: %s/%s\n", d.Namespace, d.Name)

		// Print the complete daemonset resource object as yaml
		y, err := yaml.Marshal(d)
		if err != nil {
			return fmt.Errorf("failed to marshal daemonset to yaml: %w", err)
		}
		fmt.Printf("Complete DaemonSet Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
