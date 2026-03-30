// Package ingress provides a builder and resource for managing Kubernetes Ingresses.
package ingress

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes Ingress within
// a controller's reconciliation loop.
//
// It implements the following component lifecycle interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Operational: for tracking whether the ingress has been assigned an address.
//   - concepts.Graceful: for health assessment after the component's grace period has expired.
//   - concepts.Suspendable: for controlled suspension when the parent component is suspended.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//
// Ingress resources are integration primitives: they depend on an external ingress
// controller to assign load balancer addresses. The default operational status handler
// reports OperationPending (concepts.OperationalStatusPending) until at least one IP or
// hostname is assigned, then Operational.
type Resource struct {
	base *generic.IntegrationResource[*networkingv1.Ingress, *Mutator]
}

// Identity returns a unique identifier for the Ingress in the format
// "networking.k8s.io/v1/Ingress/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes Ingress object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes Ingress into the desired state.
//
// The mutation process follows this order:
//  1. Feature mutations: all registered feature-gated mutations are applied in order.
//  2. Suspension mutation: if the component is suspended, the suspension mutation is applied.
//
// This method is invoked by the framework during the reconciliation loop.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates whether the Ingress has reached its operational state.
//
// By default, it uses DefaultOperationalStatusHandler, which reports OperationPending
// until the ingress controller has assigned at least one IP or hostname, then Operational.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the Ingress after the component's
// grace period has expired.
//
// By default, it uses DefaultGraceStatusHandler, which categorizes the current state into:
//   - GraceStatusHealthy: At least one IP or hostname is assigned in Status.LoadBalancer.Ingress.
//   - GraceStatusDegraded: No load balancer address has been assigned yet.
//
// This information is surfaced through the component's health reporting, allowing
// operators to understand the severity of a load balancer assignment delay.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the Ingress should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false. Deleting
// an Ingress causes the ingress controller to reload its configuration, affecting
// the entire cluster's routing — not just the suspended service.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the suspension of the Ingress.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior is a no-op — the Ingress is left in place and the
// backend service returning 502/503 is the expected observable behaviour.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which immediately reports
// Suspended with a reason indicating the backend is unavailable.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled Ingress.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values (such as assigned load balancer
// addresses) from the Ingress.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard was registered, the resource is unconditionally unblocked.
func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
	return r.base.GuardStatus()
}
