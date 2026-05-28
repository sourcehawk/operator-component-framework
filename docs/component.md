# Component System

The `component` package provides a structured way to manage logical features in a Kubernetes operator by grouping
related resources into **Components**.

A Component acts as a single behavioral unit: it reconciles multiple resources, manages their shared lifecycle, and
reports their aggregate health through one condition on the owner CRD.

## Table of Contents

- [Building a Component](#building-a-component)
  - [Resource Registration Options](#resource-registration-options)
  - [Building Resource Options with Feature Gating](#building-resource-options-with-feature-gating)
- [Component Feature Gates](#component-feature-gates)
- [Prerequisites](#prerequisites)
  - [Registering Prerequisites](#registering-prerequisites)
  - [Prerequisite Behavior](#prerequisite-behavior)
  - [Status Reporting](#status-reporting)
- [Reconciliation Lifecycle](#reconciliation-lifecycle)
- [Cluster-Scoped Resources](#cluster-scoped-resources)
- [Status Model](#status-model)
  - [Alive Resources](#alive-resources-alive-interface)
  - [Completable Resources](#completable-resources-completable-interface)
  - [Operational Resources](#operational-resources-operational-interface)
  - [Static Resources](#static-resources-no-interface)
  - [Grace States](#grace-states)
  - [Suspension States](#suspension-states)
  - [Guard State](#guard-state)
  - [Prerequisite State](#prerequisite-state)
  - [Feature Gate State](#feature-gate-state)
  - [Condition Priority](#condition-priority)
- [Grace Period](#grace-period)
- [Suspension Lifecycle](#suspension-lifecycle)
- [ReconcileContext](#reconcilecontext)
- [Persisting Status with FlushStatus](#persisting-status-with-flushstatus)
- [Guards](#guards)
  - [Registering a Guard](#registering-a-guard)
  - [Guard Behavior](#guard-behavior)
  - [Status Reporting](#status-reporting-1)
- [Best Practices](#best-practices)

## Building a Component

Components are constructed through a builder. The builder collects resource registrations, configuration, and lifecycle
flags, then produces an immutable `Component` ready for reconciliation.

```go
comp, err := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithFeatureGate(webFeature).                                     // optional: disable to remove all resources
    WithPrerequisite(component.DependsOn("DatabaseReady")).   // optional: wait for another component
    WithResource(deployment, component.ResourceOptions{}).
    WithResource(configMap, component.ResourceOptions{ReadOnly: true}).
    WithResource(oldService, component.ResourceOptions{Delete: true}).
    WithGracePeriod(5 * time.Minute).
    Suspend(owner.Spec.Suspended).
    Build()
if err != nil {
    return err
}
```

### Resource Registration Options

Each resource is registered with a `ResourceOptions` struct that controls how the component interacts with it:

| Option                                                           | Behavior                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ResourceOptions{}` (default)                                    | **Managed**: created or updated; health contributes to condition                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `ResourceOptions{ReadOnly: true}`                                | **Read-only**: fetched but never modified; health still contributes                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `ResourceOptions{Delete: true}`                                  | **Delete-only**: removed from the cluster if present; does not contribute to health                                                                                                                                                                                                                                                                                                                                                                                                   |
| `ResourceOptions{ParticipationMode: ParticipationModeAuxiliary}` | The resource's health does not contribute to the component condition. The component can become Ready regardless of this resource's state. **Exception:** a blocked [guard](#guards) always contributes to the condition regardless of participation mode, because it halts the entire reconciliation pipeline                                                                                                                                                                         |
| `ResourceOptions{SuppressGraceInconsistencyWarning: true}`       | Suppresses the warning log emitted when the resource's grace handler returns Healthy while its convergence handler returns non-healthy. Use this when the inconsistency is intentional (e.g., a custom grace handler that deliberately reports Healthy for a resource that has not fully converged)                                                                                                                                                                                   |
| `ResourceOptions{ReadOnly: true, BlockOnAbsence: true}`          | **Read-only with watch-driven retry**: a NotFound from the cluster is recorded as a blocked status (`waiting for <resource>`) and short-circuits the remaining resources, instead of erroring back through controller-runtime's exponential backoff. Use only when the consumer has a watch on the resource's type so the reconcile is re-enqueued when it appears                                                                                                                    |
| `ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true}`          | **Optional read-only**: a NotFound from the cluster is silently ignored. The entry contributes nothing to the component's conditions, no observation is recorded, and the data extractor is not invoked. Subsequent resources reconcile unchanged. State recorded from earlier reconciles (last observation, extracted data) is preserved across an absence rather than reset. Use for resources that may legitimately be absent (e.g. a referenced Secret owned by another operator) |

### Building Resource Options with Feature Gating

When a resource's lifecycle depends on a feature gate or runtime conditions, use `ResourceOptionsBuilder` to construct
the options declaratively. The builder integrates with the [feature system](../pkg/feature/) so that entire resources
can be conditionally created or deleted based on feature state.

```go
opts, err := component.NewResourceOptionsBuilder().
    WithFeatureGate(metricsFeature).
    Auxiliary().
    Build()
if err != nil {
    return err
}

builder.WithResource(exporterService, opts)
```

The builder evaluates all conditions at `Build()` time and produces a plain `ResourceOptions` value. The `WithResource`
signature is unchanged.

**Methods:**

| Method                            | Effect                                                                                                                                                                                                                                                                                                                                                                 |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithFeatureGate(f feature.Gate)` | Gates the resource on a feature. When disabled, the resource is deleted.                                                                                                                                                                                                                                                                                               |
| `When(truth bool)`                | Adds a boolean condition (AND logic). If any condition is false, the resource is deleted. Calls are additive.                                                                                                                                                                                                                                                          |
| `Auxiliary()`                     | Sets participation mode to `Auxiliary` (resource does not affect component health).                                                                                                                                                                                                                                                                                    |
| `ReadOnly()`                      | Marks the resource as read-only. If the resource is also gated by a disabled feature, deletion takes precedence over read-only.                                                                                                                                                                                                                                        |
| `BlockOnAbsence()`                | Opts a read-only resource into guard-blocked semantics on NotFound. Requires `ReadOnly()` and is mutually exclusive with `IgnoreIfAbsent()`; `Build()` errors otherwise. Requires a watch on the resource's type to avoid stalling until the periodic resync.                                                                                                          |
| `IgnoreIfAbsent()`                | Opts a read-only resource into "optional" semantics: a NotFound is silently ignored, the entry is skipped, no condition or observation is reported, and the data extractor is not invoked. State recorded from earlier reconciles is preserved across an absence. Requires `ReadOnly()` and is mutually exclusive with `BlockOnAbsence()`; `Build()` errors otherwise. |

For the common case of gating a resource on a single feature, use the convenience function:

```go
opts, err := component.ResourceOptionsFor(tracingFeature)
```

**Resolution rules:**

1. If the feature is non-nil and evaluates to disabled, the resource is deleted.
2. If any When condition evaluates to false, the resource is deleted.
3. Deletion takes precedence over read-only mode.
4. Participation mode is preserved regardless of deletion state.

**Example: mixed feature-gated and static resources:**

```go
tracingOpts, err := component.ResourceOptionsFor(
    feature.NewVersionGate(owner.Spec.Version, nil).When(owner.Spec.TracingEnabled),
)
if err != nil {
    return err
}

comp, err := component.NewComponentBuilder().
    WithName("api-server").
    WithConditionType("ApiServerReady").
    WithResource(apiDeployment, component.ResourceOptions{}).
    WithResource(jaegerSidecar, tracingOpts).
    Build()
```

When `TracingEnabled` is true, the Jaeger sidecar is created and managed. When false, it is deleted from the cluster.

## Component Feature Gates

A component-level feature gate controls whether the component is active. When the gate is disabled, the component
deletes all of its resources and reports a `True` condition with reason `Disabled`. When enabled (or not set), the
component reconciles normally.

```go
comp, err := component.NewComponentBuilder().
    WithName("monitoring-sidecar").
    WithConditionType("MonitoringReady").
    WithFeatureGate(monitoringFeature).
    WithResource(exporterDeployment, component.ResourceOptions{}).
    WithResource(exporterService, component.ResourceOptions{}).
    Suspend(owner.Spec.Suspended).
    Build()
```

A disabled feature gate takes precedence over suspension. If the gate is disabled and the component is also marked
suspended, the component is treated as disabled (resources are deleted), not suspended.

The condition when the gate is disabled:

```yaml
type: MonitoringReady
status: "True"
reason: Disabled
message: "Component is disabled."
```

The `True` status follows the convention that `True` means "in its expected state", consistent with how a `Suspended`
component also reports `True`.

## Prerequisites

Prerequisites are initialization barriers that prevent a component from reconciling until a condition is met. Unlike
resource-level [guards](#guards), prerequisites are evaluated only while the component's condition reason indicates it
has not yet proceeded past initialization. The barrier remains active while the condition reason is `Unknown`,
`PrerequisiteNotMet`, `Disabled`, or `FeatureGateError`. Once the reason changes to any other value, the barrier is
permanently passed and the prerequisite is never re-evaluated.

This makes prerequisites suitable for expressing startup dependencies between components. If a dependency later becomes
unhealthy, the dependent component continues to reconcile its own resources. Prerequisites answer the question "can this
component be created?", not "should this component keep running?".

### Registering Prerequisites

Prerequisites are registered on the component builder using `WithPrerequisite`. Multiple prerequisites can be
registered; all must be satisfied before the component proceeds.

```go
comp, err := component.NewComponentBuilder().
    WithName("api-server").
    WithConditionType("ApiServerReady").
    WithPrerequisite(component.DependsOn("DatabaseReady")).
    WithPrerequisite(component.DependsOn("CacheReady")).
    WithResource(apiDeployment, component.ResourceOptions{}).
    WithResource(apiService, component.ResourceOptions{}).
    Suspend(owner.Spec.Suspended).
    Build()
```

The built-in `DependsOn` helper checks whether a named condition on the owner object has `Status: True`. The owner is
read from the `ReconcileContext` passed to `Check`, so no cluster reads are performed.

For custom logic, implement the `Prerequisite` interface:

```go
type Prerequisite interface {
    Check(rec ReconcileContext) (PrerequisiteResult, error)
}
```

### Prerequisite Behavior

- Prerequisites are evaluated before any resources are reconciled or suspended.
- The barrier is considered active when the component's condition reason is `Unknown`, `PrerequisiteNotMet`, `Disabled`,
  or `FeatureGateError`. Any other reason means the component has proceeded past initialization and the barrier is
  permanently passed.
- While the barrier is active, suspension is a no-op. No resources exist to suspend.
- A feature gate check runs before the prerequisite check. If the gate is disabled, prerequisites are not evaluated.
- Prerequisites are evaluated in registration order. The first unmet prerequisite short-circuits the check.
- A prerequisite error sets the component condition to `False` with reason `PrerequisiteNotMet`.

### Status Reporting

A blocked prerequisite produces a condition like:

```yaml
type: ApiServerReady
status: "False"
reason: PrerequisiteNotMet
message:
  'Prerequisite not met: waiting for condition "DatabaseReady" to become True (currently False: Database is still
  creating resources)'
```

## Reconciliation Lifecycle

`comp.Reconcile(ctx, recCtx)` runs a multi-phase process on every call:

**Phase 1: Feature gate check.** If a feature gate is set and disabled, all resources managed by the component are
deleted and the condition is set to `True/Disabled`. No further processing occurs.

**Phase 2: Prerequisite check.** If prerequisites are registered and the initialization barrier has not yet been passed
(condition reason is `Unknown`, `PrerequisiteNotMet`, `Disabled`, or `FeatureGateError`), all prerequisites are
evaluated. If any prerequisite is not met, the condition is set to `False/PrerequisiteNotMet` and no resources are
reconciled or suspended.

**Phase 3: Suspension check.** If the component is marked suspended, it calls `Suspend()` on all managed resources that
support suspension (create/update resources, not read-only ones), updates the condition, then processes any pending
deletions and returns. The remaining phases are skipped.

**Phase 4: Resource reconciliation.** All non-delete resources are processed sequentially in registration order,
regardless of whether they are managed or read-only. For each resource:

1. If the resource has a [guard](#guards), the guard is evaluated first. If blocked, the resource and all subsequent
   resources are skipped.
2. The resource is either applied to the cluster (managed) or fetched from it (read-only). Managed resources use
   Server-Side Apply and get a controller owner reference pointing to the owner CRD, unless the resource is
   cluster-scoped and the owner is namespace-scoped (see [Cluster-Scoped Resources](#cluster-scoped-resources)).
3. For read-only resources that implement `ObservationRecorder`, the framework records the fetched object back onto the
   resource so that subsequent inspection observes the live cluster state. Resources built from `generic.BaseResource`
   implement this automatically.
4. If the resource implements `DataExtractable`, its data extractors run immediately. This makes extracted data
   available to subsequent resources' guards and mutations within the same reconciliation cycle.

This means a read-only resource registered before a managed resource can extract data that feeds into the managed
resource's guard or mutations.

**Phase 5: Status aggregation and condition update.** The health of each resource is collected, the grace period is
consulted, and a single aggregate condition is written to the owner object's conditions **in memory**. `Reconcile` never
calls the Kubernetes API to persist status; the controller does that in a single write at the end of its reconcile loop.
See [Persisting Status with FlushStatus](#persisting-status-with-flushstatus).

**Phase 6: Resource deletion.** Resources registered for deletion are removed from the cluster.

## Cluster-Scoped Resources

When a component manages cluster-scoped resources (e.g., `ClusterRole`, `PersistentVolume`) and the owner CRD is
namespace-scoped, the framework **automatically skips** setting a controller owner reference on those resources. This is
a Kubernetes API constraint: a namespace-scoped object cannot own a cluster-scoped object.

The scope of both the owner and the resource is determined at reconcile time using the cluster's REST mapper. No
configuration is needed; the framework detects the incompatibility and logs an info-level message.

**Garbage collection caveat:** Without an owner reference, cluster-scoped resources are **not** automatically deleted
when the owner is removed. To ensure cleanup, either:

- Register the resource with `ResourceOptions{Delete: true}` so it is removed during reconciliation when no longer
  needed.
- Use a finalizer on the owner CRD to clean up cluster-scoped resources before the owner is deleted.

If the owner CRD is itself cluster-scoped, owner references are set normally on all resources regardless of their scope.

## Status Model

The status values a component reports depend on which lifecycle interfaces its resources implement. The component
aggregates across all registered resources and surfaces the most critical state.

### Alive Resources (`Alive` interface)

Reported by long-running workloads (Deployments, StatefulSets, DaemonSets):

| State      | Meaning                                                  |
| ---------- | -------------------------------------------------------- |
| `Healthy`  | The resource has reached its desired state               |
| `Creating` | The resource is being provisioned for the first time     |
| `Updating` | The resource is being modified with new configuration    |
| `Scaling`  | The resource is changing its replica count               |
| `Failing`  | The resource is failing to converge to its desired state |

### Completable Resources (`Completable` interface)

Reported by run-to-completion resources (Jobs, tasks):

| State         | Meaning                             |
| ------------- | ----------------------------------- |
| `Completed`   | The resource finished successfully  |
| `TaskRunning` | The resource is currently executing |
| `TaskPending` | The resource is waiting to start    |
| `TaskFailing` | The resource finished with an error |

### Operational Resources (`Operational` interface)

Reported by integration resources whose readiness depends on external systems (Services, Ingresses, Gateways, CronJobs):

| State              | Meaning                                           |
| ------------------ | ------------------------------------------------- |
| `Operational`      | The resource is fully operational                 |
| `OperationPending` | The resource is waiting on an external dependency |
| `OperationFailing` | The resource failed to reach an operational state |

### Static Resources (no interface)

Resources that implement none of the above interfaces are considered ready as long as they exist in the cluster. If a
static resource has a [guard](#guards), it can report `Blocked` when the guard precondition is not met.

### Grace States

When a component has a grace period configured and a `Graceful` resource has not reached its target state within that
period, the `Graceful` interface determines the post-expiry severity:

| State      | Meaning                                                                            |
| ---------- | ---------------------------------------------------------------------------------- |
| `Healthy`  | The resource is healthy (grace period expired without issue)                       |
| `Degraded` | The resource is partially functional or convergence is taking longer than expected |
| `Down`     | The resource is completely non-functional                                          |

### Suspension States

Reported during intentional deactivation:

| State               | Meaning                                                |
| ------------------- | ------------------------------------------------------ |
| `PendingSuspension` | Suspension is acknowledged but has not started         |
| `Suspending`        | Resources are actively being scaled down or cleaned up |
| `Suspended`         | All resources have reached their suspended state       |

### Guard State

| State     | Meaning                                                                      |
| --------- | ---------------------------------------------------------------------------- |
| `Blocked` | A resource's guard precondition is not met; it and subsequent resources wait |

See [Guards](#guards) for details.

### Prerequisite State

| State                | Meaning                                                                            |
| -------------------- | ---------------------------------------------------------------------------------- |
| `PrerequisiteNotMet` | A component-level prerequisite is not satisfied; no resources have been reconciled |

See [Prerequisites](#prerequisites) for details.

### Feature Gate State

| State      | Meaning                                                         |
| ---------- | --------------------------------------------------------------- |
| `Disabled` | The component's feature gate is disabled; all resources deleted |

See [Component Feature Gates](#component-feature-gates) for details.

### Condition Priority

When aggregating across multiple resources, the most critical state wins:

1. `Error` / `Down` / `Degraded`: something is wrong
2. Suspension states: the component is intentionally inactive
3. `Disabled`: the component is intentionally removed by a feature gate
4. `Blocked` / `PrerequisiteNotMet`: a precondition is not met
5. Converging states (`Creating`, `Updating`, `Scaling`, `TaskRunning`, `TaskPending`, `OperationPending`): the
   component is progressing
6. `Healthy` / `Completed` / `Operational`: all resources are in their target state

## Grace Period

The grace period defines how long a component may remain in a converging state (`Creating`, `Updating`, `Scaling`)
before transitioning to `Degraded` or `Down`.

```go
component.NewComponentBuilder().
    WithGracePeriod(5 * time.Minute).
    // ...
```

During the grace period the component reports its real converging state, not a failure. After the period expires, if the
component is still not `Ready`, the framework escalates to `Degraded` or `Down` based on resource health.

This prevents spurious failure alerts during normal operations like rolling updates.

## Suspension Lifecycle

Suspension allows a component to be intentionally deactivated without deleting its configuration. When `Suspend(true)`
is set on the builder:

1. The component calls `Suspend()` on all `Suspendable` resources.
2. Each resource performs its suspension behavior, typically scaling to zero replicas.
3. The component polls `SuspensionStatus()` on each resource.
4. Once all resources report `Suspended`, the condition transitions to `Suspended`.

Resources that do not yet exist in the cluster are created in their suspended state (with suspension mutations already
applied). For example, a Deployment is created with zero replicas. This ensures the resource is immediately available
when suspension ends.

Resources with `DeleteOnSuspend` enabled are **not** created if they are already absent. Their absence is treated as
already suspended. This avoids a create→delete churn loop on every reconcile while the component remains suspended.

Resources that are not `Suspendable` are left in place.

## ReconcileContext

`ReconcileContext` carries all dependencies for a reconciliation pass. Pass it from your controller on each call:

```go
recCtx := component.ReconcileContext{
    Client:   r.Client,    // sigs.k8s.io/controller-runtime/pkg/client
    Scheme:   r.Scheme,    // *runtime.Scheme
    Recorder: r.Recorder,  // record.EventRecorder
    Metrics:  r.Metrics,   // component.Recorder (condition metrics), optional
    Owner:    owner,       // the CRD that owns this component
}

err = comp.Reconcile(ctx, recCtx)
```

Dependencies are passed explicitly so components remain testable and decoupled from global state.

The `Metrics` field is optional. When set, the framework records Prometheus metrics for every condition reported during
a reconcile. The recorder implementation is provided by
[go-crd-condition-metrics](https://github.com/sourcehawk/go-crd-condition-metrics). Leave the field `nil` to opt out of
metric recording.

## Persisting Status with FlushStatus

`Component.Reconcile` only mutates the owner's status conditions in memory. The controller is responsible for writing
those conditions to the Kubernetes API by calling `component.FlushStatus` once per reconcile, typically from a deferred
call so that conditions set on error paths are still persisted:

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (_ reconcile.Result, err error) {
    owner := &v1alpha1.MyApp{}
    if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    recCtx := component.ReconcileContext{
        Client:   r.Client,
        Scheme:   r.Scheme,
        Recorder: r.Recorder,
        Metrics:  r.Metrics,
        Owner:    owner,
    }
    defer func() {
        if flushErr := component.FlushStatus(ctx, recCtx); flushErr != nil && err == nil {
            err = flushErr
        }
    }()

    comp, err := buildMyComponent(owner)
    if err != nil {
        return reconcile.Result{}, err
    }
    return reconcile.Result{}, comp.Reconcile(ctx, recCtx)
}
```

`FlushStatus` performs one `Status().Update` call that writes every condition currently on the owner in memory, wrapped
in `retry.RetryOnConflict`. If another writer updated the owner between the controller's initial `Get` and this call,
`FlushStatus` refetches, reapplies the conditions staged during the reconcile, and retries. Conditions managed by other
writers on the same owner are preserved because `meta.SetStatusCondition` merges by condition type.

After the update succeeds, `FlushStatus` records metrics for every condition on the owner. If `rec.Metrics` is nil,
metric recording is skipped.

This split is what allows a controller with several components (see [Keep Controllers Thin](./guidelines.md) and
[One Component Per Logical Condition](./guidelines.md)) to stage several conditions during one reconcile and persist
them all in a single write. Persisting after every component would race the components' writes against each other and
produce 409 conflicts.

## Guards

Guards allow resources within a component to express runtime dependencies on each other. A guard is a precondition
function registered on a resource that is evaluated before the resource is applied. If the guard returns `Blocked`, the
resource and all resources registered after it are skipped for that reconciliation cycle.

Combined with per-resource data extraction, guards enable indirect dependency graphs: Resource A is applied first, its
data extractor runs and populates a shared variable, and Resource B's guard checks that variable before allowing B to
proceed.

### Registering a Guard

Guards are registered on the resource builder using `WithGuard`. The guard function receives a copy of the resource
object and returns a `GuardStatusWithReason`.

The following example shows the complete pattern. A cloud provider role resource extracts its ARN after being applied. A
bucket resource uses that ARN in its spec and guards against being applied before the ARN is available:

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, err error) {
    // ...fetch owner and build recCtx...

    defer func() {
        if flushErr := component.FlushStatus(ctx, recCtx); flushErr != nil && err == nil {
            err = flushErr
        }
    }()

    // roleARN is scoped to this reconcile call. The role resource's data extractor
    // populates it after the role is applied. Because extraction runs per-resource
    // (not after all resources), roleARN is set before the bucket's guard evaluates.
    var roleARN string

    comp, err := buildCloudComponent(owner, &roleARN)
    if err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, comp.Reconcile(ctx, recCtx)
}

func buildCloudComponent(owner *v1alpha1.MyApp, roleARN *string) (*component.Component, error) {
    // First resource: the cloud provider role.
    // After it is applied, the data extractor reads the ARN from the object.
    roleRes, err := static.NewBuilder(newCloudRole(owner)).
        WithDataExtractor(func(obj uns.Unstructured) error {
            *roleARN = obj.Object["status"].(map[string]any)["arn"].(string)
            return nil
        }).
        Build()
    if err != nil {
        return nil, err
    }

    // Second resource: the cloud provider bucket.
    // The role's data extractor populates *roleARN earlier in this same reconcile
    // cycle, which causes the guard to clear. The mutation then runs lazily at
    // Mutate() time and injects the now-populated *roleARN into the bucket spec.
    bucketRes, err := static.NewBuilder(newCloudBucket(owner)).
        WithGuard(func(_ uns.Unstructured) (concepts.GuardStatusWithReason, error) {
            if *roleARN == "" {
                return concepts.GuardStatusWithReason{
                    Status: concepts.GuardStatusBlocked,
                    Reason: "waiting for cloud provider role ARN",
                }, nil
            }
            return concepts.GuardStatusWithReason{
                Status: concepts.GuardStatusUnblocked,
            }, nil
        }).
        WithMutation(unstruct.Mutation{
            Name: "set-role-arn",
            Mutate: func(m *unstruct.Mutator) error {
                m.EditContent(func(e *editors.UnstructuredContentEditor) error {
                    return e.SetNestedString(*roleARN, "spec", "roleARN")
                })
                return nil
            },
        }).
        Build()
    if err != nil {
        return nil, err
    }

    // Registration order matters: the role must be registered before the bucket.
    return component.NewComponentBuilder().
        WithName("cloud-resources").
        WithConditionType("CloudResourcesReady").
        WithResource(roleRes, component.ResourceOptions{}).
        WithResource(bucketRes, component.ResourceOptions{}).
        Build()
}
```

The guard function receives the resource's object but is not required to use it. Guards that only check external state
(closure variables populated by prior extractors) can ignore the parameter.

### Guard Behavior

- Guards are evaluated in resource registration order, before each resource is applied.
- When a guard returns `Blocked`, the blocked resource contributes a `Blocked` status to the component condition
  regardless of the resource's participation mode. All resources after it are skipped entirely. This override exists
  because a blocked guard halts the entire pipeline, and subsequent required resources would otherwise be silently
  absent from health aggregation.
- On the next reconciliation cycle, if the guard clears (returns `Unblocked`), the resource is applied normally.
- Guards are **not** evaluated during suspension. The suspension path always proceeds regardless of guard state.
- A guard evaluation error is treated as a reconciliation failure and sets the component condition to `Error`.

### Status Reporting

A blocked guard produces a condition like:

```yaml
type: WebInterfaceReady
status: "False"
reason: Blocked
message: "waiting for cloud provider role ARN"
```

The `Blocked` status is not sticky -- it is self-reinforcing because the guard re-evaluates on every reconcile. When the
guard clears, the status immediately transitions to the next applicable state (e.g., `Creating`).

## Best Practices

**Keep controllers thin.** The controller's job is to fetch the owner CRD, decide which components should exist, and
call `Reconcile` on each. Resource-level logic belongs in the component and its primitives.

**One component per user-visible feature.** If you want a `WebInterfaceReady` and a `DatabaseReady` condition on your
CRD, those are two separate components.

**Group by lifecycle.** Resources that must live and die together belong in the same component. If they have independent
lifecycles, split them.

**Use `ParticipationModeAuxiliary` for non-critical resources.** A metrics exporter sidecar should not block your
primary component from becoming `Ready`. All resource types default to `ParticipationModeRequired`, so set
`ParticipationModeAuxiliary` explicitly when a resource's health should not gate the component condition.
