// Package integration provides an unstructured resource primitive for Kubernetes
// integration objects that depend on external assignments and require operational
// status tracking and suspension support.
package integration

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing an unstructured Kubernetes
// integration object within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for operational status tracking.
//   - component.Graceful: for health assessment during grace periods.
//   - component.Suspendable: for graceful deactivation.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// The operational status handler is required. All other handlers default to
// safe no-ops when omitted.
type Resource struct {
	base *generic.IntegrationResource[*uns.Unstructured, *unstruct.Mutator]
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

// ConvergingStatus reports the resource's operational status.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the resource when it has not yet
// reached full readiness after the grace period expires.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
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
