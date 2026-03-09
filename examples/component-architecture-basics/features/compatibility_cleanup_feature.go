// Package features contains example feature definitions for the component architecture.
package features

import (
	"github.com/sourcehawk/operator-component-framework/examples/component-architecture-basics/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

var legacyBehaviorFeature = MustRegister("LegacyBehaviorExample", "< 8.0.0")

// NewLegacyBehaviorFeature creates a Feature Mutation for older versions (< 8.0.0).
// It demonstrates how Feature Mutations are used to apply legacy behavior for backwards compatibility,
// such as adding deprecated configuration or reverting modern defaults to legacy values.
func NewLegacyBehaviorFeature(version string) feature.Mutation[*resources.DeploymentResourceMutator] {
	return feature.Mutation[*resources.DeploymentResourceMutator]{
		Name: "LegacyBehavior",
		Feature: feature.NewResourceFeature(
			version, []feature.VersionConstraint{legacyBehaviorFeature},
		),
		Mutate: func(m *resources.DeploymentResourceMutator) error {
			// Set a deprecated env var
			m.EnsureContainerEnvVar("DEPRECATED_SETTING", "legacy-value")
			// Remove the new one as it's not supported in legacy versions
			m.RemoveContainerEnvVar("NEW_MANDATORY_SETTING")
			return nil
		},
	}
}
