// Package resources provides resource implementations for the clusterrolebinding primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/clusterrolebinding-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrolebinding"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewClusterRoleBindingResource constructs a clusterrolebinding primitive resource
// with all the features.
func NewClusterRoleBindingResource(owner *sharedapp.ExampleApp) (*clusterrolebinding.Resource, error) {
	// 1. Create the base ClusterRoleBinding object.
	base := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: owner.Name + "-binding",
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     owner.Name + "-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      owner.Name,
				Namespace: owner.Namespace,
			},
		},
	}

	// 2. Initialize the clusterrolebinding builder.
	builder := clusterrolebinding.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.MonitoringSubjectMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Preserve labels added by external controllers.
	builder.WithFieldApplicationFlavor(clusterrolebinding.PreserveCurrentLabels)

	// 5. Extract data from the reconciled ClusterRoleBinding.
	builder.WithDataExtractor(func(crb rbacv1.ClusterRoleBinding) error {
		fmt.Printf("Reconciled ClusterRoleBinding: %s\n", crb.Name)
		fmt.Printf("  RoleRef: %s/%s\n", crb.RoleRef.Kind, crb.RoleRef.Name)
		fmt.Printf("  Subjects (%d):\n", len(crb.Subjects))
		for _, s := range crb.Subjects {
			fmt.Printf("    - %s %s/%s\n", s.Kind, s.Namespace, s.Name)
		}
		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
