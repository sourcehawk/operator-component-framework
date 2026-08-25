package component

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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
// This is the shared core used by both applyResources (suspension path) and
// reconcileResources (normal path) to avoid duplicating the Object/Mutate/SSA/
// status-collection sequence.
func applyResource(
	ctx context.Context, rec ReconcileContext, resource Resource, componentName string,
	fieldOwner client.FieldOwner, mapper meta.RESTMapper, skipOwnerRef bool,
) (*reconcileResult, error) {
	obj, err := resource.Object()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve object for resource %s: %w", resource.Identity(), err,
		)
	}

	// Set GVK on the object (required for SSA — builders often omit TypeMeta).
	// It runs this early so the object's kind is known for the metric labels on
	// every later path, success and failure alike.
	if err := ensureGVK(obj, rec.Scheme); err != nil {
		return nil, fmt.Errorf(
			"failed to determine GVK for resource %s: %w", resource.Identity(), err,
		)
	}

	// Errors before this point leave no kind to label a metric with, and mean
	// the framework never worked out what to apply. They surface as a returned
	// error and an error condition instead.
	labels := resourceMetricLabels(rec, componentName, resource, obj)

	// Check if the object already exists (read through the client, usually the informer cache).
	// The observed object is fetched into a zeroed instance rather than a copy of
	// the desired object, so the pre-apply snapshot used by the comparison below
	// holds only what the client returned and does not rely on the client zeroing
	// its target before decoding.
	var objectExists bool
	existing, err := newEmptyObjectLike(obj)
	if err != nil {
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to prepare existence check for resource %s: %w", resource.Identity(), err,
		))
	}
	if err := rec.Client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err == nil {
		objectExists = true
	} else if !apierrors.IsNotFound(err) {
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to check existence of resource %s: %w", resource.Identity(), err,
		))
	}

	// Apply mutations to desired state
	ownerRefSkipped, err := mutateResource(resource, obj, rec.Owner, rec.Scheme, mapper, skipOwnerRef)
	if err != nil {
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to mutate resource %s: %w", resource.Identity(), err,
		))
	}

	// Prepare the object for SSA by clearing server-populated fields that must not
	// be present in the patch. After the first reconcile, DesiredObject retains the
	// server response (including these fields) because Mutate updates the internal
	// pointer and Patch writes back into the same object.
	clearServerFields(obj)

	// Re-assert the GVK. A Mutate implementation that assigns the whole struct
	// (*current = *desired) drops the TypeMeta set above, and SSA requires it.
	// The call is a no-op whenever the kind is still present.
	if err := ensureGVK(obj, rec.Scheme); err != nil {
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to determine GVK for resource %s: %w", resource.Identity(), err,
		))
	}

	// Server-Side Apply with forced ownership.
	// client.Apply is deprecated in favor of client.Client.Apply() which requires generated
	// ApplyConfiguration types. Using Patch with Apply is the pragmatic approach for untyped objects.
	if err := rec.Client.Patch(ctx, obj, client.Apply, client.ForceOwnership, fieldOwner); err != nil { //nolint:staticcheck
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to apply resource %s: %w", resource.Identity(), err,
		))
	}

	// Classify the apply by comparing the object observed before the patch (read
	// through the client, usually the informer cache) with the server's response
	// after it. Comparing the desired object before and after
	// Mutate would misreport an update on every reconcile for operators that
	// rebuild their desired objects each pass, because the owner reference and
	// feature mutations are always added relative to the freshly built object.
	convergingOperation := concepts.ConvergingOperationCreated
	if objectExists {
		changed, err := appliedObjectChanged(existing, obj)
		if err != nil {
			return nil, applyFailed(rec, labels, fmt.Errorf(
				"failed to compare applied state of resource %s: %w", resource.Identity(), err,
			))
		}
		convergingOperation = concepts.ConvergingOperationNone
		if changed {
			convergingOperation = concepts.ConvergingOperationUpdated
		}
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
		return nil, applyFailed(rec, labels, fmt.Errorf(
			"failed to determine converging status of resource %s: %w", resource.Identity(), err,
		))
	}

	recording.RecordApplyOperationEvent(rec.EventRecorder, convergingOperation, obj, rec.Owner)
	if rec.Metrics != nil {
		rec.Metrics.RecordResourceApply(labels, convergingOperation)
	}

	if status != nil {
		return &reconcileResult{Status: *status}, nil
	}
	return nil, nil
}

