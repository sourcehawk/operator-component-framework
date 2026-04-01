// Package resources provides resource factories for the mutations-and-gating example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/app"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseDeployment returns the desired-state Deployment for the given owner.
//
// The baseline represents the latest version (v2+): container named "app"
// with both an HTTP and a health port. Backward compatibility mutations roll
// this back for older versions.
func BaseDeployment(owner *app.ExampleApp) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-app",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": owner.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": owner.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: fmt.Sprintf("my-app:%s", owner.Spec.Version),
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
								{Name: "health", ContainerPort: 8081},
							},
						},
					},
				},
			},
		},
	}
}

// NewDeploymentResource constructs a Deployment with version-gated and
// boolean-gated mutations.
//
// Registration order matters: DebugLogging targets the baseline container name
// ("app") via ContainerNamed, so it must come before BackwardCompatV1Container
// which renames "app" to "server" for older versions. TracingSidecar uses
// AllContainers and is order-insensitive.
func NewDeploymentResource(owner *app.ExampleApp) (component.Resource, error) {
	return deployment.NewBuilder(BaseDeployment(owner)).
		WithMutation(features.DebugLoggingMutation(owner.Spec.EnableDebugLogging)).
		WithMutation(features.BackwardCompatV1Container(owner.Spec.Version)).
		WithMutation(features.TracingSidecarMutation(owner.Spec.EnableTracing)).
		Build()
}
