// Package features provides sample features for the daemonset primitive.
package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/daemonset"
	corev1 "k8s.io/api/core/v1"
)

// TracingFeature adds a Jaeger sidecar to the daemonset.
func TracingFeature(enabled bool) daemonset.Mutation {
	return daemonset.Mutation{
		Name:    "Tracing",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *daemonset.Mutator) error {
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
func MetricsFeature(enabled bool, port int) daemonset.Mutation {
	return daemonset.Mutation{
		Name:    "Metrics",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *daemonset.Mutator) error {
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
func VersionFeature(version string) daemonset.Mutation {
	return daemonset.Mutation{
		Name:    "Version",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *daemonset.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("agent"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Image = fmt.Sprintf("my-agent:%s", version)
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
