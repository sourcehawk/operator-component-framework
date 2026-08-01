// Package cronjob provides a builder and resource for managing Kubernetes CronJobs.
package cronjob

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes CronJob within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - concepts.Operational: for operational status tracking.
//   - concepts.Graceful: for health assessment after grace period expiry.
//   - concepts.Suspendable: for controlled suspension via spec.suspend.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting information after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to declared data extractions on read-only reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*batchv1.CronJob, *Mutator]
}

// Identity returns a unique identifier for the CronJob in the format
// "batch/v1/CronJob/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes CronJob object.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes CronJob into the desired state.
//
// The mutation process follows a specific order:
//  1. Feature Mutations: All registered feature-based mutations are applied.
//  2. Suspension: If the resource is in a suspending state, the suspension
//     logic (setting spec.suspend = true) is applied.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the CronJob's operational status.
//
// By default, it uses DefaultOperationalStatusHandler, which reports Pending
// when the CronJob has never been scheduled and Operational when it has
// scheduled at least once.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus reports the health of the CronJob after the component's grace period
// has expired.
//
// By default, it uses DefaultGraceStatusHandler, which always reports Healthy.
// A CronJob is a passive scheduler — once it exists and is not suspended, it is
// functioning correctly regardless of whether it has fired yet.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the CronJob should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it returns false — the CronJob is kept with spec.suspend set to true.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the suspension of the CronJob.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior sets spec.suspend to true.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports Suspended
// when spec.suspend is true and no active jobs are running.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled CronJob.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// ProducedData returns the cells this CronJob declares extractions into.
// It satisfies concepts.DataProducer for component topology validation and
// introspection.
func (r *Resource) ProducedData() []concepts.DataCell {
	return r.base.ProducedData()
}

// ConsumedData returns the CronJob's declared data reads. It satisfies
// concepts.DataConsumer for component topology validation and introspection.
func (r *Resource) ConsumedData() []concepts.DataConsumption {
	return r.base.ConsumedData()
}

// RecordObservation stores the supplied object as the resource's most recently
// observed cluster state. The framework invokes this on read-only resources
// after fetching them so that declared data extractions observe the live
// object rather than the inert base used to construct the resource.
func (r *Resource) RecordObservation(observed client.Object) error {
	return r.base.RecordObservation(observed)
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard was registered, the resource is unconditionally unblocked.
func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
	return r.base.GuardStatus()
}

// Preview renders the CronJob as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the CronJob, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the CronJob was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
var _ concepts.DataProducer = (*Resource)(nil)
var _ concepts.DataConsumer = (*Resource)(nil)
