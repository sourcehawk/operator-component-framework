// Package features provides feature plan mutations for the replicaset-primitive example.
package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/replicaset"
	corev1 "k8s.io/api/core/v1"
)

// TracingFeature adds a Jaeger sidecar to the replicaset.
func TracingFeature(enabled bool) replicaset.Mutation {
	return replicaset.Mutation{
		Name:    "Tracing",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *replicaset.Mutator) error {
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

// MetricsFeature adds an exporter sidecar and some annotations.
func MetricsFeature(enabled bool, port int) replicaset.Mutation {
	return replicaset.Mutation{
		Name:    "Metrics",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *replicaset.Mutator) error {
			m.EnsureContainer(corev1.Container{
				Name:  "prometheus-exporter",
				Image: "prom/node-exporter:v1.3.1",
			})

			m.EditPodTemplateMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureAnnotation("prometheus.io/scrape", "true")
				meta.EnsureAnnotation("prometheus.io/port", fmt.Sprintf("%d", port))
				return nil
			})

			return nil
		},
	}
}

// VersionFeature sets the image version and a label.
func VersionFeature(version string) replicaset.Mutation {
	return replicaset.Mutation{
		Name:    "Version",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *replicaset.Mutator) error {
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
