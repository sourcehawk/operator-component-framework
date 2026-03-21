package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BaseBuilder provides shared behavior for all generic internal resource builders.
type BaseBuilder[T client.Object, M MutatorApplier] struct {
	BaseRes *BaseResource[T, M]
}

// InitBase initializes the base resource configuration.
func (b *BaseBuilder[T, M]) InitBase(
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
	newMutator func(T) M,
) {
	b.BaseRes = &BaseResource[T, M]{
		DesiredObject:          obj,
		IdentityFunc:           identityFunc,
		DefaultFieldApplicator: defaultApplicator,
		NewMutator:             newMutator,
	}
}

// WithMutation registers a typed feature mutation for the resource.
func (b *BaseBuilder[T, M]) WithMutation(m Mutation[M]) {
	b.BaseRes.Mutations = append(b.BaseRes.Mutations, m)
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *BaseBuilder[T, M]) WithCustomFieldApplicator(applicator FieldApplicator[T]) {
	b.BaseRes.CustomFieldApplicator = applicator
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
func (b *BaseBuilder[T, M]) WithFieldApplicationFlavor(flavor FieldApplicationFlavor[T]) {
	if flavor != nil {
		b.BaseRes.FieldFlavors = append(b.BaseRes.FieldFlavors, flavor)
	}
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *BaseBuilder[T, M]) WithDataExtractor(extractor func(T) error) {
	if extractor != nil {
		b.BaseRes.DataExtractors = append(b.BaseRes.DataExtractors, extractor)
	}
}

// WithCustomSuspendStatus overrides the resource suspension status handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) {
	b.BaseRes.SuspendStatusHandler = handler
}

// WithCustomSuspendMutation overrides the resource suspension mutation handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendMutation(handler func(M) error) {
	b.BaseRes.SuspendMutationHandler = handler
}

// WithCustomSuspendDeletionDecision overrides the resource delete-on-suspend decision handler.
func (b *BaseBuilder[T, M]) WithCustomSuspendDeletionDecision(handler func(T) bool) {
	b.BaseRes.DeleteOnSuspendHandler = handler
}

// ValidateBase validates the base resource configuration.
func (b *BaseBuilder[T, M]) ValidateBase() error {
	if isNil(b.BaseRes.DesiredObject) {
		return errors.New("object cannot be nil")
	}

	if b.BaseRes.DesiredObject.GetName() == "" {
		return errors.New("object name cannot be empty")
	}

	if b.BaseRes.DesiredObject.GetNamespace() == "" {
		return errors.New("object namespace cannot be empty")
	}

	if b.BaseRes.IdentityFunc == nil {
		return errors.New("identity function cannot be nil")
	}

	if b.BaseRes.DefaultFieldApplicator == nil {
		return errors.New("default field applicator cannot be nil")
	}

	if b.BaseRes.NewMutator == nil {
		return errors.New("mutator factory cannot be nil")
	}

	return nil
}
