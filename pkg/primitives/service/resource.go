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
// shared-controller fields (OwnerReferences, Finalizers), the immutable
// spec.clusterIP and spec.clusterIPs fields that Kubernetes assigns after creation,
// the server-populated Status (including LoadBalancer.Ingress), and auto-allocated
// NodePort values for NodePort and LoadBalancer Services. NodePorts are only
// preserved when the resulting Service type is NodePort or LoadBalancer and the
// desired port's NodePort is 0 (unset); explicitly specified NodePort values in
// the desired object are always applied.
func DefaultFieldApplicator(current, desired *corev1.Service) error {
	original := current.DeepCopy()
	clusterIP := current.Spec.ClusterIP
	clusterIPs := current.Spec.ClusterIPs
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	generic.PreserveStatus(current, original)
	if clusterIP != "" {
		current.Spec.ClusterIP = clusterIP
		current.Spec.ClusterIPs = clusterIPs
	}
	// Only preserve nodePorts when the resulting Service type supports them.
	effectiveType := current.Spec.Type
	if effectiveType == "" {
		effectiveType = corev1.ServiceTypeClusterIP
	}
	if effectiveType == corev1.ServiceTypeNodePort || effectiveType == corev1.ServiceTypeLoadBalancer {
		preserveNodePorts(current, original)
	}
	return nil
}

// preserveNodePorts restores auto-allocated nodePort values from the original
// object when the desired port's NodePort is 0. Ports are matched by Name when
// both ports have a non-empty Name; otherwise they are matched by Port+Protocol
// (treating empty protocol as TCP).
func preserveNodePorts(current, original *corev1.Service) {
	if len(original.Spec.Ports) == 0 {
		return
	}

	for i := range current.Spec.Ports {
		if current.Spec.Ports[i].NodePort != 0 {
			continue // explicitly set, don't override
		}
		for _, orig := range original.Spec.Ports {
			if orig.NodePort == 0 {
				continue
			}
			if matchPort(current.Spec.Ports[i], orig) {
				current.Spec.Ports[i].NodePort = orig.NodePort
				break
			}
		}
	}
}

func matchPort(a, b corev1.ServicePort) bool {
	if a.Name != "" && b.Name != "" {
		return a.Name == b.Name
	}
	return a.Port == b.Port && normalizeProtocol(a.Protocol) == normalizeProtocol(b.Protocol)
}

func normalizeProtocol(p corev1.Protocol) corev1.Protocol {
	if p == "" {
		return corev1.ProtocolTCP
	}
	return p
}

// Resource is a high-level abstraction for managing a Kubernetes Service within
// a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for tracking whether the Service is operational.
//   - component.Suspendable: for participating in the component suspension lifecycle.
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
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false — the
// Service is left in place during suspension.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the suspension of the Service.
//
// The default handler is a no-op — the Service is left unaffected by suspension.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus reports the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which always reports
// Suspended because the default behaviour leaves the Service untouched.
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
