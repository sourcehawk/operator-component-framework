// Package resources provides resource implementations for the serviceaccount primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/serviceaccount-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/serviceaccount"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewServiceAccountResource constructs a serviceaccount primitive resource with all the features.
func NewServiceAccountResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base ServiceAccount object.
	base := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-sa",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
	}

	// 2. Initialize the serviceaccount builder.
	builder := serviceaccount.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.ImagePullSecretMutation(owner.Spec.Version))
	builder.WithMutation(features.PrivateRegistryMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	// In this example, we reuse EnableMetrics to drive the disable-automount feature
	// purely to demonstrate the mutation. A real application would typically use a
	// dedicated DisableAutomount boolean in its spec instead.
	disableAutomount := owner.Spec.EnableMetrics
	builder.WithMutation(features.DisableAutomountMutation(owner.Spec.Version, disableAutomount))

	// 4. Preserve labels added by external controllers.
	builder.WithFieldApplicationFlavor(serviceaccount.PreserveCurrentLabels)

	// 5. Extract data from the reconciled ServiceAccount.
	builder.WithDataExtractor(func(sa corev1.ServiceAccount) error {
		fmt.Printf("Reconciled ServiceAccount: %s\n", sa.Name)
		fmt.Printf("  ImagePullSecrets: ")
		for i, ref := range sa.ImagePullSecrets {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(ref.Name)
		}
		fmt.Println()
		if sa.AutomountServiceAccountToken != nil {
			fmt.Printf("  AutomountServiceAccountToken: %v\n", *sa.AutomountServiceAccountToken)
		} else {
			fmt.Println("  AutomountServiceAccountToken: <nil>")
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
