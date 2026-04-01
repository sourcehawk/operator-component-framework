package features

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	corev1 "k8s.io/api/core/v1"
)

// DebugLoggingMutation sets LOG_LEVEL=debug on the application container when
// enabled. It targets [selectors.ContainerNamed] with the baseline name "app",
// so it must be registered before any backward compat mutation that renames
// the container. The edit carries through the rename because backward compat
// mutations only overwrite specific fields (Name, Ports), not the environment.
func DebugLoggingMutation(enabled bool) deployment.Mutation {
	return deployment.Mutation{
		Name:    "DebugLogging",
		Feature: feature.NewVersionGate("any", nil).When(enabled),
		Mutate: func(m *deployment.Mutator) error {
			m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
				ce.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
				return nil
			})
			return nil
		},
	}
}
