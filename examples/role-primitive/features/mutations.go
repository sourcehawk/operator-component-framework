// Package features provides sample mutations for the role primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/role"
	rbacv1 "k8s.io/api/rbac/v1"
)

// BaseRuleMutation sets the foundational RBAC rules for the application.
// It is always enabled.
func BaseRuleMutation(version string) role.Mutation {
	return role.Mutation{
		Name:    "base-rules",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *role.Mutator) error {
			m.EditRules(func(e *editors.PolicyRulesEditor) error {
				e.SetRules([]rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "list", "watch"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"get", "list"},
					},
				})
				return nil
			})
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the Role.
// It is always enabled.
func VersionLabelMutation(version string) role.Mutation {
	return role.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *role.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// SecretAccessMutation adds permission to read secrets.
// It is enabled when enableTracing is true (tracing requires reading TLS secrets).
func SecretAccessMutation(version string, enableTracing bool) role.Mutation {
	return role.Mutation{
		Name:    "secret-access",
		Feature: feature.NewResourceFeature(version, nil).When(enableTracing),
		Mutate: func(m *role.Mutator) error {
			m.EditRules(func(e *editors.PolicyRulesEditor) error {
				e.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"get", "list"},
				})
				return nil
			})
			return nil
		},
	}
}

// MetricsAccessMutation adds permission to read services for metrics scraping.
// It is enabled when enableMetrics is true.
func MetricsAccessMutation(version string, enableMetrics bool) role.Mutation {
	return role.Mutation{
		Name:    "metrics-access",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
		Mutate: func(m *role.Mutator) error {
			m.EditRules(func(e *editors.PolicyRulesEditor) error {
				e.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"services", "endpoints"},
					Verbs:     []string{"get", "list", "watch"},
				})
				return nil
			})
			return nil
		},
	}
}
