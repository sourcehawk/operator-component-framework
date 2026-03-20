package concepts

import "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

// ConvergingOperation represents the result of a CreateOrUpdate operation on a resource.
// It provides context to the Alive interface to help determine the ConvergingStatus.
type ConvergingOperation string

const (
	// ConvergingOperationCreated indicates that the resource was newly created.
	ConvergingOperationCreated ConvergingOperation = "Created"
	// ConvergingOperationUpdated indicates that an existing resource was updated.
	ConvergingOperationUpdated ConvergingOperation = "Updated"
	// ConvergingOperationNone indicates that no changes were made to the resource.
	ConvergingOperationNone ConvergingOperation = "None"
)

// ConvergingOperationFromOperationResult maps a controllerutil.OperationResult to a ConvergingStatus.
func ConvergingOperationFromOperationResult(result controllerutil.OperationResult) ConvergingOperation {
	switch result {
	case controllerutil.OperationResultCreated:
		return ConvergingOperationCreated
	case controllerutil.OperationResultUpdated:
		return ConvergingOperationUpdated
	}
	return ConvergingOperationNone
}
