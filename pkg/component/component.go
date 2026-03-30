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

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
	//  - If the resource is static (not implementing any of the concepts mentioned), the mode only
	//    takes effect when the resource has a guard. A guarded static resource can report Blocked,
	//    which participates in health aggregation.
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

	// reconcileResources holds all non-delete resources in registration order.
	// Each entry records whether the resource is read-only or managed (create/update).
	reconcileResources  []reconcileEntry
	deleteResources     []Resource
	resourceLookup      map[string]Resource
	participationLookup map[string]ParticipationMode

	gracePeriod time.Duration

	// featureGate controls whether the component is active. When the gate is
	// disabled, all resources are deleted and the condition is set to True/Disabled.
	featureGate feature.Gate

	// prerequisites are initialization barriers that must all be met before the
	// component reconciles for the first time. Once the component passes through
	// to normal reconciliation, prerequisites are never re-evaluated.
	prerequisites []Prerequisite
}

// reconcileEntry pairs a resource with its reconciliation mode.
type reconcileEntry struct {
	Resource Resource
	ReadOnly bool
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
//  1. Feature gate check: If a feature gate is set and disabled, all resources
//     managed by the component are deleted and the condition is set to
//     True/Disabled. No further processing occurs.
//
//  2. Prerequisite check: If prerequisites are registered and the initialization
//     barrier has not yet been passed, all prerequisites are evaluated. The barrier
//     is considered active while the condition reason is Unknown, PrerequisiteNotMet,
//     Disabled, or FeatureGateError. If any prerequisite is not met, the condition
//     is set to False/PrerequisiteNotMet and no resources are reconciled or
//     suspended. Once the component reconciles or suspends successfully, the barrier
//     is permanently cleared and prerequisites are never re-evaluated.
//
//  3. Suspension check: If the component is marked as suspended, it performs
//     suspension of all managed (non-read-only) resources. Guards are not evaluated.
//     The status is updated to reflect suspension progress (PendingSuspension,
//     Suspending, or Suspended), and then deletion resources are processed.
//
//  4. Resource reconciliation: All non-delete resources are processed sequentially
//     in registration order, regardless of whether they are managed or read-only.
//     For each resource:
//     - Its guard (if any) is evaluated. A blocked guard stops processing of that
//     resource and all subsequent resources.
//     - The resource is either applied (managed) or fetched (read-only).
//     - Its data extractors run immediately, making extracted data available to
//     subsequent resources' guards and mutations.
//
//  5. Status Aggregation: Collects converging status from all processed resources
//     (including any blocked guard result).
//
//  6. Condition Update: Derives a new component condition using a stateful
//     progression model that considers the aggregate resource status, the
//     previous condition, and the configured grace period to avoid churn.
//
//  7. Resource Deletion: Finally, it deletes any resources registered for deletion.
func (c *Component) Reconcile(ctx context.Context, rec ReconcileContext) error {
	// Add logging context to the logger within this reconcile
	logger := log.FromContext(ctx).WithValues(
		"component", c.name,
		"condition", c.conditionType,
	)
	ctx = log.IntoContext(ctx, logger)

	mapper := rec.Client.RESTMapper()
	if mapper == nil {
		return fail(
			ctx, rec, c.conditionType, fmt.Errorf(
				"ReconcileContext.Client.RESTMapper() returned nil; a valid RESTMapper is required for reconciliation",
			),
		)
	}

	// Feature gate: when disabled, delete all resources and report Disabled.
	if c.featureGate != nil {
		enabled, err := c.featureGate.Enabled()
		if err != nil {
			cond := conditionFeatureGateError(c.conditionType, err, rec.Owner.GetGeneration())
			_ = setStatusCondition(ctx, rec, cond)
			return err
		}

		if !enabled {
			if err := deleteResources(ctx, rec, c.allManagedResources(), withDeletionReason("disabled feature gate")); err != nil {
				return fail(ctx, rec, c.conditionType, err)
			}

			cond := conditionDisabled(c.conditionType, rec.Owner.GetGeneration())
			return setStatusCondition(ctx, rec, cond)
		}
	}

	// Prerequisite barrier: block reconciliation until all prerequisites are met.
	// The barrier is active while the condition reason is Unknown, PrerequisiteNotMet,
	// Disabled, or FeatureGateError. Once the component reconciles or suspends
	// successfully, the barrier is permanently cleared.
	if len(c.prerequisites) > 0 {
		currentCondition := c.GetCondition(rec.Owner)
		if c.prerequisiteBarrierActive(currentCondition) {
			result, err := c.evaluatePrerequisites(rec)
			if err != nil {
				cond := conditionPrerequisiteNotMet(c.conditionType, err.Error(), rec.Owner.GetGeneration())
				_ = setStatusCondition(ctx, rec, cond)
				return err
			}

			if result.Status == PrerequisiteStatusNotMet {
				cond := conditionPrerequisiteNotMet(c.conditionType, result.Reason, rec.Owner.GetGeneration())
				return setStatusCondition(ctx, rec, cond)
			}
		}
	}

	// Perform suspension reconciliation if component is marked as suspended.
	// Only managed (non-read-only) resources are suspended.
	if c.suspended {
		managed := c.managedResources()
		results, err := suspendResources(ctx, rec, managed, c.name, mapper)
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

	// Reconcile all resources in registration order. Each resource is either
	// fetched (read-only) or applied (managed) with guard evaluation and
	// per-resource data extraction, so that extracted data from earlier resources
	// is available to subsequent resources' guards and mutations.
	results, err := reconcileResources(ctx, rec, c.reconcileResources, c.name, mapper)
	if err != nil {
		return fail(ctx, rec, c.conditionType, err)
	}

	// Determine new condition for component
	cond := newConvergingStatusCondition(
		ctx,
		rec.Owner,
		convergeResults(results).filterParticipators(c.participationLookup),
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

// allManagedResources returns every managed (non-read-only) resource known to
// the component, combining non-read-only reconcile entries and delete entries
// into a single slice. This is used when the feature gate is disabled and all
// managed resources must be deleted. Read-only resources are excluded because
// they are never created or modified by the component.
func (c *Component) allManagedResources() []Resource {
	resources := make([]Resource, 0, len(c.reconcileResources)+len(c.deleteResources))
	for _, entry := range c.reconcileResources {
		if !entry.ReadOnly {
			resources = append(resources, entry.Resource)
		}
	}
	resources = append(resources, c.deleteResources...)
	return resources
}

// prerequisiteBarrierActive reports whether the prerequisite initialization
// barrier is still active based on the component's current condition. The
// barrier is active when the component has never successfully reconciled,
// indicated by a condition reason of Unknown, PrerequisiteNotMet, Disabled,
// or FeatureGateError. Disabled is included because a component that was
// gated off has never reconciled its resources, so prerequisites must be
// evaluated when the gate is later enabled. FeatureGateError is included
// because the feature gate check runs before prerequisites; a failure there
// means the component never reached resource reconciliation.
func (c *Component) prerequisiteBarrierActive(cond Condition) bool {
	reason := Status(cond.Reason)
	return reason == Unknown || reason == PrerequisiteNotMet || reason == Disabled || reason == FeatureGateError
}

// evaluatePrerequisites checks all registered prerequisites in order. It
// returns the first NotMet result or error encountered. If all prerequisites
// are satisfied, it returns a Met result.
func (c *Component) evaluatePrerequisites(rec ReconcileContext) (PrerequisiteResult, error) {
	for _, prereq := range c.prerequisites {
		result, err := prereq.Check(rec)
		if err != nil {
			return PrerequisiteResult{}, err
		}
		if result.Status == PrerequisiteStatusNotMet {
			return result, nil
		}
	}
	return PrerequisiteResult{Status: PrerequisiteStatusMet}, nil
}

// managedResources returns only the non-read-only resources from the reconcile list.
// This is used during suspension, where only managed resources are suspended.
func (c *Component) managedResources() []Resource {
	var managed []Resource
	for _, entry := range c.reconcileResources {
		if !entry.ReadOnly {
			managed = append(managed, entry.Resource)
		}
	}
	return managed
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
