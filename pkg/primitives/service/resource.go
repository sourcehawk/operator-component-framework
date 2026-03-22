// Package service provides a builder and resource for managing Kubernetes Services.
package service

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired while
// preserving server-managed metadata (ResourceVersion, UID, Generation, etc.),
// shared-controller fields (OwnerReferences, Finalizers), and the immutable
// spec.clusterIP and spec.clusterIPs fields that Kubernetes assigns after creation.
func DefaultFieldApplicator(current, desired *corev1.Service) error {
	original := current.DeepCopy()
	clusterIP := current.Spec.ClusterIP
	clusterIPs := current.Spec.ClusterIPs
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	if clusterIP != "" {
		current.Spec.ClusterIP = clusterIP
		current.Spec.ClusterIPs = clusterIPs
	}
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes Service within
// a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for tracking whether the Service is operational.
//   - component.Suspendable: for controlled deletion when the component is suspended.
//   - component.DataExtractable: for exporting values after successful reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*corev1.Service, *Mutator]
}

// Identity returns a unique identifier for the Service in the format
// "v1/Service/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes Service object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes Service into the desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired base state,
//     using either DefaultFieldApplicator or a custom applicator if one is configured.
//     The default applicator preserves spec.clusterIP and spec.clusterIPs.
//  2. Field application flavors: any registered flavors are applied in registration order.
//  3. Feature mutations: all registered feature-gated mutations are applied in order.
//  4. Suspension: if the resource is in a suspending state, the suspension mutation is applied.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the operational status of the Service.
//
// By default, it uses DefaultOperationalStatusHandler, which considers LoadBalancer
// services pending until an ingress IP or hostname is assigned, and all other service
// types immediately operational.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the Service should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns true — Services
// are deleted on suspend because they have no meaningful "scaled down" state.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the suspension of the Service.
//
// The default handler is a no-op because Services are deleted on suspend
// rather than mutated.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus reports the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which always reports
// Suspended with a reason indicating the Service is deleted on suspend.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled Service.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values such as assigned ClusterIP or
// LoadBalancer ingress from the Service.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
