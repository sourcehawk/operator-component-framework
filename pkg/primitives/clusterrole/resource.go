package clusterrole

import (
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes ClusterRole within
// a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// ClusterRole resources are static: they do not model convergence health, grace periods,
// or suspension. Use a workload or task primitive for resources that require those concepts.
//
// ClusterRole is cluster-scoped: it has no namespace.
type Resource struct {
	base *generic.StaticResource[*rbacv1.ClusterRole, *Mutator]
}

// Identity returns a unique identifier for the ClusterRole in the format
// "rbac.authorization.k8s.io/v1/ClusterRole/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes ClusterRole object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes ClusterRole into the desired state.
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
// of the reconciled ClusterRole.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the ClusterRole.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
