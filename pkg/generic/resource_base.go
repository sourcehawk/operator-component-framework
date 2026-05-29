package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BaseResource provides shared behavior for all generic internal resource implementations.
type BaseResource[T client.Object, M FeatureMutator] struct {
	DesiredObject T

	IdentityFunc func(T) string

	DataExtractors []func(T) error

	NewMutator func(T) M
	Mutations  []Mutation[M]

	Suspender func(M) error

	GuardHandler func(T) (concepts.GuardStatusWithReason, error)

	SuspendStatusHandler   func(T) (concepts.SuspensionStatusWithReason, error)
	SuspendMutationHandler func(M) error
	DeleteOnSuspendHandler func(T) bool
}

// Identity returns the stable framework identity for the resource.
func (r *BaseResource[T, M]) Identity() string {
	return r.IdentityFunc(r.DesiredObject)
}

// Object returns a deep copy of the desired object.
func (r *BaseResource[T, M]) Object() (client.Object, error) {
	obj, ok := r.DesiredObject.DeepCopyObject().(client.Object)
	if !ok {
		return nil, fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	return obj, nil
}

// Mutate applies feature mutations and any active suspension mutation to the provided current object.
func (r *BaseResource[T, M]) Mutate(current client.Object) error {
	applied, err := ApplyMutations(
		current,
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

// PreviewObject returns the object as it would appear after feature mutations have been applied,
// without modifying the resource's internal DesiredObject.
//
// The desired object is used as a stand-in for the current cluster state.
//
// Suspension mutations are not applied; the preview reflects content state only.
func (r *BaseResource[T, M]) PreviewObject() (T, error) {
	currentCopy, ok := r.DesiredObject.DeepCopyObject().(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	return ApplyMutations(
		currentCopy,
		r.NewMutator,
		r.Mutations,
		nil,
	)
}

// Preview renders the desired object as a client.Object with feature mutations
// applied, without modifying the resource's internal state. It wraps
// PreviewObject so resources can be previewed polymorphically through the
// component's Previewable capability.
//
// Suspension mutations are not applied; the preview reflects content state only.
func (r *BaseResource[T, M]) Preview() (client.Object, error) {
	return r.PreviewObject()
}

// ExtractData runs all registered data extractors against a deep copy of the reconciled object.
//
// For managed resources the reconciled object is the desired state produced by Mutate.
// For read-only resources it is the object most recently supplied via RecordObservation,
// which the read flow invokes after fetching from the cluster.
func (r *BaseResource[T, M]) ExtractData() error {
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

// RecordObservation stores the supplied object as the resource's most recently observed
// cluster state. The framework invokes this on read-only resources immediately after
// fetching them, so that subsequent capabilities such as ExtractData observe the live
// object rather than the inert base used to construct the resource.
//
// RecordObservation returns an error when the supplied object is not assignable to the
// resource's underlying type.
func (r *BaseResource[T, M]) RecordObservation(observed client.Object) error {
	typed, ok := observed.(T)
	if !ok {
		return fmt.Errorf(
			"failed to record observation: expected object of type %T, got %T",
			r.DesiredObject, observed,
		)
	}
	r.DesiredObject = typed
	return nil
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard handler is configured, the resource is unconditionally unblocked.
// The handler receives a deep copy of the desired object to prevent accidental mutations.
func (r *BaseResource[T, M]) GuardStatus() (concepts.GuardStatusWithReason, error) {
	if r.GuardHandler == nil {
		return concepts.GuardStatusWithReason{
			Status: concepts.GuardStatusUnblocked,
		}, nil
	}

	copyObj, ok := r.DesiredObject.DeepCopyObject().(T)
	if !ok {
		return concepts.GuardStatusWithReason{}, fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	return r.GuardHandler(copyObj)
}

// DeleteOnSuspend reports whether the resource should be deleted when suspended.
func (r *BaseResource[T, M]) DeleteOnSuspend() bool {
	if r.DeleteOnSuspendHandler == nil {
		return false
	}
	return r.DeleteOnSuspendHandler(r.DesiredObject)
}

// Suspend registers the configured suspension mutation for the next mutate cycle.
func (r *BaseResource[T, M]) Suspend() error {
	if r.SuspendMutationHandler == nil {
		return fmt.Errorf("suspend mutation handler is not configured")
	}

	r.Suspender = func(m M) error {
		defer func() { r.Suspender = nil }()
		return r.SuspendMutationHandler(m)
	}

	return nil
}

// SuspensionStatus reports the resource's suspension status using the configured handler.
func (r *BaseResource[T, M]) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	if r.SuspendStatusHandler == nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf("suspend status handler is not configured")
	}
	return r.SuspendStatusHandler(r.DesiredObject)
}
