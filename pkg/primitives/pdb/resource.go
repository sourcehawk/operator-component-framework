// Package pdb provides a builder and resource for managing Kubernetes PodDisruptionBudgets.
package pdb

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	policyv1 "k8s.io/api/policy/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes PodDisruptionBudget
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
//
// PodDisruptionBudget resources are static: they do not model convergence health,
// grace periods, or suspension. Use a workload or task primitive for resources
// that require those concepts.
type Resource struct {
	base *generic.StaticResource[*policyv1.PodDisruptionBudget, *Mutator]
}

// Identity returns a unique identifier for the PodDisruptionBudget in the format
// "policy/v1/PodDisruptionBudget/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes PodDisruptionBudget object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes PodDisruptionBudget into the desired state.
//
// The mutation process follows this order:
//  1. The desired base state is applied to the current object.
//  2. Feature mutations: all registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled PodDisruptionBudget.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the PodDisruptionBudget.
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

// PreviewObject returns the PodDisruptionBudget as it would appear after feature mutations
// have been applied, without modifying the resource's internal state.
//
// Suspension mutations are not applied; the preview reflects content state only.
func (r *Resource) PreviewObject() (*policyv1.PodDisruptionBudget, error) {
	return r.base.PreviewObject()
}
