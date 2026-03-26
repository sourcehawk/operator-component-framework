package generic

import (
	"errors"
	"reflect"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// BaseBuilder provides shared behavior for all generic internal resource builders.
type BaseBuilder[T client.Object, M FeatureMutator] struct {
	BaseRes      *BaseResource[T, M]
	clusterScope bool
}

// InitBase initializes the base resource configuration.
//
// Safe defaults are configured for suspension handlers so that custom resource
// wrappers built on the generic layer do not need to set them explicitly:
//   - SuspendMutationHandler: no-op (does nothing on suspend)
//   - SuspendStatusHandler: reports Suspended immediately
//
// DeleteOnSuspend defaults to false via a nil-check in BaseResource.DeleteOnSuspend.
func (b *BaseBuilder[T, M]) InitBase(
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) {
	b.BaseRes = &BaseResource[T, M]{
		DesiredObject: obj,
		IdentityFunc:  identityFunc,
		NewMutator:    newMutator,
		SuspendMutationHandler: func(_ M) error {
			return nil
		},
		SuspendStatusHandler: func(_ T) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "default suspension status",
			}, nil
		},
	}
}

// WithMutation registers a typed feature mutation for the resource.
func (b *BaseBuilder[T, M]) WithMutation(m Mutation[M]) {
	b.BaseRes.Mutations = append(b.BaseRes.Mutations, m)
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

// MarkClusterScoped marks the resource as cluster-scoped. ValidateBase will
// reject a non-empty namespace instead of requiring one.
func (b *BaseBuilder[T, M]) MarkClusterScoped() {
	b.clusterScope = true
}

// ValidateBase validates the base resource configuration.
func (b *BaseBuilder[T, M]) ValidateBase() error {
	if isNil(b.BaseRes.DesiredObject) {
		return errors.New("object cannot be nil")
	}

	if b.BaseRes.DesiredObject.GetName() == "" {
		return errors.New("object name cannot be empty")
	}

	if b.clusterScope {
		if b.BaseRes.DesiredObject.GetNamespace() != "" {
			return errors.New("cluster-scoped object must not have a namespace")
		}
	} else {
		if b.BaseRes.DesiredObject.GetNamespace() == "" {
			return errors.New("object namespace cannot be empty")
		}
	}

	if b.BaseRes.IdentityFunc == nil {
		return errors.New("identity function cannot be nil")
	}

	if b.BaseRes.NewMutator == nil {
		return errors.New("mutator factory cannot be nil")
	}

	return nil
}
