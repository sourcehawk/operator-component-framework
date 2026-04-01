package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseDeployment returns the desired-state Deployment for the app component.
func BaseDeployment(owner *app.ExampleApp) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-app",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
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
							Image: "my-app:latest",
						},
					},
				},
			},
		},
	}
}

// NewDeploymentResource constructs a Deployment for the app component.
func NewDeploymentResource(owner *app.ExampleApp) (component.Resource, error) {
	builder := deployment.NewBuilder(BaseDeployment(owner))
	builder.WithMutation(deployment.Mutation{
		Name:    "Version",
		Feature: feature.NewVersionGate(owner.Spec.Version, nil),
		Mutate: func(m *deployment.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Image = fmt.Sprintf("my-app:%s", owner.Spec.Version)
				return nil
			})
			return nil
		},
	})

	return builder.Build()
}
