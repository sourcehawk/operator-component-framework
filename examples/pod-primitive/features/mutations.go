// Package features provides sample mutations for the pod primitive example.
package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"
	corev1 "k8s.io/api/core/v1"
)

// TracingFeature adds a Jaeger sidecar to the pod.
func TracingFeature(enabled bool) pod.Mutation {
	return pod.Mutation{
		Name:    "Tracing",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *pod.Mutator) error {
			m.EnsureContainer(corev1.Container{
				Name:  "jaeger-agent",
				Image: "jaegertracing/jaeger-agent:1.28",
			})

			m.EnsureContainerEnvVar(corev1.EnvVar{
				Name:  "JAEGER_AGENT_HOST",
				Value: "localhost",
			})

			return nil
		},
	}
}

// VersionFeature sets the image version and a label.
func VersionFeature(version string) pod.Mutation {
	return pod.Mutation{
		Name:    "Version",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *pod.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Image = fmt.Sprintf("my-app:%s", version)
				return nil
			})

			m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})

			return nil
		},
	}
}
