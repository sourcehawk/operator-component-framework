package pvc

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a PVC Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// status handlers, and data extractors. Build() validates the configuration and
// returns an initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*corev1.PersistentVolumeClaim, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PersistentVolumeClaim object.
//
// The PVC object serves as the desired base state. During reconciliation the Resource
// will make the cluster's state match this base, modified by any registered mutations
// and flavors.
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
		DefaultFieldApplicator,
		NewMutator,
	)

	base.
		WithCustomOperationalStatus(DefaultOperationalStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a mutation for the PVC.
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
// state to the existing PVC in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object while preserving immutable fields on
// existing PVCs. Use a custom applicator when you need different merge semantics.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *corev1.PersistentVolumeClaim) error,
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
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*corev1.PersistentVolumeClaim](flavor))
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

// WithDataExtractor registers a function to read values from the PVC after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled PVC. This is useful
// for surfacing the bound volume name, capacity, or other status fields to
// other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(corev1.PersistentVolumeClaim) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(p *corev1.PersistentVolumeClaim) error {
			return extractor(*p)
		})
	}
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
