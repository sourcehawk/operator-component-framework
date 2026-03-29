// Package features provides sample mutations for the pv primitive example.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"
	corev1 "k8s.io/api/core/v1"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the PersistentVolume.
// It is always enabled.
func VersionLabelMutation(version string) pv.Mutation {
	return pv.Mutation{
		Name:    "version-label",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *pv.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// RetainPolicyMutation sets the reclaim policy to Retain when enabled.
// This is gated by a boolean condition.
func RetainPolicyMutation(version string, enabled bool) pv.Mutation {
	return pv.Mutation{
		Name:    "retain-policy",
		Feature: feature.NewVersionGate(version, nil).When(enabled),
		Mutate: func(m *pv.Mutator) error {
			m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
			return nil
		},
	}
}

// MountOptionsMutation adds NFS mount options when enabled.
// This is gated by a boolean condition.
func MountOptionsMutation(version string, enabled bool) pv.Mutation {
	return pv.Mutation{
		Name:    "mount-options",
		Feature: feature.NewVersionGate(version, nil).When(enabled),
		Mutate: func(m *pv.Mutator) error {
			m.SetMountOptions([]string{"hard", "nfsvers=4.1"})
			return nil
		},
	}
}

// ExampleOperationalStatus demonstrates the default operational status handler
// by returning the status for a given PV phase.
func ExampleOperationalStatus(phase corev1.PersistentVolumePhase) (concepts.OperationalStatusWithReason, error) {
	p := &corev1.PersistentVolume{
		Status: corev1.PersistentVolumeStatus{Phase: phase},
	}
	return pv.DefaultOperationalStatusHandler(concepts.ConvergingOperationNone, p)
}
