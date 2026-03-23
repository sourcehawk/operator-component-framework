package networkpolicy

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired while
// preserving server-managed metadata (ResourceVersion, UID, Generation, etc.)
// and shared-controller fields (OwnerReferences, Finalizers) from the original
// current object.
func DefaultFieldApplicator(current, desired *networkingv1.NetworkPolicy) error {
	original := current.DeepCopy()
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes NetworkPolicy
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//
// NetworkPolicy resources are static: they do not model convergence health, grace
// periods, or suspension. Use a workload or task primitive for resources that
// require those concepts.
type Resource struct {
	base *generic.StaticResource[*networkingv1.NetworkPolicy, *Mutator]
}

// Identity returns a unique identifier for the NetworkPolicy in the format
// "networking.k8s.io/v1/NetworkPolicy/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes NetworkPolicy object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes NetworkPolicy into the
// desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired base
//     state, using either DefaultFieldApplicator or a custom applicator if one is
//     configured.
//  2. Field application flavors: any registered flavors are applied in registration
//     order.
//  3. Feature mutations: all registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled NetworkPolicy.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the NetworkPolicy.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
