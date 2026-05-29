// Package task provides an unstructured resource primitive for Kubernetes objects
// that run to completion, requiring completion status tracking and suspension support.
package task

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing an unstructured Kubernetes
// task object within a controller's reconciliation loop.
//
// It implements the following interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Completable: for completion status tracking.
//   - concepts.Suspendable: for graceful deactivation.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
//
// The converging status handler is required; all other handlers default to
// safe no-ops when omitted.
type Resource struct {
	base *generic.TaskResource[*uns.Unstructured, *unstruct.Mutator]
}

// Identity returns a unique identifier for the resource derived from its GVK,
// namespace, and name.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying unstructured Kubernetes object.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of the unstructured object into the
// desired state by applying all registered feature mutations and any active
// suspension mutation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the resource's completion status.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.CompletionStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the resource should be deleted from the
// cluster when the parent component is suspended.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the resource by registering a mutation
// that will be executed during the next Mutate call.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep
// copy of the reconciled object.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// RecordObservation stores the supplied object as the resource's most recently
// observed cluster state. The framework invokes this on read-only resources
// after fetching them so that registered data extractors observe the live
// object rather than the inert base used to construct the resource.
func (r *Resource) RecordObservation(observed client.Object) error {
	return r.base.RecordObservation(observed)
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard was registered, the resource is unconditionally unblocked.
func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
	return r.base.GuardStatus()
}

// PreviewObject returns the object as it would appear after feature mutations
// have been applied, without modifying the resource's internal state.
//
// Suspension mutations are not applied; the preview reflects content state only.
func (r *Resource) PreviewObject() (*uns.Unstructured, error) {
	return r.base.PreviewObject()
}

// Preview renders the object as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}
