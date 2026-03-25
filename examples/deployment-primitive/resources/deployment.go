// Package resources provides resource implementations for the deployment primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/deployment-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/deployment-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewDeploymentResource constructs a deployment primitive resource with all the features.
func NewDeploymentResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base deployment object.
	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-deployment",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
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

	// 2. Initialize the deployment builder.
	builder := deployment.NewBuilder(base)

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
	builder.WithDataExtractor(func(d appsv1.Deployment) error {
		fmt.Printf("Reconciling deployment: %s, ready replicas: %d\n", d.Name, d.Status.ReadyReplicas)

		// Print the complete deployment resource object as yaml
		y, err := yaml.Marshal(d)
		if err != nil {
			return fmt.Errorf("failed to marshal deployment to yaml: %w", err)
		}
		fmt.Printf("Complete Deployment Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 7. Build the final resource.
	return builder.Build()
}
