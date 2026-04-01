// Package features provides example mutations for the mutations-and-gating example.
package features

import (
	"github.com/Masterminds/semver/v3"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

// semverConstraint implements feature.VersionConstraint using Masterminds/semver.
type semverConstraint struct {
	c *semver.Constraints
}

// MustConstraint parses a semver constraint expression or panics.
func MustConstraint(expr string) feature.VersionConstraint {
	c, err := semver.NewConstraint(expr)
	if err != nil {
		panic(err)
	}
	return &semverConstraint{c: c}
}

// Enabled reports whether the constraint is satisfied for the given version.
func (s *semverConstraint) Enabled(version string) (bool, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, err
	}
	return s.c.Check(v), nil
}
