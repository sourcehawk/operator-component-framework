// Package pv provides a builder and resource for managing Kubernetes PersistentVolumes.
package pv

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a PersistentVolume Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// operational status handlers, and data extractors. Build() validates the
// configuration and returns an initialized Resource ready for use in a
// reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*corev1.PersistentVolume, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PersistentVolume object.
//
// The PersistentVolume object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations and flavors.
//
// PersistentVolumes are cluster-scoped; the provided object must have a Name set
// but must not have a Namespace. This is validated during the Build() call.
func NewBuilder(pv *corev1.PersistentVolume) *Builder {
	identityFunc := func(p *corev1.PersistentVolume) string {
		return fmt.Sprintf("v1/PersistentVolume/%s", p.Name)
	}

	base := generic.NewIntegrationBuilder[*corev1.PersistentVolume, *Mutator](
		pv,
		identityFunc,
		DefaultFieldApplicator,
		NewMutator,
	)
	base.MarkClusterScoped()
	base.WithCustomOperationalStatus(DefaultOperationalStatusHandler)

	return &Builder{base: base}
}

// WithMutation registers a mutation for the PersistentVolume.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation,
// after the baseline field applicator and any registered flavors have run.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing PersistentVolume in the cluster.
//
// The default applicator (DefaultFieldApplicator) preserves immutable fields
// (volume source, volume mode, claim ref) on existing PVs while replacing all
// other fields from the desired state. Use a custom applicator when you need
// different preservation semantics.
//
// The applicator receives the current object from the API server and the desired
// object from the Resource, and is responsible for merging the desired changes
// into the current object.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *corev1.PersistentVolume) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
//
// Flavors run after the baseline applicator (default or custom) in registration
// order. They are typically used to preserve fields from the live cluster object
// that should not be overwritten by the desired state.
//
// A nil flavor is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*corev1.PersistentVolume](flavor))
	return b
}

// WithCustomOperationalStatus overrides the default logic for determining if the
// PersistentVolume is operationally ready.
//
// The default behavior uses DefaultOperationalStatusHandler, which considers a PV
// operational when its phase is Available or Bound. Use this method if your PV
// requires more complex readiness checks.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *corev1.PersistentVolume) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithDataExtractor registers a function to read values from the PersistentVolume
// after it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled PersistentVolume. This is
// useful for surfacing generated or updated fields to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(corev1.PersistentVolume) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(pv *corev1.PersistentVolume) error {
			return extractor(*pv)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No PersistentVolume object was provided.
//   - The PersistentVolume is missing a Name.
//   - The PersistentVolume has a Namespace set (PVs are cluster-scoped).
//   - Identity function, field applicator, or mutator factory is nil.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}

	return &Resource{base: genericRes}, nil
}
