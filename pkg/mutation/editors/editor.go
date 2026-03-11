package editors

// RawEditor provides access to the raw underlying Kubernetes object.
//
// This interface allows users to perform unconstrained modifications to the
// object when the provided typed helpers are insufficient.
type RawEditor[T any] interface {
	// Raw returns a pointer to the raw Kubernetes object.
	Raw() *T
}
