package pvc

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a PVC Resource.
//
// It provides a fluent API for registering mutations, status handlers, and data
// extractors. Build() validates the configuration and returns an initialized
// Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*corev1.PersistentVolumeClaim, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PersistentVolumeClaim object.
//
// The PVC object serves as the desired base state. During reconciliation the Resource
// will make the cluster's state match this base, modified by any registered mutations.
//
// The provided PVC must have both Name and Namespace set, which is validated during
// the Build() call.
func NewBuilder(pvc *corev1.PersistentVolumeClaim) *Builder {
	identityFunc := func(p *corev1.PersistentVolumeClaim) string {
		return fmt.Sprintf("v1/PersistentVolumeClaim/%s/%s", p.Namespace, p.Name)
	}

	base := generic.NewIntegrationBuilder[*corev1.PersistentVolumeClaim, *Mutator](
		pvc,
		identityFunc,
		NewMutator,
	)

	base.
		WithCustomOperationalStatus(DefaultOperationalStatusHandler).
		WithCustomGraceStatus(DefaultGraceStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers one or more mutations for the PVC.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(ms ...Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*Mutator](m))
	}
	return b
}

// WithCustomOperationalStatus overrides the default logic for determining if the
// PVC has reached its desired operational state.
//
// The default behavior uses DefaultOperationalStatusHandler, which checks whether
// the PVC is Bound. Use this method if you need additional checks, such as verifying
// specific annotations or conditions.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *corev1.PersistentVolumeClaim) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default logic for assessing the health of
// the PVC when the component's grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler, which considers a Bound
// PVC healthy, a Lost PVC down, and all other phases degraded.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*corev1.PersistentVolumeClaim) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which always reports
// Suspended since PVCs have no runtime state to wind down.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*corev1.PersistentVolumeClaim) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the PVC should be modified when the
// component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op.
// Override this if you need to add annotations or labels when suspended.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the PVC when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which does not delete
// PVCs during suspension to preserve data. Return true from this handler if you
// want the PVC to be completely removed from the cluster when suspended.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*corev1.PersistentVolumeClaim) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the PVC
// is applied during reconciliation. If the guard returns Blocked, the PVC and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(corev1.PersistentVolumeClaim) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the PVC reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the PVC reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithDataExtractor registers a function to read values from the PVC after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled PVC. This is useful
// for surfacing the bound volume name, capacity, or other status fields to
// other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(corev1.PersistentVolumeClaim) error) *Builder {
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No PVC object was provided.
//   - The PVC is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this PVC produces the value of cell. fn
// computes the value from a copy of the reconciled PVC; the framework
// stores it in the cell and marks it present, immediately after the PVC
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(corev1.PersistentVolumeClaim) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
