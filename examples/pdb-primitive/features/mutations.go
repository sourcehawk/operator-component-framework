// Package features provides sample mutations for the PDB primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pdb"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the PDB.
// It is always enabled.
func VersionLabelMutation(version string) pdb.Mutation {
	return pdb.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *pdb.Mutator) error {
			return m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
		},
	}
}

// StrictAvailabilityMutation switches the PDB from percentage-based MinAvailable
// to an absolute MaxUnavailable of 1 when metrics are enabled, indicating that
// the service requires stricter disruption control.
func StrictAvailabilityMutation(version string, metricsEnabled bool) pdb.Mutation {
	return pdb.Mutation{
		Name:    "strict-availability",
		Feature: feature.NewResourceFeature(version, nil).When(metricsEnabled),
		Mutate: func(m *pdb.Mutator) error {
			return m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
				e.ClearMinAvailable()
				e.SetMaxUnavailable(intstr.FromInt32(1))
				return nil
			})
		},
	}
}
