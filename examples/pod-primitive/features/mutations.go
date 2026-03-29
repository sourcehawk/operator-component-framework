// Package features provides sample mutations for the pod primitive example.
//
// These examples only mutate object metadata because most Pod spec fields
// are immutable after creation. Kubernetes does allow a small set of in-place
// updates (notably container images), but other fields such as env, args, and
// resources require the Pod to be deleted and recreated.
package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"
)

// TracingFeature marks the pod as tracing-enabled via metadata.
// Controllers or sidecar injectors can watch the label and handle
// any required Pod replacement or injection.
func TracingFeature(enabled bool) pod.Mutation {
	return pod.Mutation{
		Name:    "Tracing",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *pod.Mutator) error {
			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureLabel("sidecar.jaegertracing.io/inject", "true")
				return nil
			})
			return nil
		},
	}
}

// VersionFeature records the desired version on the pod as a label.
// It avoids mutating container images directly; while Kubernetes allows
// in-place image updates, this example keeps mutations in metadata only.
func VersionFeature(version string) pod.Mutation {
	return pod.Mutation{
		Name:    "Version",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *pod.Mutator) error {
			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}
