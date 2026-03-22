// Package resources provides resource implementations for the rolebinding primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/rolebinding-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/rolebinding"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewRoleBindingResource constructs a rolebinding primitive resource with all the features.
func NewRoleBindingResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base RoleBinding object.
	//
	// roleRef is set here because it is immutable after creation.
	// Subjects are managed via mutations.
	base := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-binding",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     owner.Name + "-role",
		},
	}

	// 2. Initialize the rolebinding builder.
	builder := rolebinding.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.BaseSubjectsMutation(
		owner.Spec.Version, owner.Name+"-sa", owner.Namespace,
	))
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.MonitoringSubjectMutation(
		owner.Spec.Version, owner.Spec.EnableMetrics,
	))

	// 4. Preserve labels added by external controllers.
	builder.WithFieldApplicationFlavor(rolebinding.PreserveCurrentLabels)

	// 5. Extract data from the reconciled RoleBinding.
	builder.WithDataExtractor(func(rb rbacv1.RoleBinding) error {
		fmt.Printf("Reconciled RoleBinding: %s\n", rb.Name)
		fmt.Printf("  RoleRef: %s/%s\n", rb.RoleRef.Kind, rb.RoleRef.Name)
		for _, s := range rb.Subjects {
			fmt.Printf("  Subject: %s/%s (ns: %s)\n", s.Kind, s.Name, s.Namespace)
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
