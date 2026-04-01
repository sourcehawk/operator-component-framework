// Package resources provides resource factories for the grace-inconsistency example.
package resources

import (
	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseDeployment returns the desired-state Deployment for the monitoring sidecar.
func BaseDeployment(owner *app.ExampleApp) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-monitoring",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": owner.Name, "role": "monitoring"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": owner.Name, "role": "monitoring"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "prometheus-exporter",
							Image: "prom/node-exporter:v1.3.1",
						},
					},
				},
			},
		},
	}
}

// NewDeploymentResource constructs a Deployment with a custom grace handler
// that always returns Healthy, regardless of replica readiness.
//
// This is useful for soft-dependency resources like a monitoring sidecar: if it
// has not converged yet, the operator still considers the component healthy
// rather than blocking on it.
func NewDeploymentResource(owner *app.ExampleApp) (component.Resource, error) {
	builder := deployment.NewBuilder(BaseDeployment(owner))

	builder.WithCustomGraceStatus(func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "monitoring is a soft dependency, always considered healthy",
		}, nil
	})

	return builder.Build()
}
