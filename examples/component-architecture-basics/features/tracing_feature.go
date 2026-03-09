package features

import (
	"github.com/sourcehawk/operator-component-framework/examples/component-architecture-basics/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

var tracingFeature = MustRegister("TracingExample", ">= 8.1.0")

// NewTracingFeature creates a Feature Mutation that is enabled for versions >= 8.1.0,
// and only if the boolean flag is true.
// It demonstrates how to use Additional boolean gating with the When() method on
// the ResourceFeature to combine semver with other feature toggles.
func NewTracingFeature(version string, enabled bool) feature.Mutation[*resources.DeploymentResourceMutator] {
	return feature.Mutation[*resources.DeploymentResourceMutator]{
		Name: "TracingFeature",
		Feature: feature.NewResourceFeature(
			version, []feature.VersionConstraint{tracingFeature},
		).When(enabled),
		Mutate: func(m *resources.DeploymentResourceMutator) error {
			m.EnsureContainerEnvVar("ENABLE_TRACING", "true")
			return nil
		},
	}
}
