package component

import "github.com/sourcehawk/operator-component-framework/pkg/feature"

// ResourceOptionsBuilder constructs a ResourceOptions value, optionally
// integrating with the feature gating system to control whether a resource
// is created or deleted based on feature state.
//
// When a feature is set and evaluates to disabled, the resource is marked
// for deletion regardless of other settings (ReadOnly, ParticipationMode).
// Additional boolean conditions added via WithTruth follow the same
// semantics: if any condition is false, the resource is deleted.
type ResourceOptionsBuilder struct {
	feature feature.Gate
	truths  []bool

	readOnly          bool
	participationMode ParticipationMode
}

// NewResourceOptionsBuilder creates a new ResourceOptionsBuilder with default
// settings. Without any modifiers, Build returns a ResourceOptions equivalent
// to the zero value (create, required, read-write).
func NewResourceOptionsBuilder() *ResourceOptionsBuilder {
	return &ResourceOptionsBuilder{}
}

// WithFeatureGate sets a feature.Gate that gates the resource's existence.
//
// When the feature evaluates to disabled, the resulting ResourceOptions will
// have Delete set to true, causing the component to remove the resource from
// the cluster. When enabled, the resource is created normally.
//
// A nil feature is treated as unconditionally enabled (no gating).
func (b *ResourceOptionsBuilder) WithFeatureGate(f feature.Gate) *ResourceOptionsBuilder {
	b.feature = f
	return b
}

// WithTruth adds a boolean condition that must be true for the resource
// to be created. If the condition is false, the resource is marked for
// deletion, following the same semantics as a disabled feature.
//
// Calls are additive: all values passed through WithTruth must be true
// for the resource to be created. Conditions are evaluated with AND logic
// alongside any configured feature.
func (b *ResourceOptionsBuilder) WithTruth(truth bool) *ResourceOptionsBuilder {
	b.truths = append(b.truths, truth)
	return b
}

// Auxiliary sets the participation mode to Auxiliary, meaning the resource
// does not affect the component's health aggregation.
//
// This is equivalent to setting ParticipationMode to ParticipationModeAuxiliary.
func (b *ResourceOptionsBuilder) Auxiliary() *ResourceOptionsBuilder {
	b.participationMode = ParticipationModeAuxiliary
	return b
}

// ReadOnly marks the resource as read-only. The component will fetch the
// resource's current state but will not create or update it.
//
// If the resource is also gated by a disabled feature or a false truth
// condition, deletion takes precedence over read-only mode.
func (b *ResourceOptionsBuilder) ReadOnly() *ResourceOptionsBuilder {
	b.readOnly = true
	return b
}

// Build evaluates the configured feature and truth conditions and returns
// the resulting ResourceOptions.
//
// Feature evaluation can fail (e.g., version constraint parsing errors),
// in which case Build returns the error. If no feature is set and no truths
// are configured, Build always succeeds.
//
// Resolution rules:
//   - If the feature is non-nil and Enabled() returns false, Delete is true.
//   - If any truth condition is false, Delete is true.
//   - If Delete is true, ReadOnly is forced to false (deletion takes precedence).
//   - ParticipationMode is preserved regardless of deletion state.
func (b *ResourceOptionsBuilder) Build() (ResourceOptions, error) {
	shouldDelete := false

	if b.feature != nil {
		enabled, err := b.feature.Enabled()
		if err != nil {
			return ResourceOptions{}, err
		}
		if !enabled {
			shouldDelete = true
		}
	}

	if !shouldDelete {
		for _, t := range b.truths {
			if !t {
				shouldDelete = true
				break
			}
		}
	}

	return ResourceOptions{
		Delete:            shouldDelete,
		ReadOnly:          b.readOnly && !shouldDelete,
		ParticipationMode: b.participationMode,
	}, nil
}

// ResourceOptionsFor is a convenience function that creates ResourceOptions
// gated by a single feature.Gate.
//
// When the feature is enabled, the resource is created with default options
// (required participation, read-write). When disabled, the resource is
// marked for deletion.
//
// A nil feature is treated as unconditionally enabled.
//
// This is equivalent to:
//
//	NewResourceOptionsBuilder().WithFeatureGate(f).Build()
func ResourceOptionsFor(f feature.Gate) (ResourceOptions, error) {
	return NewResourceOptionsBuilder().WithFeatureGate(f).Build()
}
