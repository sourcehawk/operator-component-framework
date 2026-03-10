package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MutatorApplier is implemented by workload mutators that can apply their planned changes
// to the underlying Kubernetes object.
type MutatorApplier interface {
	Apply() error
}

// FeatureMutator is implemented by workload mutators that support defining feature boundaries.
type FeatureMutator interface {
	MutatorApplier
	BeginFeature()
}

// WorkloadResource is a generic internal resource implementation for long-running Kubernetes
// workload objects such as Deployments, StatefulSets, and DaemonSets.
//
// It provides shared behavior for:
//   - baseline field application
//   - field application flavors
//   - feature mutations
//   - suspension mutations
//   - data extraction
//
// Concrete workload packages are expected to wrap this type and provide kind-specific
// identity and status logic.
type WorkloadResource[T client.Object, M MutatorApplier] struct {
	Object T

	IdentityFunc func(T) string

	DefaultFieldApplicator FieldApplicator[T]
	CustomFieldApplicator  FieldApplicator[T]
	FieldFlavors           []FieldApplicationFlavor[T]

	DataExtractors []func(T) error

	NewMutator func(T) M
	Mutations  []feature.Mutation[M]

	Suspender func(M) error

	ConvergingStatusHandler func(component.ConvergingOperation, T) (component.ConvergingStatusWithReason, error)
	GraceStatusHandler      func(T) (component.GraceStatusWithReason, error)
	SuspendStatusHandler    func(T) (component.SuspensionStatusWithReason, error)
	SuspendMutationHandler  func(M) error
	DeleteOnSuspendHandler  func(T) bool
}

// Identity returns the stable framework identity for the workload.
func (r *WorkloadResource[T, M]) Identity() string {
	return r.IdentityFunc(r.Object)
}

// Object returns a deep copy of the desired workload object.
func (r *WorkloadResource[T, M]) GetObject() (client.Object, error) {
	return r.Object.DeepCopyObject().(client.Object), nil
}

// Mutate applies the baseline field applicator, field application flavors, feature mutations,
// and any active suspension mutation to the provided current object.
func (r *WorkloadResource[T, M]) Mutate(current client.Object) error {
	currentTyped, ok := current.(T)
	if !ok {
		return fmt.Errorf("expected %T, got %T", r.Object, current)
	}

	applied, err := r.ApplyBaselineAndFlavors(currentTyped)
	if err != nil {
		return err
	}

	mutator := r.NewMutator(applied)
	fm, isFeatureMutator := any(mutator).(FeatureMutator)

	for _, mutation := range r.Mutations {
		if isFeatureMutator {
			fm.BeginFeature()
		}

		if err := mutation.ApplyIntent(mutator); err != nil {
			return fmt.Errorf("failed to apply mutation intent for %s: %w", mutation.Name, err)
		}
	}

	if err := mutator.Apply(); err != nil {
		return fmt.Errorf("failed to apply planned mutations: %w", err)
	}

	if r.Suspender != nil {
		if isFeatureMutator {
			fm.BeginFeature()
		}

		if err := r.Suspender(mutator); err != nil {
			return err
		}

		if err := mutator.Apply(); err != nil {
			return fmt.Errorf("failed to apply suspension mutations: %w", err)
		}
	}

	r.Object = applied

	return nil
}

// ApplyBaselineAndFlavors runs the standard field application pipeline on the provided current object.
func (r *WorkloadResource[T, M]) ApplyBaselineAndFlavors(current T) (T, error) {
	return applyBaselineAndFlavors(
		current,
		r.Object,
		r.DefaultFieldApplicator,
		r.CustomFieldApplicator,
		r.FieldFlavors,
	)
}

// ExtractData runs all registered data extractors against a deep copy of the reconciled object.
func (r *WorkloadResource[T, M]) ExtractData() error {
	copyObj, ok := r.Object.DeepCopyObject().(T)
	if !ok {
		return fmt.Errorf("failed to deep copy object of type %T", r.Object)
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

// ConvergingStatus reports the workload's convergence status using the configured handler.
func (r *WorkloadResource[T, M]) ConvergingStatus(
	op component.ConvergingOperation,
) (component.ConvergingStatusWithReason, error) {
	if r.ConvergingStatusHandler == nil {
		return component.ConvergingStatusWithReason{}, fmt.Errorf("converging status handler is not configured")
	}
	return r.ConvergingStatusHandler(op, r.Object)
}

// GraceStatus reports the workload's grace status using the configured handler.
func (r *WorkloadResource[T, M]) GraceStatus() (component.GraceStatusWithReason, error) {
	if r.GraceStatusHandler == nil {
		return component.GraceStatusWithReason{}, fmt.Errorf("grace status handler is not configured")
	}
	return r.GraceStatusHandler(r.Object)
}

// DeleteOnSuspend reports whether the workload should be deleted when suspended.
func (r *WorkloadResource[T, M]) DeleteOnSuspend() bool {
	if r.DeleteOnSuspendHandler == nil {
		return false
	}
	return r.DeleteOnSuspendHandler(r.Object)
}

// Suspend registers the configured suspension mutation for the next mutate cycle.
func (r *WorkloadResource[T, M]) Suspend() error {
	if r.SuspendMutationHandler == nil {
		return fmt.Errorf("suspend mutation handler is not configured")
	}

	r.Suspender = func(m M) error {
		defer func() { r.Suspender = nil }()
		return r.SuspendMutationHandler(m)
	}

	return nil
}

// SuspensionStatus reports the workload's suspension status using the configured handler.
func (r *WorkloadResource[T, M]) SuspensionStatus() (component.SuspensionStatusWithReason, error) {
	if r.SuspendStatusHandler == nil {
		return component.SuspensionStatusWithReason{}, fmt.Errorf("suspend status handler is not configured")
	}
	return r.SuspendStatusHandler(r.Object)
}
