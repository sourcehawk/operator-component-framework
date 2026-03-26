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
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Completable: for completion status tracking.
//   - component.Suspendable: for graceful deactivation.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// No default handlers are provided. All status handlers must be explicitly
// registered via the Builder before calling Build().
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
