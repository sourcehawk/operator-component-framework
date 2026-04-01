// Package resources provides resource factories for the component-prerequisites example.
package resources

import (
	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseConfigMap returns the desired-state ConfigMap for the infra component.
func BaseConfigMap(owner *app.ExampleApp) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-infra-config",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
		},
		Data: map[string]string{
			"cluster-dns": "10.96.0.10",
		},
	}
}

// NewConfigMapResource constructs a ConfigMap for the infra component.
func NewConfigMapResource(owner *app.ExampleApp) (component.Resource, error) {
	return configmap.NewBuilder(BaseConfigMap(owner)).Build()
}
