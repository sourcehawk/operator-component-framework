package features

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

var _ feature.VersionConstraint = (*ExampleVersionConstraint)(nil)

// ExampleVersionConstraint is a simple implementation of feature.VersionConstraint
// for the component-architecture example.
// It uses semantic versioning to determine if a feature is enabled.
type ExampleVersionConstraint struct {
	name        string
	constraints *semver.Constraints
}

// NewExampleVersionConstraint creates a new ExampleVersionConstraint from a semver constraint string.
func NewExampleVersionConstraint(name, constraint string) (*ExampleVersionConstraint, error) {
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, fmt.Errorf("invalid semver constraint '%s' for feature '%s': %w", constraint, name, err)
	}

	return &ExampleVersionConstraint{
		name:        name,
		constraints: c,
	}, nil
}

// MustRegister is a convenience function that panics if the constraint is invalid.
func MustRegister(name, constraint string) *ExampleVersionConstraint {
	c, err := NewExampleVersionConstraint(name, constraint)
	if err != nil {
		panic(err)
	}
	return c
}

// Enabled reports whether the feature is enabled for the given version string.
func (c *ExampleVersionConstraint) Enabled(version string) (bool, error) {
	// Simple normalization: if version is empty or just a placeholder, we might handle it.
	// For this example, we assume valid semver.
	v, err := semver.NewVersion(strings.TrimPrefix(version, "v"))
	if err != nil {
		return false, fmt.Errorf("invalid version '%s': %w", version, err)
	}

	return c.constraints.Check(v), nil
}
