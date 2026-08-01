package hpa

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes HorizontalPodAutoscaler
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Operational: for reporting operational status based on HPA conditions.
//   - concepts.Graceful: for reporting health after the allowed grace period has expired.
//   - concepts.Suspendable: for default delete-on-suspend behaviour that removes the HPA to
//     prevent it from scaling the target back up during suspension.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
type Resource struct {
	base *generic.IntegrationResource[*autoscalingv2.HorizontalPodAutoscaler, *Mutator]
}

// Identity returns a unique identifier for the HPA in the format
// "autoscaling/v2/HorizontalPodAutoscaler/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes HorizontalPodAutoscaler object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes HorizontalPodAutoscaler into the desired state.
//
// All registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus reports the HPA's operational status using the configured handler.
//
// By default, it uses DefaultOperationalStatusHandler, which inspects HPA conditions
// to determine if the autoscaler is active, pending, or failing.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus reports the HPA's health after the allowed grace period has expired.
//
// By default, it uses DefaultGraceStatusHandler, which inspects HPA conditions
// to determine if the autoscaler is healthy, degraded, or down.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the HPA should be deleted from the cluster
// when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns true. The HPA is
// deleted to prevent the Kubernetes HPA controller from scaling the target back up
// while it is suspended. On resume the framework recreates the HPA with the desired spec.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend registers the configured suspension mutation for the next mutate cycle.
//
// For HPA, the default suspension mutation is a no-op since the HPA is deleted on
// suspend (no spec mutations are needed before deletion).
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus reports the suspension status of the HPA.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports Suspended
// immediately because deletion is handled by the framework after this status is reported.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled HPA.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the HPA.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// ProducedData returns the cells this HPA declares extractions into.
// It satisfies concepts.DataProducer for component topology validation and
// introspection.
func (r *Resource) ProducedData() []concepts.DataCell {
	return r.base.ProducedData()
}

// ConsumedData returns the HPA's declared data reads. It satisfies
// concepts.DataConsumer for component topology validation and introspection.
func (r *Resource) ConsumedData() []concepts.DataConsumption {
	return r.base.ConsumedData()
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

// Preview renders the HorizontalPodAutoscaler as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the HorizontalPodAutoscaler, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the HorizontalPodAutoscaler was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
var _ concepts.DataProducer = (*Resource)(nil)
var _ concepts.DataConsumer = (*Resource)(nil)
