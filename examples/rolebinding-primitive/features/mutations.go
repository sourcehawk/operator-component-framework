// Package features provides sample mutations for the rolebinding primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/rolebinding"
	rbacv1 "k8s.io/api/rbac/v1"
)

// BaseSubjectsMutation adds the application's primary service account as a
// subject. It is always enabled.
func BaseSubjectsMutation(version, saName, saNamespace string) rolebinding.Mutation {
	return rolebinding.Mutation{
		Name:    "base-subjects",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *rolebinding.Mutator) error {
			m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
				e.EnsureSubject(rbacv1.Subject{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: saNamespace,
				})
				return nil
			})
			return nil
		},
	}
}

// VersionLabelMutation sets the app.kubernetes.io/version label on the
// RoleBinding. It is always enabled.
func VersionLabelMutation(version string) rolebinding.Mutation {
	return rolebinding.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *rolebinding.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// MonitoringSubjectMutation adds a monitoring service account as a subject.
// It is enabled when enableMonitoring is true.
func MonitoringSubjectMutation(version string, enableMonitoring bool) rolebinding.Mutation {
	return rolebinding.Mutation{
		Name:    "monitoring-subject",
		Feature: feature.NewResourceFeature(version, nil).When(enableMonitoring),
		Mutate: func(m *rolebinding.Mutator) error {
			m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
				e.EnsureSubject(rbacv1.Subject{
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
