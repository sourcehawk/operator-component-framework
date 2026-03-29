// Package features provides sample features for the job primitive.
package features

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/job"
	corev1 "k8s.io/api/core/v1"
)

// TracingFeature adds tracing environment variables to the job containers.
func TracingFeature(enabled bool) job.Mutation {
	return job.Mutation{
		Name:    "Tracing",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *job.Mutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{
				Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
				Value: "http://otel-collector:4317",
			})
			m.EnsureContainerEnvVar(corev1.EnvVar{
				Name:  "OTEL_TRACES_SAMPLER",
				Value: "always_on",
			})

			return nil
		},
	}
}

// RetryPolicyFeature configures the job's retry behavior.
func RetryPolicyFeature(version string) job.Mutation {
	return job.Mutation{
		Name:    "RetryPolicy",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *job.Mutator) error {
			m.EditJobSpec(func(e *editors.JobSpecEditor) error {
				e.SetBackoffLimit(3)
				e.SetActiveDeadlineSeconds(600)
				return nil
			})

			return nil
		},
	}
}

// VersionFeature sets the image version and a label.
func VersionFeature(version string) job.Mutation {
	return job.Mutation{
		Name:    "Version",
		Feature: feature.NewVersionGate(version, nil),
		Mutate: func(m *job.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("migrate"), func(ce *editors.ContainerEditor) error {
				ce.Raw().Image = fmt.Sprintf("my-app-migration:%s", version)
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
