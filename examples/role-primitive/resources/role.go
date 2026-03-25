// Package resources provides resource implementations for the role primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/role-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/role"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewRoleResource constructs a role primitive resource with all the features.
func NewRoleResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base Role object with core permissions.
	base := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-role",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
	}

	// 2. Initialize the role builder.
	builder := role.NewBuilder(base)

	// 3. Register mutations in dependency order.
	//
	// BaseRuleMutation sets the foundational permissions. The version label
	// mutation always runs to track the app version. Secret access is
	// conditionally added based on the tracing flag (as a stand-in for a
	// feature that requires reading secrets).
	builder.WithMutation(features.BaseRuleMutation(owner.Spec.Version))
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.SecretAccessMutation(owner.Spec.Version, owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsAccessMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Extract data from the reconciled Role.
	builder.WithDataExtractor(func(r rbacv1.Role) error {
		fmt.Printf("Reconciled Role: %s\n", r.Name)
		for i, rule := range r.Rules {
			fmt.Printf("  Rule %d: apiGroups=%v resources=%v verbs=%v\n",
				i, rule.APIGroups, rule.Resources, rule.Verbs)
		}
		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
