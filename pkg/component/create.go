package component

import (
	"context"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/scope"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/recording"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// applyResources ensures that all registered "creation" resources exist and match
// the desired state in the Kubernetes cluster using Server-Side Apply.
//
// Reconciliation Strategy:
//  1. Sequential Execution: Resources are processed one by one in the order they were registered.
//  2. Fail-Fast: Processing stops at the first error. This is intentional, as resources
//     often have implicit dependencies (e.g., a Deployment depending on a ConfigMap).
//  3. Status Collection: For each resource that implements the Alive interface, its
//     converging status is collected after the Apply operation.
//
// Server-Side Apply behavior:
//   - The resource's desired state is built via Object() + Mutate(), then patched into the
//     cluster with field ownership. Only operator-managed fields are sent; server-defaulted
//     fields (e.g., imagePullPolicy, strategy) are untouched. This prevents perpetual updates
//     that occur with CreateOrUpdate when the API server re-adds defaults every reconcile.
//   - Field ownership is derived from the owner's Kind and the component name
//     (e.g., "ExampleApp/web-interface").
func applyResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
	componentName string, mapper meta.RESTMapper,
) ([]convergingResult, error) {
	fieldOwner := client.FieldOwner(
		fmt.Sprintf("%s/%s", rec.Owner.GetKind(), componentName),
	)

	var results []convergingResult

	for _, resource := range resources {
		obj, err := resource.Object()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve object for resource %s: %w", resource.Identity(), err,
			)
		}

		// Check if the object already exists (for ConvergingOperation detection)
		existingVersion := ""
		existing := obj.DeepCopyObject().(client.Object)
		if err := rec.Client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err == nil {
			existingVersion = existing.GetResourceVersion()
		} else if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"failed to check existence of resource %s: %w", resource.Identity(), err,
			)
		}

		// Apply mutations to desired state
		ownerRefSkipped, err := mutateResource(resource, obj, rec.Owner, rec.Scheme, mapper)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to mutate resource %s: %w", resource.Identity(), err,
			)
		}

		// SSA requires managedFields to be nil in the patch object.
		// After the first apply, DesiredObject retains server state including managedFields.
		// Clear it before patching to avoid "metadata.managedFields must be nil" errors.
		obj.SetManagedFields(nil)

		// Set GVK on the object (required for SSA — builders often omit TypeMeta)
		if err := ensureGVK(obj, rec.Scheme); err != nil {
			return nil, fmt.Errorf(
				"failed to determine GVK for resource %s: %w", resource.Identity(), err,
			)
		}

		// Server-Side Apply
		if err := rec.Client.Patch(ctx, obj, client.Apply, client.ForceOwnership, fieldOwner); err != nil {
			return nil, fmt.Errorf(
				"failed to apply resource %s: %w", resource.Identity(), err,
			)
		}

		// Determine operation based on whether the resource existed and whether it changed
		var convergingOperation concepts.ConvergingOperation
		switch {
		case existingVersion == "":
			convergingOperation = concepts.ConvergingOperationCreated
		case existingVersion != obj.GetResourceVersion():
			convergingOperation = concepts.ConvergingOperationUpdated
		default:
			convergingOperation = concepts.ConvergingOperationNone
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
		if status != nil {
			results = append(results, convergingResult{Resource: resource, Status: *status})
		}

		recording.RecordApplyOperationEvent(rec.Recorder, convergingOperation, obj, rec.Owner)
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
	if len(gvks) > 0 {
		obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	}
	return nil
}
