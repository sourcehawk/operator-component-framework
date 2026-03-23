package component

import (
	"context"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/scope"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/recording"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createOrUpdateResources ensures that all registered "creation" resources exist and match
// the desired state in the Kubernetes cluster.
//
// Reconciliation Strategy:
//  1. Sequential Execution: Resources are processed one by one in the order they were registered.
//  2. Fail-Fast: Processing stops at the first error. This is intentional, as resources
//     often have implicit dependencies (e.g., a Deployment depending on a ConfigMap).
//  3. Status Collection: For each resource that implements the Alive interface, its
//     converging status is collected after the CreateOrUpdate operation.
//
// CreateOrUpdate behavior:
//   - If the object doesn't exist, MutateFn will be called, and it will be created.
//   - If the object exists, CreateOrUpdate first populates the object with the server state,
//     then calls MutateFn.
//   - If the object did not exist, the object remains the one returned by Object(),
//     typically just with name/namespace/type metadata set.
//
// Returns a slice of convergingResults for all processed Alive resources, or an error if
// any operation fails.
func createOrUpdateResources(
	ctx context.Context, rec ReconcileContext, resources []Resource, mapper meta.RESTMapper,
) ([]convergingResult, error) {
	var results []convergingResult

	for _, resource := range resources {
		// Create or update resources
		obj, err := resource.Object()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve object for resource %s: %w", resource.Identity(), err,
			)
		}

		op, err := ctrl.CreateOrUpdate(ctx, rec.Client, obj, func() error {
			return mutateResource(ctx, resource, obj, rec.Owner, rec.Scheme, mapper)
		})
		// We return immediately on errors because resource may depend on each other and creating them regardless of
		// previous errors may result in the real problem being hidden.
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create or update resource %s: %w", resource.Identity(), err,
			)
		}

		convergingOperation := concepts.ConvergingOperationFromOperationResult(op)

		// Gather converging status of resources
		status, err := getConvergingStatus(resource, convergingOperation)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to determine converging status of resource %s: %w", resource.Identity(), err,
			)
		}
		if status != nil {
			results = append(results, convergingResult{Resource: resource, Status: *status})
		}

		recording.RecordCreateOrUpdateOperationEvent(rec.Recorder, op, obj, rec.Owner)
	}

	return results, nil
}

// mutateResource is the controllerutil.MutateFn used by CreateOrUpdate.
// It orchestrates the application of desired state to the Kubernetes object:
//  1. Mutations: Calls Mutate() to apply all fields to the object.
//     The `obj` passed here is the same object instance that `ctrl.CreateOrUpdate` works with.
//     If the object exists, it is pre-populated with server state; if not, it is the original
//     object from `resource.Object()`.
//  2. Ownership: Sets a controller reference pointing to the owner CRD, unless the
//     owned resource is cluster-scoped and the owner is namespace-scoped (which Kubernetes
//     does not allow). In that case the reference is skipped and a warning is logged.
func mutateResource(
	ctx context.Context, resource Resource, obj client.Object, owner client.Object,
	scheme *runtime.Scheme, mapper meta.RESTMapper,
) error {
	if err := resource.Mutate(obj); err != nil {
		return err
	}

	canSet, err := scope.CanSetOwnerReference(owner, obj, scheme, mapper)
	if err != nil {
		return fmt.Errorf(
			"failed to determine owner reference eligibility for resource %s: %w",
			resource.Identity(), err,
		)
	}

	if !canSet {
		log.FromContext(ctx).Info(
			"skipping owner reference for cluster-scoped resource owned by namespace-scoped owner; "+
				"this resource will not be garbage-collected when the owner is deleted",
			"resource", resource.Identity(),
		)
		return nil
	}

	return ctrl.SetControllerReference(owner, obj, scheme)
}
