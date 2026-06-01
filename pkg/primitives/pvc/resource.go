package pvc

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes PersistentVolumeClaim
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Operational: for tracking whether the PVC is bound and operational.
//   - concepts.Graceful: for health assessment after the component's grace period expires.
//   - concepts.Suspendable: for controlled suspension (e.g. retaining the PVC while suspending consumers).
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
//
// PVC resources follow the Integration lifecycle: they are operationally significant
// (a PVC must be Bound to be useful) and support suspension semantics.
type Resource struct {
	base *generic.IntegrationResource[*corev1.PersistentVolumeClaim, *Mutator]
}

// Identity returns a unique identifier for the PVC in the format
// "v1/PersistentVolumeClaim/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes PersistentVolumeClaim object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes PersistentVolumeClaim into the
// desired state.
//
// The mutation process follows this order:
//  1. The desired base state is applied to the current object.
//  2. Feature mutations: all registered feature-gated mutations are applied in order.
//  3. Suspension: if the resource is in a suspending state, the suspension logic is applied.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates whether the PVC has reached its desired operational state.
//
// By default, it uses DefaultOperationalStatusHandler, which checks the PVC's phase
// to determine if it is Bound (operational), Pending, or Lost (failing).
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus reports the health of the PVC after the component's grace period
// has expired.
//
// By default, it uses DefaultGraceStatusHandler, which considers a Bound PVC
// healthy, a Lost PVC down, and all other phases degraded.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the PVC should be deleted from the cluster
// when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false to preserve
// the PVC and its underlying storage during suspension.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the PVC.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behaviour uses DefaultSuspendMutationHandler, which is a no-op
// since PVCs do not require modification when suspended — the consuming workload
// is responsible for scaling down.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which always reports
// Suspended since PVCs themselves have no runtime state to wind down.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled PVC.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the PVC.
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

// Preview renders the PersistentVolumeClaim as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the PersistentVolumeClaim, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the PersistentVolumeClaim was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
