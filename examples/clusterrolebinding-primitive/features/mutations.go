// Package features provides sample mutations for the clusterrolebinding primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrolebinding"
	rbacv1 "k8s.io/api/rbac/v1"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the
// ClusterRoleBinding. It is always enabled.
func VersionLabelMutation(version string) clusterrolebinding.Mutation {
	return clusterrolebinding.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *clusterrolebinding.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// MonitoringSubjectMutation adds a monitoring service account as a subject
// when metrics are enabled.
func MonitoringSubjectMutation(version string, enableMetrics bool) clusterrolebinding.Mutation {
	return clusterrolebinding.Mutation{
		Name:    "monitoring-subject",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
		Mutate: func(m *clusterrolebinding.Mutator) error {
			m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
				e.Add(rbacv1.Subject{
					Kind:      "ServiceAccount",
					Name:      "monitoring-agent",
					Namespace: "monitoring",
				})
				return nil
			})
			return nil
		},
	}
}
