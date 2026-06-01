package pv

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes PersistentVolume
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Operational: for tracking whether the PV is operationally ready.
//   - concepts.Graceful: for assessing health after the grace period expires.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*corev1.PersistentVolume, *Mutator]
}

// Identity returns a unique identifier for the PersistentVolume in the format
// "v1/PersistentVolume/<name>".
//
// PersistentVolumes are cluster-scoped and do not include a namespace in their identity.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes PersistentVolume object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes PersistentVolume into the desired state.
//
// The mutation process follows this order:
//  1. The desired base state is applied to the current object.
//  2. Feature mutations: all registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the PersistentVolume is operationally ready.
//
// By default, it uses DefaultOperationalStatusHandler, which considers a PV
// operational when its phase is Available or Bound.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus reports the health of the PersistentVolume after the component's
// grace period has expired.
//
// By default, it uses DefaultGraceStatusHandler, which considers a PV healthy
// when its phase is Available or Bound, degraded when Pending, and down when
// Released or Failed.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled PersistentVolume.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the PV.
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

// Preview renders the PersistentVolume as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the PersistentVolume, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the PersistentVolume was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
