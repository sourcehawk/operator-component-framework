package component

import (
	"context"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/scope"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/recording"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// applyResource applies a single resource to the cluster using Server-Side Apply and
// collects its converging status. It returns the converging result (nil for static
// resources with no status interface) and any error encountered.
//
// This is the shared core used by both applyResources and applyResourcesWithGuards
// to avoid duplicating the Object/Mutate/SSA/status-collection sequence.
func applyResource(
	ctx context.Context, rec ReconcileContext, resource Resource,
	fieldOwner client.FieldOwner, mapper meta.RESTMapper,
) (*convergingResult, error) {
	obj, err := resource.Object()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve object for resource %s: %w", resource.Identity(), err,
		)
	}

	// Check if the object already exists (reads from informer cache, not the API server).
	var objectExists bool
	existing := obj.DeepCopyObject().(client.Object)
	if err := rec.Client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err == nil {
		objectExists = true
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf(
			"failed to check existence of resource %s: %w", resource.Identity(), err,
		)
	}

	// Snapshot the object before mutations for operation detection.
	// Comparing pre-Mutate vs post-Mutate detects whether the operator's desired
	// state actually changed, without being affected by status subresource updates
	// from other controllers (Mutate does not touch status).
	preMutate := obj.DeepCopyObject()

	// Apply mutations to desired state
	ownerRefSkipped, err := mutateResource(resource, obj, rec.Owner, rec.Scheme, mapper)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to mutate resource %s: %w", resource.Identity(), err,
		)
	}

	// Determine the converging operation before clearing server fields.
	var convergingOperation concepts.ConvergingOperation
	switch {
	case !objectExists:
		convergingOperation = concepts.ConvergingOperationCreated
	case !equality.Semantic.DeepEqual(preMutate, obj):
		convergingOperation = concepts.ConvergingOperationUpdated
	default:
		convergingOperation = concepts.ConvergingOperationNone
	}

	// Prepare the object for SSA by clearing server-populated fields that must not
	// be present in the patch. After the first reconcile, DesiredObject retains the
	// server response (including these fields) because Mutate updates the internal
	// pointer and Patch writes back into the same object.
	clearServerFields(obj)

	// Set GVK on the object (required for SSA — builders often omit TypeMeta)
	if err := ensureGVK(obj, rec.Scheme); err != nil {
		return nil, fmt.Errorf(
			"failed to determine GVK for resource %s: %w", resource.Identity(), err,
		)
	}

	// Server-Side Apply with forced ownership.
	// client.Apply is deprecated in favor of client.Client.Apply() which requires generated
	// ApplyConfiguration types. Using Patch with Apply is the pragmatic approach for untyped objects.
	if err := rec.Client.Patch(ctx, obj, client.Apply, client.ForceOwnership, fieldOwner); err != nil { //nolint:staticcheck
		return nil, fmt.Errorf(
			"failed to apply resource %s: %w", resource.Identity(), err,
		)
	}

	if ownerRefSkipped && convergingOperation != concepts.ConvergingOperationNone {
		log.FromContext(ctx).Info(
			"skipping owner reference for cluster-scoped resource owned by namespace-scoped owner; "+
				"this resource will not be garbage-collected when the owner is deleted",
			"resource", resource.Identity(),
		)
	}

	// Gather converging status of resources
	status, err := getConvergingStatus(resource, convergingOperation)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to determine converging status of resource %s: %w", resource.Identity(), err,
		)
	}

	recording.RecordApplyOperationEvent(rec.Recorder, convergingOperation, obj, rec.Owner)

	if status != nil {
		result := convergingResult{Resource: resource, Status: *status}
		return &result, nil
	}
	return nil, nil
}

// applyResources ensures that all registered "creation" resources exist and match
// the desired state in the Kubernetes cluster using Server-Side Apply.
//
// Reconciliation Strategy:
//  1. Sequential Execution: Resources are processed one by one in the order they were registered.
//  2. Fail-Fast: Processing stops at the first error. This is intentional, as resources
//     often have implicit dependencies (e.g., a Deployment depending on a ConfigMap).
//  3. Status Collection: For each resource that implements a lifecycle concept interface,
//     its converging status is collected after the Apply operation.
//
// Server-Side Apply behavior:
//   - The resource's desired state is built via Object() + Mutate(), then patched into the
//     cluster with forced field ownership. Only operator-managed fields are sent; server-defaulted
//     fields (e.g., imagePullPolicy, strategy) are untouched. This prevents perpetual updates
//     that occur with CreateOrUpdate when the API server re-adds defaults every reconcile.
//   - Field ownership is derived from the owner's Kind and the component name
//     (e.g., "ExampleApp/web-interface"). Forced ownership means the framework takes control
//     of any conflicting fields from other managers for fields it explicitly declares.
func applyResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
	componentName string, mapper meta.RESTMapper,
) ([]convergingResult, error) {
	fieldOwner := client.FieldOwner(
		fmt.Sprintf("%s/%s", rec.Owner.GetKind(), componentName),
	)

	var results []convergingResult

	for _, resource := range resources {
		result, err := applyResource(ctx, rec, resource, fieldOwner, mapper)
		if err != nil {
			return nil, err
		}
		if result != nil {
			results = append(results, *result)
		}
	}

	return results, nil
}

