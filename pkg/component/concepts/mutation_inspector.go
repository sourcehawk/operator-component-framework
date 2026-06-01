package concepts

// MutationInspector surfaces, read-only, the mutations registered on a built
// resource or component and which of them fire at the version it was built at.
//
// All built-in primitives satisfy this through generic.BaseResource, and the
// component aggregates its managed resources. It is an inert capability: nothing
// in the reconcile path calls it, so importing it costs nothing at runtime.
type MutationInspector interface {
	// RegisteredMutations returns the Names of every mutation registered on the
	// unit, independent of the version it was built at. Names are unique within a
	// resource (the resource builder rejects a duplicate at build time), and the
	// returned list is deduplicated across a component's resources, so it is always
	// a set.
	RegisteredMutations() []string

	// FiringSet returns the Names of registered mutations whose gate is enabled
	// for the version the unit was built at. A mutation with a nil gate fires
	// unconditionally and is always included. It returns an error if any gate's
	// Enabled evaluation fails, since a swallowed gate error would silently
	// misclassify a version regime.
	FiringSet() ([]string, error)
}
