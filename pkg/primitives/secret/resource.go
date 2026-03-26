package secret

import (
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes Secret within
// a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - concepts.DataExtractable: for exporting values after successful reconciliation.
//
// Secret resources are static: they do not model convergence health, grace periods,
// or suspension. Use a workload or task primitive for resources that require those concepts.
type Resource struct {
	base *generic.StaticResource[*corev1.Secret, *Mutator]
}

// Identity returns a unique identifier for the Secret in the format
// "v1/Secret/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes Secret object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes Secret into the desired state.
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
// of the reconciled Secret.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the Secret.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
