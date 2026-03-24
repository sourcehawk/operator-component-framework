package concepts

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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

// StaleGenerationStatus checks whether a resource's controller has observed the latest spec by
// comparing ObservedGeneration against the object's Generation. If the controller is behind,
// it returns a non-nil AliveStatusWithReason with an appropriate Creating or Updating status.
// If the generation is current, it returns nil.
//
// This should be called at the top of a DefaultConvergingStatusHandler before evaluating
// readiness fields, which may be stale when the controller has not yet reconciled the
// latest generation.
//
//	if status := concepts.StaleGenerationStatus(op, obj.Status.ObservedGeneration, obj.Generation, "deployment"); status != nil {
//	    return *status, nil
//	}
func StaleGenerationStatus(
	op ConvergingOperation, observedGeneration, generation int64, resourceKind string,
) *AliveStatusWithReason {
	if observedGeneration >= generation {
		return nil
	}

	var status AliveConvergingStatus
	switch op {
	case ConvergingOperationCreated:
		status = AliveConvergingStatusCreating
	default:
		status = AliveConvergingStatusUpdating
	}

	return &AliveStatusWithReason{
		Status: status,
		Reason: fmt.Sprintf("Waiting for %s controller to observe latest spec", resourceKind),
	}
}
