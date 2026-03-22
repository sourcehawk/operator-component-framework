package clusterrolebinding

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired, preserving
// roleRef on updates.
//
// roleRef is immutable after creation in the Kubernetes RBAC API. When the current
// object has a ResourceVersion (indicating it already exists in the cluster), the
// applicator restores the original roleRef after copying.
func DefaultFieldApplicator(current, desired *rbacv1.ClusterRoleBinding) error {
	roleRef := current.RoleRef
	resourceVersion := current.ResourceVersion
	*current = *desired.DeepCopy()
	if resourceVersion != "" {
		current.RoleRef = roleRef
	}
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes ClusterRoleBinding
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// ClusterRoleBinding resources are static: they do not model convergence health,
// grace periods, or suspension. Use a workload or task primitive for resources
// that require those concepts.
type Resource struct {
	base *generic.StaticResource[*rbacv1.ClusterRoleBinding, *Mutator]
}

// Identity returns a unique identifier for the ClusterRoleBinding in the format
// "rbac.authorization.k8s.io/v1/ClusterRoleBinding/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes ClusterRoleBinding object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes ClusterRoleBinding into the
// desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired base state,
//     using either DefaultFieldApplicator or a custom applicator if one is configured.
//  2. Field application flavors: any registered flavors are applied in registration order.
//  3. Feature mutations: all registered feature-gated mutations are applied in order.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled ClusterRoleBinding.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the ClusterRoleBinding.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
