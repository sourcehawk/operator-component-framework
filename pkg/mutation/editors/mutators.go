package editors

// ObjectMutator is implemented by all primitive mutators.
//
// It guarantees that the Kubernetes object's own metadata is always mutable
// through a consistent API, regardless of which primitive type is being used.
type ObjectMutator interface {
	EditObjectMetadata(edit func(*ObjectMetaEditor) error)
}
