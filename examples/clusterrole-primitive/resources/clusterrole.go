// Package resources provides resource implementations for the clusterrole primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/clusterrole-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrole"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewClusterRoleResource constructs a clusterrole primitive resource with all the features.
func NewClusterRoleResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base ClusterRole object.
	// ClusterRole is cluster-scoped — no namespace is set.
	base := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: owner.Name + "-role",
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
	}

	// 2. Initialize the clusterrole builder.
	builder := clusterrole.NewBuilder(base)

	// 3. Register mutations in dependency order.
	// CoreRulesMutation and VersionLabelMutation always run first.
	// SecretAccess and DeploymentAccess are feature-gated.
	builder.WithMutation(features.CoreRulesMutation())
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.SecretAccessMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	builder.WithMutation(features.DeploymentAccessMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Extract data from the reconciled ClusterRole.
	builder.WithDataExtractor(func(cr rbacv1.ClusterRole) error {
		fmt.Printf("Reconciled ClusterRole: %s\n", cr.Name)
		fmt.Printf("  Rules: %d\n", len(cr.Rules))
		for i, rule := range cr.Rules {
			fmt.Printf("    [%d] APIGroups=%v Resources=%v Verbs=%v\n",
				i, rule.APIGroups, rule.Resources, rule.Verbs)
		}
		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
