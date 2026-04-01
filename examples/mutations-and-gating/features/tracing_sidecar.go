package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	corev1 "k8s.io/api/core/v1"
)

// TracingSidecarMutation injects a Jaeger sidecar and sets JAEGER_AGENT_HOST on
// the application container. It is boolean-gated on the enableTracing flag.
func TracingSidecarMutation(enabled bool) deployment.Mutation {
	return deployment.Mutation{
		Name:    "Tracing",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *deployment.Mutator) error {
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
