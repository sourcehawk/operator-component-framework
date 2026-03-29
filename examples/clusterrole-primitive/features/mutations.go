// Package features provides sample mutations for the clusterrole primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrole"
	rbacv1 "k8s.io/api/rbac/v1"
)

// CoreRulesMutation grants read access to core resources (pods, services, configmaps).
// It is always enabled — Feature is nil so it applies unconditionally.
func CoreRulesMutation() clusterrole.Mutation {
	return clusterrole.Mutation{
		Name: "core-rules",
		Mutate: func(m *clusterrole.Mutator) error {
			m.AddRule(rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"pods", "services", "configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			})
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the ClusterRole.
// It is always enabled — Feature is nil so it applies unconditionally.
func VersionLabelMutation(version string) clusterrole.Mutation {
	return clusterrole.Mutation{
		Name: "version-label",
		Mutate: func(m *clusterrole.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// SecretAccessMutation grants read access to secrets.
// It is enabled when needsSecrets is true.
func SecretAccessMutation(version string, needsSecrets bool) clusterrole.Mutation {
	return clusterrole.Mutation{
		Name:    "secret-access",
		Feature: feature.NewVersionGate(version, nil).When(needsSecrets),
		Mutate: func(m *clusterrole.Mutator) error {
			m.AddRule(rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			})
			return nil
		},
	}
}

// DeploymentAccessMutation grants read/write access to deployments.
// It is enabled when manageDeployments is true.
func DeploymentAccessMutation(version string, manageDeployments bool) clusterrole.Mutation {
	return clusterrole.Mutation{
		Name:    "deployment-access",
		Feature: feature.NewVersionGate(version, nil).When(manageDeployments),
		Mutate: func(m *clusterrole.Mutator) error {
			m.AddRule(rbacv1.PolicyRule{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
			})
			return nil
		},
	}
}
