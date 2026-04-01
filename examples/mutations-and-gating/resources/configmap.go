package resources

import (
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/app"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseConfigMap returns the desired-state ConfigMap for the given owner.
// The baseline includes the core server configuration in app.yaml.
func BaseConfigMap(owner *app.ExampleApp) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-config",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Data: map[string]string{
			"app.yaml": "server:\n  port: 8080\n  timeout: 30s\n",
		},
	}
}

// NewConfigMapResource constructs a ConfigMap with server and metrics config.
// The metrics section is boolean-gated at the mutation level, while the entire
// ConfigMap can be gated at the resource level by the controller.
func NewConfigMapResource(owner *app.ExampleApp) (component.Resource, error) {
	builder := configmap.NewBuilder(BaseConfigMap(owner))
	builder.WithMutation(features.MetricsConfigMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	return builder.Build()
}
