package concepts

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ConvergingOperation classifies what the framework's apply did to a resource in
// the current reconcile. It provides context to the lifecycle interfaces (Alive,
// Completable, Operational) to help determine the converging status, and it
// selects the Created/Updated event recorded for the resource.
//
// The framework derives it by comparing the object observed before the
// Server-Side Apply (read through the client, usually the informer cache) with
// the API server's response to it, on the fields the component's field manager
// owns before or after the apply. The classification therefore reflects whether
// the applied desired state changed the object, not how the desired object was
// constructed: operators that rebuild their desired objects on every reconcile
// and operators that keep them across reconciles get the same result. A write
// by another field manager, such as a finalizer or label another controller
// adds between the read and the apply, is not a change.
//
// The owned fields come from the object's managedFields. When the read returns
// none, as with a cache that strips them, the framework compares the whole
// object apart from the status subresource, managedFields, resourceVersion and
// generation, and a write by another field manager in that window does count
// as a change.
type ConvergingOperation string

const (
	// ConvergingOperationCreated indicates that the resource did not exist before
	// the apply and was created by it.
	ConvergingOperationCreated ConvergingOperation = "Created"
	// ConvergingOperationUpdated indicates that the resource existed and the apply
	// changed it: a field the component's field manager owns, such as a label,
	// an annotation, an owner reference, or a field of the spec or data, was set
	// to a new value, added, or removed.
	ConvergingOperationUpdated ConvergingOperation = "Updated"
	// ConvergingOperationNone indicates that the apply left the existing resource
	// unchanged, or that the resource was only read (read-only resources).
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