// resourceMetricLabels builds the label set for a resource's metrics.
//
// The identifier comes from the resource when it implements
// concepts.MetricsIdentifiable and returns a non-empty value, and defaults to
// the lowercased kind otherwise. The default is always bounded and needs no
// configuration, at the cost of collapsing two resources of the same kind in
// one component into a single series until one of them is given an identifier.
//
// The default is resolved here rather than in the resource, so that every
// Resource implementation is labelled the same way, hand-written ones included.
func resourceMetricLabels(
	rec ReconcileContext, componentName string, resource Resource, obj client.Object,
) ResourceMetricLabels {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	identifier := strings.ToLower(kind)
	if identifiable, ok := resource.(concepts.MetricsIdentifiable); ok {
		// A blank identifier is treated as unset, matching what the builders
		// reject at build time. A hand-written implementation is not held to
		// that check, and " " is not a label value anyone meant to key a series
		// by. Anything else is taken verbatim: rewriting a caller's identifier
		// would silently split or merge series.
		if configured := identifiable.MetricsIdentifier(); strings.TrimSpace(configured) != "" {
			identifier = configured
		}
	}
	return ResourceMetricLabels{
		OwnerKind:  rec.Owner.GetKind(),
		Component:  componentName,
		Identifier: identifier,
		Kind:       kind,
	}
}

// applyFailed records an apply error and returns the error unchanged, so that
// every error return after the object's kind is known stays a single statement.
func applyFailed(rec ReconcileContext, labels ResourceMetricLabels, err error) error {
	if rec.Metrics != nil {
		rec.Metrics.RecordResourceApplyError(labels)
	}
	return err
}

