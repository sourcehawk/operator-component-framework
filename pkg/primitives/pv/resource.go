package pv

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes PersistentVolume
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for tracking whether the PV is operationally ready.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// PersistentVolume resources use the Integration lifecycle: they report an
// OperationalStatus rather than an Alive/Grace status, and do not support suspension.
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

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled PersistentVolume.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the PV.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
