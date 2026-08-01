// Package pv provides a builder and resource for managing Kubernetes PersistentVolumes.
package pv

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a PersistentVolume Resource.
//
// It provides a fluent API for registering mutations, operational status handlers,
// and data extractors. Build() validates the configuration and returns an
// initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*corev1.PersistentVolume, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PersistentVolume object.
//
// The PersistentVolume object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
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
		NewMutator,
	)
	base.MarkClusterScoped()
	base.WithCustomOperationalStatus(DefaultOperationalStatusHandler)
	base.WithCustomGraceStatus(DefaultGraceStatusHandler)

	return &Builder{base: base}
}

// WithMutation registers one or more mutations for the PersistentVolume.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation,
// after the baseline field applicator and any registered flavors have run.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(ms ...Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*Mutator](m))
	}
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

// WithCustomGraceStatus overrides the default logic for assessing the health of
// the PersistentVolume when the component's grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler, which considers a PV
// healthy when its phase is Available or Bound, degraded when Pending, and down
// when Released or Failed.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*corev1.PersistentVolume) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the PersistentVolume
// is applied during reconciliation. If the guard returns Blocked, the PersistentVolume and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(corev1.PersistentVolume) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the PersistentVolume reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the PersistentVolume reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
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
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No PersistentVolume object was provided.
//   - The PersistentVolume is missing a Name.
//   - The PersistentVolume has a Namespace set (PVs are cluster-scoped).
//   - Identity function or mutator factory is nil.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}

	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this PersistentVolume produces the value of cell. fn
// computes the value from a copy of the reconciled PersistentVolume; the framework
// stores it in the cell and marks it present, immediately after the PersistentVolume
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(corev1.PersistentVolume) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
