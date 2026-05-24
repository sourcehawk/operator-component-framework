package component

import (
	"context"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// readResource fetches a single read-only resource from the cluster and collects
// its converging status. It returns the converging result (nil for static resources
// with no status interface) and any error encountered.
//
// This is the read-only counterpart of applyResource, used by reconcileResources
// when processing read-only entries in the unified reconciliation loop.
func readResource(
	ctx context.Context, rec ReconcileContext, resource Resource,
) (*reconcileResult, error) {
	object, err := resource.Object()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve read-only object from resource %s: %w",
			resource.Identity(), err,
		)
	}

	if err := rec.Client.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve read-only resource %s from the k8s api: %w", resource.Identity(), err,
		)
	}

	if recorder, ok := resource.(concepts.ObservationRecorder); ok {
		if err := recorder.RecordObservation(object); err != nil {
			return nil, fmt.Errorf(
				"failed to record observation for resource %s: %w", resource.Identity(), err,
			)
		}
	}

	status, err := getConvergingStatus(resource, concepts.ConvergingOperationNone)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to determine converging status of resource %s: %w", resource.Identity(), err,
		)
	}

	if status != nil {
		return &reconcileResult{Status: *status}, nil
	}
	return nil, nil
}

// readResources fetches the latest state of all "read-only" resources from the cluster.
// These resources are not modified by the component but their health (if they implement Alive)
// is factored into the overall component condition.
//
// Like applyResources, this function is sequential and fails fast on the first error.
// ConvergingOperationNone is passed to Alive.ConvergingStatus, as no mutations were performed.
func readResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
) ([]reconcileResult, error) {
	var results []reconcileResult

	for _, resource := range resources {
		result, err := readResource(ctx, rec, resource)
		if err != nil {
			return nil, err
		}
		if result != nil {
			results = append(results, *result)
		}
	}

	return results, nil
}
