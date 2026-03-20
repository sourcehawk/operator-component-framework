package features

import (
	"github.com/sourcehawk/operator-component-framework/examples/custom-resource-implementation/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

var legacyFeature = MustRegister("LegacyExample", "< 9.0.0")

// NewLegacyCompatibilityFeature creates a Feature Mutation enabled for versions < 9.0.0.
// This demonstrates the use of Feature Mutations for backward compatibility,
// adding legacy flags and removing modern settings using the restricted mutator.
func NewLegacyCompatibilityFeature(version string) feature.Mutation[*resources.DeploymentResourceMutator] {
	return feature.Mutation[*resources.DeploymentResourceMutator]{
		Name: "LegacyCompatibilityFeature",
		Feature: feature.NewResourceFeature(
			version, []feature.VersionConstraint{legacyFeature},
		),
		Mutate: func(m *resources.DeploymentResourceMutator) error {
			// Add legacy flag
			m.EnsureContainerArg("--legacy-mode")

			// For older versions, we might need to remove a modern environment variable
			// that causes issues in legacy mode.
			m.RemoveContainerEnvVar("MODERN_MODE_ENABLED")

			return nil
		},
	}
}