// reconcileResources processes all resources in registration order. Each resource is
// either fetched (read-only) or applied (managed) depending on its mode. Guards are
// evaluated before each resource, and data extraction runs immediately after, so that
// extracted data from earlier resources is available to subsequent resources' guards
// and mutations regardless of whether the earlier resource was read-only or managed.
//
// When a guard returns Blocked, the resource is not processed and all subsequent
// resources are skipped. Guards are not evaluated during suspension (the caller uses
// applyResources for that path).
func reconcileResources(
	ctx context.Context, rec ReconcileContext, entries []reconcileEntry,
	componentName string, mapper meta.RESTMapper,
) ([]convergingResult, error) {
	fieldOwner := client.FieldOwner(
		fmt.Sprintf("%s/%s", rec.Owner.GetKind(), componentName),
	)

	var results []convergingResult

	for _, entry := range entries {
		resource := entry.Resource

		// Evaluate guard before processing
		if guardable, ok := resource.(concepts.Guardable); ok {
			guardResult, err := guardable.GuardStatus()
			if err != nil {
				return nil, fmt.Errorf(
					"failed to evaluate guard for resource %s: %w", resource.Identity(), err,
				)
			}
			if guardResult.Status == concepts.GuardStatusBlocked {
				results = append(results, convergingResult{
					Resource: resource,
					Status: convergingStatusWithReason{
						Status: convergingStatusGuardBlocked,
						Reason: guardResult.Reason,
					},
				})
				return results, nil
			}
		}

		// Process the resource based on its mode
		var result *convergingResult
		var err error
		if entry.ReadOnly {
			result, err = readResource(ctx, rec, resource)
		} else {
			result, err = applyResource(ctx, rec, resource, fieldOwner, mapper)
		}
		if err != nil {
			return nil, err
		}
		if result != nil {
			results = append(results, *result)
		}

		// Per-resource data extraction: run immediately after processing so that
		// extracted data is available to subsequent resources' guards and mutations.
		if err := extractResourceData([]Resource{resource}); err != nil {
			return nil, fmt.Errorf(
				"failed to extract data from resource %s: %w", resource.Identity(), err,
			)
		}
	}

	return results, nil
}

// mutateResource applies all desired-state mutations and sets the controller owner reference.
func mutateResource(
	resource Resource, obj client.Object, owner client.Object,
	scheme *runtime.Scheme, mapper meta.RESTMapper,
) (ownerRefSkipped bool, err error) {
	if err := resource.Mutate(obj); err != nil {
		return false, err
	}

	canSet, err := scope.CanSetOwnerReference(owner, obj, scheme, mapper)
	if err != nil {
		return false, fmt.Errorf(
			"failed to determine owner reference eligibility for resource %s: %w",
			resource.Identity(), err,
		)
	}

	if !canSet {
		return true, nil
	}

	return false, ctrl.SetControllerReference(owner, obj, scheme)
}

// clearServerFields removes metadata fields that the API server populates on
// responses but must not be present in an SSA patch object. After the first
// apply, the resource's internal DesiredObject retains these from the server
// response because Patch writes back into the same pointer.
func clearServerFields(obj client.Object) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetGeneration(0)
	obj.SetDeletionTimestamp(nil)
	obj.SetDeletionGracePeriodSeconds(nil)
	obj.SetSelfLink("")
}

// ensureGVK sets the GroupVersionKind on the object if it is not already set.
// SSA requires TypeMeta to be present on the patch object.
func ensureGVK(obj client.Object, scheme *runtime.Scheme) error {
	if obj.GetObjectKind().GroupVersionKind().Kind != "" {
		return nil
	}
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no GVK registered in scheme for type %T", obj)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}
