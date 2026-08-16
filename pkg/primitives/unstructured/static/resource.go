// Package static provides an unstructured resource primitive for static
// Kubernetes objects that do not model convergence health, grace periods,
// or suspension.
package static

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a static unstructured
// Kubernetes object within a controller's reconciliation loop.
//
// It implements the following interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to declared data extractions on read-only reconciliation.
//
// Static unstructured resources do not model convergence health, grace periods,
// or suspension. Use the workload, integration, or task unstructured variants
// for resources that require those concepts.
type Resource struct {
	base *generic.StaticResource[*uns.Unstructured, *unstruct.Mutator]
}

// Identity returns a unique identifier for the resource derived from its GVK,
// namespace, and name.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// MetricsIdentifier returns the identifier set with
// Builder.WithMetricsIdentifier, or an empty string when none was set, in which
// case the framework labels the resource with its lowercased kind. It satisfies
// concepts.MetricsIdentifiable.
func (r *Resource) MetricsIdentifier() string {
	return r.base.MetricsIdentifier()
}

// Object returns a deep copy of the underlying unstructured Kubernetes object.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of the unstructured object into the
// desired state by applying all registered feature mutations.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ExtractData executes all declared data extractions against a deep
// copy of the reconciled object.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// ProducedData returns the cells this unstructured object declares
// extractions into. It satisfies concepts.DataProducer for component
// topology validation and introspection.
func (r *Resource) ProducedData() []concepts.DataCell {
	return r.base.ProducedData()
}

// ConsumedData returns the unstructured object's declared data reads. It
// satisfies concepts.DataConsumer for component topology validation and
// introspection.
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

// Preview renders the object as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the unstructured object, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the unstructured object was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
var _ concepts.DataProducer = (*Resource)(nil)
var _ concepts.DataConsumer = (*Resource)(nil)
var _ concepts.MetricsIdentifiable = (*Resource)(nil)
