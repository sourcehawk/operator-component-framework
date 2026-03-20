package component

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a generic interface for handling Kubernetes resources within a Component.
// Implementations of this interface wrap a specific Kubernetes object and define
// how it participates in the component's lifecycle.
type Resource interface {
	// Mutate applies all applicable mutations on the resource retrieved from the kubernetes api.
	//
	// If the object exists: `current` is the object fetched from the API server.
	// If it does not exist: `current` is the same base object returned by Object() and passed into CreateOrUpdate,
	// not a fetched server object.
	//
	// The Resource implementation is responsible for applying all desired fields from its
	// internal core state to the current resource before proceeding with feature mutations.
	//
	// The mutations are applied every time the component is reconciled.
	Mutate(current client.Object) error
	// Object returns a copy of the managed Kubernetes resource.
	//
	// The returned object's state depends on the reconciliation phase:
	//   - Before reconciliation: Represents the baseline state before any mutations or side effects.
	//   - After reconciliation: Represents the state as applied to the Kubernetes API, including all mutations.
	//
	// This method is primarily intended for use by the Component reconciler. Implementers should
	// avoid calling this directly to retrieve data; instead, use provided patterns like DataExtractable.
	Object() (client.Object, error)
	// Identity returns a unique identifier for the resource in the format <apiVersion>/<kind>/<name>.
	Identity() string
}
