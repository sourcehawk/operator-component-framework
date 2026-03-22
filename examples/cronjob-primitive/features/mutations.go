package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/cronjob"
	corev1 "k8s.io/api/core/v1"
)

// TracingFeature adds tracing environment variables to all containers.
func TracingFeature(enabled bool) cronjob.Mutation {
	return cronjob.Mutation{
		Name:    "Tracing",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *cronjob.Mutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{
				Name:  "JAEGER_AGENT_HOST",
				Value: "localhost",
			})
			m.EnsureContainerEnvVar(corev1.EnvVar{
				Name:  "TRACING_ENABLED",
				Value: "true",
			})
			return nil
		},
	}
}

// MetricsFeature adds metrics annotations to the pod template.
func MetricsFeature(enabled bool) cronjob.Mutation {
	return cronjob.Mutation{
		Name:    "Metrics",
		Feature: feature.NewResourceFeature("any", nil).When(enabled),
		Mutate: func(m *cronjob.Mutator) error {
			m.EditPodTemplateMetadata(func(meta *editors.ObjectMetaEditor) error {
				meta.EnsureAnnotation("prometheus.io/scrape", "true")
				meta.EnsureAnnotation("prometheus.io/port", "9090")
				return nil
			})
			return nil
		},
	}
}

// VersionFeature sets the image version and a label.
func VersionFeature(version string) cronjob.Mutation {
	return cronjob.Mutation{
		Name:    "Version",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *cronjob.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("worker"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Image = fmt.Sprintf("my-worker:%s", version)
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
