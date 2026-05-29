// Package service provides a builder and resource for managing Kubernetes Services.
package service

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
//   - concepts.Operational: for tracking whether the Service is operational.
//   - concepts.Graceful: for health assessment after the component's grace period expires.
//   - concepts.Suspendable: for participating in the component suspension lifecycle.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
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
//  1. Feature mutations: all registered feature-gated mutations are applied in order.
//  2. Suspension: if the resource is in a suspending state, the suspension mutation is applied.
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

// GraceStatus reports the health of the Service after the component's grace period
// has expired.
//
// By default, it uses DefaultGraceStatusHandler, which considers LoadBalancer services
// degraded until an ingress IP or hostname is assigned, and all other service types
// immediately healthy.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
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

// PreviewObject returns the Service as it would appear after feature mutations
// have been applied, without modifying the resource's internal state.
//
// Suspension mutations are not applied; the preview reflects content state only.
func (r *Resource) PreviewObject() (*corev1.Service, error) {
	return r.base.PreviewObject()
}

// Preview renders the Service as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}
