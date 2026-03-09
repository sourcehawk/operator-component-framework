package component

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// readResources fetches the latest state of all "read-only" resources from the cluster.
// These resources are not modified by the component but their health (if they implement Alive)
// is factored into the overall component condition.
//
// Like createOrUpdateResources, this function is sequential and fails fast on the first error.
// ConvergingOperationNone is passed to Alive.ConvergingStatus, as no mutations were performed.
func readResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
) ([]convergingResult, error) {
	var results []convergingResult

	for _, resource := range resources {
		// Get readonly resources
		object, err := resource.Object()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve read-only object from resource %s: %w",
				resource.Identity(), err,
			)
		}

		// Applies the object to the resource
		if err := rec.Client.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve read-only resource %s from the k8s api: %w", resource.Identity(), err,
			)
		}

		// Gather converging status of alive readonly resources
		if alive, ok := resource.(Alive); ok {
			status, err := alive.ConvergingStatus(ConvergingOperationNone)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to get converging status of read-only resource %s: %w",
					resource.Identity(), err,
				)
			}

			results = append(results, convergingResult{Resource: resource, Status: status})
		}
	}

	return results, nil
}
