// Package resources provides resource implementations for the configmap primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/configmap-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewConfigMapResource constructs a configmap primitive resource with all the features.
func NewConfigMapResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base ConfigMap object.
	//
	// app.yaml is initialised to an empty string to declare operator ownership of
	// that key. PreserveExternalEntries only copies keys that are absent from the
	// applied object — an empty value is sufficient to signal ownership and
	// prevent the live cluster value from bleeding into the next reconcile cycle
	// when a feature is toggled off.
	base := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-config",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Data: map[string]string{
			"app.yaml": "",
		},
	}

	// 2. Initialize the configmap builder.
	builder := configmap.NewBuilder(base)

	// 3. Register mutations in dependency order.
	//
	// BaseConfigMutation and VersionLabelMutation always run first to establish
	// the baseline. Tracing and metrics sections are then merged on top.
	builder.WithMutation(features.BaseConfigMutation(owner.Spec.Version))
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.TracingConfigMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsConfigMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Preserve entries added by external controllers or admission webhooks.
	builder.WithFieldApplicationFlavor(features.PreserveExternalEntriesFlavor())

	// 5. Extract data from the reconciled ConfigMap.
	builder.WithDataExtractor(func(cm corev1.ConfigMap) error {
		fmt.Printf("Reconciled ConfigMap: %s\n", cm.Name)
		for key, value := range cm.Data {
			fmt.Printf("  [%s]:\n%s\n", key, value)
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