// applyFieldOwner returns the Server-Side Apply field manager for one component
// of one owner: "<Kind>/<component>/<owner UID>", for example
// "ExampleApp/web-interface/3d8a9d5e-1c2b-4f6e-9a7d-0b1c2d3e4f5a". The owner's
// UID is part of the name so that two owners of the same kind whose components
// render the same object are distinct managers to the API server. With a manager
// shared across owners, each owner's forced apply would relinquish the other's
// fields wholesale and neither would ever see a conflict. The UID rather than
// the owner's name keeps the manager independent of the length of user-chosen
// names, which matters for the API server's 128-character manager limit, and
// reads the same for cluster-scoped and namespaced owners.
func applyFieldOwner(owner OperatorCRD, componentName string) client.FieldOwner {
	return client.FieldOwner(
		fmt.Sprintf("%s/%s/%s", owner.GetKind(), componentName, owner.GetUID()),
	)
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
//  4. Data Extraction: Each resource's declared data extractions run immediately after it
//     is applied, so a later resource's mutations can read what an earlier one produced.
//     Guards are not evaluated on this path; the caller uses reconcileResources for that.
//
// Server-Side Apply behavior:
//   - The resource's desired state is built via Object() + Mutate(), then patched into the
//     cluster with forced field ownership. Only operator-managed fields are sent; server-defaulted
//     fields (e.g., imagePullPolicy, strategy) are untouched. This prevents perpetual updates
//     that occur with CreateOrUpdate when the API server re-adds defaults every reconcile.
//   - Field ownership is derived from the owner's Kind, the component name and the
//     owner's UID (see applyFieldOwner). Forced ownership means the framework takes
//     control of any conflicting fields from other managers for fields it explicitly
//     declares.
func applyResources(
	ctx context.Context, rec ReconcileContext, entries []reconcileEntry,
	componentName string, mapper meta.RESTMapper,
) ([]reconcileResult, error) {
	fieldOwner := applyFieldOwner(rec.Owner, componentName)

	var results []reconcileResult

	for _, entry := range entries {
		result, err := applyResource(
			ctx, rec, entry.Resource, componentName, fieldOwner, mapper, entry.Options.Unowned,
		)
		if err != nil {
			return nil, err
		}
		if result != nil {
			results = append(results, *result)
		}

		// Per-resource data extraction: run immediately after the apply so that
		// extracted data is available to subsequent resources' mutations. This
		// path is used during suspension, where a consumer's content mutations
		// still run and may Require a cell an earlier managed producer fills.
		// extractResourceData already wraps failures with the resource identity.
		if err := extractResourceData([]Resource{entry.Resource}); err != nil {
			return nil, err
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
) ([]reconcileResult, error) {
	fieldOwner := applyFieldOwner(rec.Owner, componentName)

	var results []reconcileResult

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
				results = append(results, reconcileResult{
					Entry: entry,
					Status: convergingStatusWithReason{
						Status: convergingStatusGuardBlocked,
						Reason: guardResult.Reason,
					},
				})
				return results, nil
			}
		}

		// Process the resource based on its mode
		var result *reconcileResult
		var err error
		if entry.Options.ReadOnly {
			result, err = readResource(ctx, rec, resource)
		} else {
			result, err = applyResource(
				ctx, rec, resource, componentName, fieldOwner, mapper, entry.Options.Unowned,
			)
		}
		if err != nil {
			if entry.Options.ReadOnly && apierrors.IsNotFound(err) {
				switch {
				case entry.Options.IgnoreIfAbsent:
					continue
				case entry.Options.BlockOnAbsence:
					results = append(results, reconcileResult{
						Entry: entry,
						Status: convergingStatusWithReason{
							Status: convergingStatusGuardBlocked,
							Reason: fmt.Sprintf("waiting for %s", resource.Identity()),
						},
					})
					return results, nil
				}
			}
			return nil, err
		}
		if result != nil {
			result.Entry = entry
			results = append(results, *result)
		}

		// Per-resource data extraction: run immediately after processing so that
		// extracted data is available to subsequent resources' guards and mutations.
		// extractResourceData already wraps failures with the resource identity.
		if err := extractResourceData([]Resource{resource}); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// mutateResource applies all desired-state mutations and sets the controller owner
// reference. When skipOwnerRef is true the owner reference is intentionally omitted;
// the resource is not garbage-collected when the owner CR is deleted.
func mutateResource(
	resource Resource, obj client.Object, owner client.Object,
	scheme *runtime.Scheme, mapper meta.RESTMapper, skipOwnerRef bool,
) (ownerRefSkipped bool, err error) {
	if err := resource.Mutate(obj); err != nil {
		return false, err
	}

	if skipOwnerRef {
		// Remove only the owner reference that points to the component owner, preserving
		// any other owner references that Mutate() may have set for different objects.
		// This also clears a cached ref from a previous reconcile where Unowned() was not set
		// (the DesiredObject pointer retains the server response). Omitting or emptying the
		// field in the SSA patch removes entries this field manager previously owned.
		ownerUID := owner.GetUID()
		existing := obj.GetOwnerReferences()
		n := 0
		for _, ref := range existing {
			if ref.UID != ownerUID {
				existing[n] = ref
				n++
			}
		}
		obj.SetOwnerReferences(existing[:n])
		return false, nil
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

// newEmptyObjectLike returns a zero-valued object of the same Go type as obj,
// carrying obj's GroupVersionKind so that unstructured objects remain
// addressable through the client. It returns an error for a nil or non-pointer
// object, which the client could not decode into anyway.
func newEmptyObjectLike(obj client.Object) (client.Object, error) {
	val := reflect.ValueOf(obj)
	if !val.IsValid() || val.Kind() != reflect.Pointer || val.IsNil() {
		return nil, fmt.Errorf("object must be a non-nil pointer, got %T", obj)
	}
	empty, ok := reflect.New(val.Type().Elem()).Interface().(client.Object)
	if !ok {
		return nil, fmt.Errorf("zero value of %T does not implement client.Object", obj)
	}
	empty.GetObjectKind().SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	return empty, nil
}

// appliedObjectChanged reports whether a Server-Side Apply changed the object,
// by comparing the object observed before the patch (read through the client,
// usually the informer cache) with the API server's response to it.
//
// The comparison ignores fields that change without the desired state changing:
// the status subresource (written by other controllers), managedFields and
// resourceVersion (bookkeeping the server touches on any write), generation
// (derived from the compared content) and TypeMeta (present or absent depending
// on how the object was decoded). Everything else, including labels,
// annotations, owner references and the object's spec or data, counts.
//
// It does not rely on resourceVersion because concurrent status writes bump it
// without touching the desired state, and because fake clients bump it on
// every apply, even a no-op one.
func appliedObjectChanged(before, after client.Object) (bool, error) {
	beforeContent, err := comparableContent(before)
	if err != nil {
		return false, err
	}
	afterContent, err := comparableContent(after)
	if err != nil {
		return false, err
	}
	return !equality.Semantic.DeepEqual(beforeContent, afterContent), nil
}

// comparableContent renders an object as an unstructured map with the fields
// appliedObjectChanged ignores removed. It copies the top-level and metadata
// maps before deleting keys so the object itself is never mutated (for
// *unstructured.Unstructured, ToUnstructured returns the object's own content).
func comparableContent(obj client.Object) (map[string]any, error) {
	converted, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	content := make(map[string]any, len(converted))
	for k, v := range converted {
		content[k] = v
	}
	delete(content, "apiVersion")
	delete(content, "kind")
	delete(content, "status")

	if rawMetadata, ok := content["metadata"].(map[string]any); ok {
		metadata := make(map[string]any, len(rawMetadata))
		for k, v := range rawMetadata {
			metadata[k] = v
		}
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "generation")
		content["metadata"] = metadata
	}

	return content, nil
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
