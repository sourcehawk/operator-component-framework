// Package resources provides resource implementations for the secret primitive example.
package resources

import (
	"encoding/base64"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/secret-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewSecretResource constructs a secret primitive resource with all the features.
func NewSecretResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base Secret object.
	base := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-credentials",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
	}

	// 2. Initialize the secret builder.
	builder := secret.NewBuilder(base)

	// 3. Register mutations in dependency order.
	//
	// BaseCredentialsMutation and VersionLabelMutation always run first to establish
	// the baseline. Tracing and metrics tokens are then added on top.
	builder.WithMutation(features.BaseCredentialsMutation(owner.Spec.Version))
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.TracingTokenMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsTokenMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Preserve entries added by external controllers or admission webhooks.
	builder.WithFieldApplicationFlavor(features.PreserveExternalEntriesFlavor())

	// 5. Extract data from the reconciled Secret.
	builder.WithDataExtractor(func(s corev1.Secret) error {
		fmt.Printf("Reconciled Secret: %s\n", s.Name)
		for key, value := range s.Data {
			fmt.Printf("  [%s]: %s\n", key, base64.StdEncoding.EncodeToString(value))
		}
		for key, value := range s.StringData {
			fmt.Printf("  [%s] (stringData): %s\n", key, value)
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
