// Package features provides sample mutations for the serviceaccount primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/serviceaccount"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the ServiceAccount.
// It is always enabled.
func VersionLabelMutation(version string) serviceaccount.Mutation {
	return serviceaccount.Mutation{
		Name:    "version-label",
		Feature: nil,
		Mutate: func(m *serviceaccount.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// ImagePullSecretMutation ensures the default registry pull secret is attached
// to the ServiceAccount. It is always enabled.
func ImagePullSecretMutation(version string) serviceaccount.Mutation {
	return serviceaccount.Mutation{
		Name:    "image-pull-secret",
		Feature: nil,
		Mutate: func(m *serviceaccount.Mutator) error {
			m.EnsureImagePullSecret("default-registry-creds")
			return nil
		},
	}
}

// PrivateRegistryMutation adds a private registry pull secret to the ServiceAccount.
// It is enabled when usePrivateRegistry is true.
func PrivateRegistryMutation(version string, usePrivateRegistry bool) serviceaccount.Mutation {
	return serviceaccount.Mutation{
		Name:    "private-registry",
		Feature: feature.NewResourceFeature(version, nil).When(usePrivateRegistry),
		Mutate: func(m *serviceaccount.Mutator) error {
			m.EnsureImagePullSecret("private-registry-creds")
			return nil
		},
	}
}

// DisableAutomountMutation disables automatic mounting of the service account token.
// It is enabled when disableAutomount is true.
func DisableAutomountMutation(version string, disableAutomount bool) serviceaccount.Mutation {
	return serviceaccount.Mutation{
		Name:    "disable-automount",
		Feature: feature.NewResourceFeature(version, nil).When(disableAutomount),
		Mutate: func(m *serviceaccount.Mutator) error {
			v := false
			m.SetAutomountServiceAccountToken(&v)
			return nil
		},
	}
}
