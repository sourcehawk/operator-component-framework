package rolebinding

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces the current RoleBinding with a deep copy of
// the desired object, preserving server-managed metadata (ResourceVersion, UID,
// Generation, etc.) and shared-controller fields (OwnerReferences, Finalizers)
// from the original current object. It also preserves the live roleRef when the
// resource already exists in the cluster, since roleRef is immutable after
// creation in Kubernetes.
func DefaultFieldApplicator(current, desired *rbacv1.RoleBinding) error {
	original := current.DeepCopy()
	roleRef := current.RoleRef
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	if original.ResourceVersion != "" {
		current.RoleRef = roleRef
	}
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes RoleBinding
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// RoleBinding resources are static: they do not model convergence health,
// grace periods, or suspension.
type Resource struct {
	base *generic.StaticResource[*rbacv1.RoleBinding, *Mutator]
}

// Identity returns a unique identifier for the RoleBinding in the format
// "rbac.authorization.k8s.io/v1/RoleBinding/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes RoleBinding object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes RoleBinding into the
// desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired
//     base state, using either DefaultFieldApplicator or a custom applicator
//     if one is configured. roleRef is preserved from the live object.
//  2. Field application flavors: any registered flavors are applied in
//     registration order.
//  3. Feature mutations: all registered feature-gated mutations are applied
//     in order.
//
// This method is invoked by the framework during the Update phase of
// reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ExtractData executes all registered data extractor functions against a deep
// copy of the reconciled RoleBinding.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the RoleBinding.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
