package component

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
)

// OperatorCRD defines the interface for the custom resource that owns the component.
// It must support status conditions and provide its kind for recording purposes.
type OperatorCRD interface {
	client.Object

	// GetStatusConditions returns a pointer to the slice of status conditions.
	// This is used to read and update the component's status condition on the owner.
	GetStatusConditions() *[]metav1.Condition
	// GetKind returns the string representation of the CRD's Kind.
	GetKind() string
}

// Recorder is an interface for recording status condition changes as metrics.
type Recorder interface {
	// RecordConditionFor records a condition change for a specific object and kind.
	RecordConditionFor(
		kind string, object ocm.ObjectLike,
		conditionType, conditionStatus, conditionReason string, lastTransitionTime time.Time,
		extraLabelValues ...string,
	)
}

// ReconcileContext carries the dependencies and target object for a reconciliation loop.
type ReconcileContext struct {
	// Client is the Kubernetes client for resource operations.
	Client client.Client
	// Scheme is the runtime scheme for the operator.
	Scheme *runtime.Scheme
	// Recorder is the event recorder for publishing Kubernetes events.
	Recorder record.EventRecorder
	// Metrics is the recorder for status condition metrics.
	Metrics Recorder
	// Owner is the custom resource that owns and is updated by the components.
	Owner OperatorCRD
}

// ParticipationMode describes in what way the resource participates in the component health aggregation.
type ParticipationMode string

const (
	// ParticipationModeRequired The resource must be in a 'Healthy', 'Completed' or 'Operational' state for the
	// component to be considered healthy.
	//
	//  - concepts.Alive: It must be 'Healthy'
	//  - concepts.Operational: It must be 'Operational'
	//  - concepts.Completable: It must be 'Completed'
	//  - If the resource is static, e.g. not implementing any of the concepts mentioned, the mode has no effect,
	//    since the resource's health is determined by whether it can be created or not.
	ParticipationModeRequired ParticipationMode = "Required"
	// ParticipationModeAuxiliary The resource is auxiliary and not part of component health evaluation.
	ParticipationModeAuxiliary ParticipationMode = "Auxiliary"
)

// Component represents a logical grouping of Kubernetes resources that are
// reconciled together and reported as a single condition on an owning object.
//
// A component is responsible for:
//   - Creating, updating, or reading its registered resources
//   - Aggregating resource-level readiness into a single converging status
//   - Managing optional grace-period behavior for degraded or down states
//   - Handling suspension, including mutation-based suspension and optional deletion
//
// Each Component manages exactly one condition type on the owner and is reconciled
// independently of other components. Resources are registered during construction
// using WithResource and the configuration is finalized by calling Build().
type Component struct {
	name      string
	suspended bool

	conditionType ConditionType

	createResources     []Resource
	readResources       []Resource
	deleteResources     []Resource
	resourceLookup      map[string]Resource
	participationLookup map[string]ParticipationMode

	gracePeriod time.Duration
}

// GetName returns the name of the component, which is used for logging and identification.
func (c *Component) GetName() string {
	return c.name
}

// GetCondition returns the current component condition from the owner object.
// It returns a synthetic "Unknown" condition if the condition is not yet present on the owner.
//
// Note: Always reconcile before retrieving this condition to ensure it reflects the
// latest cluster state; otherwise, it may be stale.
func (c *Component) GetCondition(owner OperatorCRD) Condition {
	cond := meta.FindStatusCondition(*owner.GetStatusConditions(), string(c.conditionType))
	if cond == nil {
		return conditionUnknown(c.conditionType, owner.GetGeneration())
	}

	return Condition(*cond)
}

// Reconcile converges the component to the desired state.
//
// A component manages its own condition on the parent and updates it accordingly
// to represent currently observable facts about the component status.
//
// Reconciliation follows these steps:
//
//  1. Suspension check: If the component is marked as suspended, it performs
//     suspension of all registered creation resources, updates the status to
//     reflect the suspension progress (PendingSuspension, Suspending, or Suspended),
//     and finally processes any deletion resources.
//
//  2. Resource Creation/Update: If not suspended, it creates or updates all
//     registered creation resources. If any resource creation fails, the
//     component condition is set to Error and reconciliation stops.
//
//  3. Read-only Resources: Fetches the current state of all registered
//     read-only resources from the cluster.
//
//  4. Status Aggregation: Collects converging status from all resources that implement the resource concepts.
//
//  5. Condition Update: Derives a new component condition using a stateful
//     progression model that considers the aggregate resource status, the
//     previous condition, and the configured grace period to avoid churn.
//
//  6. Resource Deletion: Finally, it deletes any resources registered for deletion.
func (c *Component) Reconcile(ctx context.Context, rec ReconcileContext) error {
	// Add logging context to the logger within this reconcile
	logger := log.FromContext(ctx).WithValues(
		"component", c.name,
		"condition", c.conditionType,
	)
	ctx = log.IntoContext(ctx, logger)

	mapper := rec.Client.RESTMapper()
	if mapper == nil {
		return fmt.Errorf("ReconcileContext.Client.RESTMapper() returned nil; a valid RESTMapper is required for reconciliation")
	}

	// Perform suspension reconciliation if component is marked as suspended
	if c.suspended {
		results, err := suspendResources(ctx, rec, c.createResources, mapper)
		if err != nil {
			return fail(ctx, rec, c.conditionType, err)
		}

		cond := suspendingCondition(
			c.conditionType,
			suspensionResults(results).summary(),
			rec.Owner.GetGeneration(),
		)
		if err := setStatusCondition(ctx, rec, cond); err != nil {
			return err
		}

		if err := deleteResources(ctx, rec, c.deleteResources); err != nil {
			return fail(ctx, rec, c.conditionType, err)
		}

		return nil
	}

	// Create or update resources otherwise
	createResults, err := createOrUpdateResources(ctx, rec, c.createResources, mapper)
	if err != nil {
		return fail(ctx, rec, c.conditionType, err)
	}

	// Get readonly resources
	readonlyResults, err := readResources(ctx, rec, c.readResources)
	if err != nil {
		return fail(ctx, rec, c.conditionType, err)
	}

	// Extract resource data if any
	if err := extractResourceData(append(c.createResources, c.readResources...)); err != nil {
		return fail(ctx, rec, c.conditionType, err)
	}

	// Determine new condition for component
	cond := newConvergingStatusCondition(
		ctx,
		rec.Owner,
		convergeResults(append(createResults, readonlyResults...)).filterParticipators(c.participationLookup),
		c.gracePeriod,
		c.GetCondition(rec.Owner),
	)
	if err := setStatusCondition(ctx, rec, cond); err != nil {
		return err
	}

	if err := deleteResources(ctx, rec, c.deleteResources); err != nil {
		return fail(ctx, rec, c.conditionType, err)
	}

	return nil
}

// fail sets the component's error status condition on the owner and returns the
// provided error.
//
// This helper centralizes the common reconciliation pattern where a failure
// should both:
//  1. Update the component condition on the owner to reflect the error.
//  2. Propagate the error to stop further reconciliation.
//
// The error from setting the status condition is intentionally ignored because
// the original reconciliation error is considered the primary failure.
func fail(
	ctx context.Context,
	rec ReconcileContext,
	conditionType ConditionType,
	err error,
) error {
	cond := conditionError(conditionType, err, rec.Owner.GetGeneration())
	_ = setStatusCondition(ctx, rec, cond)
	return err
}
