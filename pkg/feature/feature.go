// Package feature provides gating mechanisms for conditional mutations and resource lifecycle control.
package feature

import "fmt"

// Gate is an optional gate for a Mutation or resource.
// If Enabled returns false, the associated mutation is not applied,
// or the associated resource is marked for deletion.
type Gate interface {
	Enabled() (bool, error)
}

// Mutation defines a conditional mutation applied to an object of type T.
//
// If Feature is nil the mutation is applied unconditionally on every
// reconciliation. If Feature is non-nil the mutation is applied only when
// Feature.Enabled() returns true.
type Mutation[T any] struct {
	// Name is a human-readable identifier used for error reporting.
	Name string
	// Feature gates this mutation. If nil, the mutation is applied unconditionally.
	Feature Gate
	// Mutate is the function that applies the changes to the object.
	Mutate func(T) error
}

// ApplyIntent applies the mutation to obj.
//
// If Feature is nil the mutation is applied unconditionally.
// If Feature is non-nil and disabled, ApplyIntent returns nil without
// performing any action.
// If the mutation would be applied but Mutate is nil, it returns an error.
func (m *Mutation[T]) ApplyIntent(obj T) error {
	if m.Feature != nil {
		enabled, err := m.Feature.Enabled()
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
	}

	if m.Mutate == nil {
		return fmt.Errorf("mutation handler of %s is nil", m.Name)
	}

	return m.Mutate(obj)
}

// VersionConstraint defines a condition based on a semantic version.
// Implementations should report whether the constraint is satisfied for the given version string.
type VersionConstraint interface {
	// Enabled reports whether the constraint is satisfied for the given version string.
	Enabled(version string) (bool, error)
}

// VersionGate is a Gate implementation that combines semantic version constraints
// with boolean conditions.
//
// A VersionGate is enabled only when all registered semver constraints match
// the current version and all additional truth conditions added via When are true.
type VersionGate struct {
	current            string
	versionConstraints []VersionConstraint

	// requiredTruths contains additional boolean conditions that must all be true
	// for the gate to be enabled.
	requiredTruths []bool
}

// NewVersionGate creates a new VersionGate for the given current version
// and semver constraints.
//
// Nil constraints are ignored.
func NewVersionGate(currentVersion string, versionConstraints []VersionConstraint) *VersionGate {
	var constraints []VersionConstraint

	for _, constraint := range versionConstraints {
		if constraint != nil {
			constraints = append(constraints, constraint)
		}
	}

	return &VersionGate{
		current:            currentVersion,
		versionConstraints: constraints,
	}
}

// NewBooleanGate creates a VersionGate that is enabled only when enabled is true.
//
// It is shorthand for NewVersionGate("", nil).When(enabled): a gate with no
// version constraints whose result is driven solely by the boolean. Use it for a
// mutation or resource toggled by a spec flag rather than an application version.
// Because it returns a *VersionGate, further conditions can still be composed with
// When.
func NewBooleanGate(enabled bool) *VersionGate {
	return NewVersionGate("", nil).When(enabled)
}

// When adds a boolean condition that must be true for the gate to be enabled.
//
// Calls are additive: all values passed through When must be true for Enabled()
// to return true.
func (v *VersionGate) When(truth bool) *VersionGate {
	v.requiredTruths = append(v.requiredTruths, truth)
	return v
}

// Enabled reports whether the gate is enabled.
//
// The gate is enabled only if:
//   - all When conditions are true
//   - all version constraints match the current version.
func (v *VersionGate) Enabled() (bool, error) {
	for _, truth := range v.requiredTruths {
		if !truth {
			return false, nil
		}
	}

	for _, constraint := range v.versionConstraints {
		enabled, err := constraint.Enabled(v.current)
		if err != nil {
			return false, err
		}
		if !enabled {
			return false, nil
		}
	}

	return true, nil
}
