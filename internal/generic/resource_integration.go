package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationResource is a generic internal resource implementation for Kubernetes
// integration objects such as Services, Ingresses, and Gateways.
//
// It provides shared behavior for:
//   - baseline field application
//   - field application flavors
//   - feature mutations
//   - data extraction
//
// Concrete integration packages are expected to wrap this type and provide kind-specific
// identity and status logic.
type IntegrationResource[T client.Object, M MutatorApplier] struct {
	DesiredObject T

	IdentityFunc func(T) string

	DefaultFieldApplicator FieldApplicator[T]
	CustomFieldApplicator  FieldApplicator[T]
	FieldFlavors           []FieldApplicationFlavor[T]

	DataExtractors []func(T) error

	NewMutator func(T) M
	Mutations  []feature.Mutation[M]

	Suspender func(M) error

	OperationalStatusHandler func(concepts.ConvergingOperation, T) (concepts.OperationalStatusWithReason, error)
	SuspendStatusHandler     func(T) (concepts.SuspensionStatusWithReason, error)
	SuspendMutationHandler   func(M) error
	DeleteOnSuspendHandler   func(T) bool
}

// Identity returns the stable framework identity for the integration.
func (r *IntegrationResource[T, M]) Identity() string {
	return r.IdentityFunc(r.DesiredObject)
}

// Object returns a deep copy of the desired integration object.
func (r *IntegrationResource[T, M]) Object() (client.Object, error) {
	return r.DesiredObject.DeepCopyObject().(client.Object), nil
}

// Mutate applies the baseline field applicator, field application flavors, and feature mutations
// to the provided current object.
func (r *IntegrationResource[T, M]) Mutate(current client.Object) error {
	applied, err := ApplyMutations(
		current,
		r.DesiredObject,
		r.DefaultFieldApplicator,
		r.CustomFieldApplicator,
		r.FieldFlavors,
		r.NewMutator,
		r.Mutations,
		r.Suspender,
	)
	if err != nil {
		return err
	}

	r.DesiredObject = applied

	return nil
}

// ApplyBaselineAndFlavors runs the standard field application pipeline on the provided current object.
func (r *IntegrationResource[T, M]) ApplyBaselineAndFlavors(current T) (T, error) {
	return applyBaselineAndFlavors(
		current,
		r.DesiredObject,
		r.DefaultFieldApplicator,
		r.CustomFieldApplicator,
		r.FieldFlavors,
	)
}

// ExtractData runs all registered data extractors against a deep copy of the reconciled object.
func (r *IntegrationResource[T, M]) ExtractData() error {
	copyObj, ok := r.DesiredObject.DeepCopyObject().(T)
	if !ok {
		return fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	for _, extractor := range r.DataExtractors {
		if extractor == nil {
			continue
		}
		if err := extractor(copyObj); err != nil {
			return err
		}
	}

	return nil
}

// ConvergingStatus reports the integration's operational status using the configured handler.
func (r *IntegrationResource[T, M]) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.OperationalStatusWithReason, error) {
	if r.OperationalStatusHandler == nil {
		return concepts.OperationalStatusWithReason{}, fmt.Errorf("operational status handler is not configured")
	}
	return r.OperationalStatusHandler(op, r.DesiredObject)
}

// DeleteOnSuspend reports whether the integration should be deleted when suspended.
func (r *IntegrationResource[T, M]) DeleteOnSuspend() bool {
	if r.DeleteOnSuspendHandler == nil {
		return false
	}
	return r.DeleteOnSuspendHandler(r.DesiredObject)
}

// Suspend registers the configured suspension mutation for the next mutate cycle.
func (r *IntegrationResource[T, M]) Suspend() error {
	if r.SuspendMutationHandler == nil {
		return fmt.Errorf("suspend mutation handler is not configured")
	}

	r.Suspender = func(m M) error {
		defer func() { r.Suspender = nil }()
		return r.SuspendMutationHandler(m)
	}

	return nil
}

// SuspensionStatus reports the integration's suspension status using the configured handler.
func (r *IntegrationResource[T, M]) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	if r.SuspendStatusHandler == nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf("suspend status handler is not configured")
	}
	return r.SuspendStatusHandler(r.DesiredObject)
}
