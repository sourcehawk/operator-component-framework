# Component System

The `component` package provides a structured way to manage logical features in a Kubernetes operator by grouping
related resources into **Components**.

A Component acts as a single behavioral unit: it reconciles multiple resources, manages their shared lifecycle, and
reports their aggregate health through one condition on the owner CRD.

## Why Components Exist

In complex operators, the same reconciliation patterns get reimplemented for every feature. Each controller coordinates
its own resources, manages its own lifecycle (rollout, suspension, degradation), and reports status in its own way. The
logic is duplicated but never identical, because there is no shared structure enforcing consistency.

Most teams do try to organize. Resource construction moves into `pkg/` and concerns get split into separate files:

```
controllers/
├── frontend_controller.go        # Orchestrates create/update/delete/suspend/status for frontend
└── backend_controller.go         # Orchestrates create/update/delete/suspend/status for backend
pkg/
├── frontend/
│   ├── deployment.go             # Constructs the Deployment
│   ├── service.go                # Constructs the Service
│   └── resources.go              # Wires resources together
└── backend/
    ├── deployment.go
    └── configmap.go
```

This moves files around but doesn't change the underlying problem. Each controller still reimplements the same lifecycle
patterns in slightly different ways. Version-specific behavior and feature flags compound things further: a probe format
changes in v1.3, so `pkg/frontend/deployment.go` gains an `if version < "1.3"` branch. A tracing sidecar is
feature-gated, so that lands in the same file, or the controller, or a new `features.go`, wherever the last author
decided. Conditional logic accumulates until the only way to know what a resource actually looks like is to run the
operator and inspect the output.

The component model replaces this with a layout where each concern has exactly one home:

```
controllers/
├── frontend_controller.go        # Builds components, calls Reconcile
└── backend_controller.go
pkg/components/
├── web-interface/
│   ├── component.go              # Assembles primitives into a component
│   ├── resources/
│   │   ├── deployment.go         # Baseline Deployment definition
│   │   └── service.go            # Baseline Service definition
│   └── features/
│       ├── tracing.go            # Mutation: adds tracing sidecar
│       ├── tracing_test.go
│       ├── legacy_probes.go      # Mutation: version-gated probe adjustment
│       └── legacy_probes_test.go
└── api-server/
    ├── component.go
    ├── resources/
    │   ├── deployment.go
    │   └── configmap.go
    └── features/
        ├── rate_limiting.go
        └── rate_limiting_test.go
```

Lifecycle behavior (rollout, suspension, status reporting) is handled by the framework, so controllers no longer
reimplement it independently. Version-specific behavior and feature flags are expressed as isolated mutations, each in
its own file with its own tests, rather than conditional branches layered into resource definitions. The baseline
definition for each resource is always the canonical desired state, readable on its own without tracing through every
mutation that might apply to it.

## Building a Component

Components are constructed through a builder. The builder collects resource registrations, configuration, and lifecycle
flags, then produces an immutable `Component` ready for reconciliation.

```go
comp, err := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
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

| Option                                                           | Behavior                                                                                                                                 |
| ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `ResourceOptions{}` (default)                                    | **Managed**: created or updated; health contributes to condition                                                                         |
| `ResourceOptions{ReadOnly: true}`                                | **Read-only**: fetched but never modified; health still contributes                                                                      |
| `ResourceOptions{Delete: true}`                                  | **Delete-only**: removed from the cluster if present; does not contribute to health                                                      |
| `ResourceOptions{ParticipationMode: ParticipationModeAuxiliary}` | The resource's health does not contribute to the component condition. The component can become Ready regardless of this resource's state |

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

| Method                            | Effect                                                                                                                          |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `WithFeatureGate(f feature.Gate)` | Gates the resource on a feature. When disabled, the resource is deleted.                                                        |
| `WithTruth(truth bool)`           | Adds a boolean condition (AND logic). If any truth is false, the resource is deleted. Calls are additive.                       |
| `Auxiliary()`                     | Sets participation mode to `Auxiliary` (resource does not affect component health).                                             |
| `ReadOnly()`                      | Marks the resource as read-only. If the resource is also gated by a disabled feature, deletion takes precedence over read-only. |

For the common case of gating a resource on a single feature, use the convenience function:

```go
opts, err := component.ResourceOptionsFor(tracingFeature)
```

**Resolution rules:**

1. If the feature is non-nil and evaluates to disabled, the resource is deleted.
2. If any truth condition is false, the resource is deleted.
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

## Reconciliation Lifecycle

`comp.Reconcile(ctx, recCtx)` runs a six-phase process on every call:

**Phase 1: Suspension check.** If the component is marked suspended, it calls `Suspend()` on all managed resources that
support suspension (create/update resources, not read-only ones), updates the condition, then processes any pending
deletions and returns. The remaining phases are skipped.

**Phase 2: Resource reconciliation.** All non-delete resources are processed sequentially in registration order,
regardless of whether they are managed or read-only. For each resource:

1. If the resource has a [guard](#guards), the guard is evaluated first. If blocked, the resource and all subsequent
   resources are skipped.
2. The resource is either applied to the cluster (managed) or fetched from it (read-only). Managed resources use
   Server-Side Apply and get a controller owner reference pointing to the owner CRD, unless the resource is
   cluster-scoped and the owner is namespace-scoped (see [Cluster-Scoped Resources](#cluster-scoped-resources)).
3. If the resource implements `DataExtractable`, its data extractors run immediately. This makes extracted data
   available to subsequent resources' guards and mutations within the same reconciliation cycle.

This means a read-only resource registered before a managed resource can extract data that feeds into the managed
resource's guard or mutations.

**Phase 3: Status aggregation and condition update.** The health of each resource is collected, the grace period is
consulted, and a single aggregate condition is written to the owner object's status.

**Phase 4: Resource deletion.** Resources registered for deletion are removed from the cluster.

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

### Condition Priority

When aggregating across multiple resources, the most critical state wins:

1. `Error` / `Down` / `Degraded`: something is wrong
2. Suspension states: the component is intentionally inactive
3. `Blocked`: a resource is blocked on a guard precondition
4. Converging states (`Creating`, `Updating`, `Scaling`, `TaskRunning`, `TaskPending`, `OperationPending`): the
   component is progressing
5. `Healthy` / `Completed` / `Operational`: all resources are in their target state

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
    Metrics:  r.Metrics,   // component.Recorder (condition metrics)
    Owner:    owner,       // the CRD that owns this component
}

err = comp.Reconcile(ctx, recCtx)
```

Dependencies are passed explicitly so components remain testable and decoupled from global state.

The `Metrics` field is required. The framework records Prometheus metrics for every condition state transition during
reconciliation. The recorder implementation is provided by
[go-crd-condition-metrics](https://github.com/sourcehawk/go-crd-condition-metrics).

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
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ...fetch owner...

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
- When a guard returns `Blocked`, the blocked resource contributes a `Blocked` status to the component condition. All
  resources after it are skipped entirely.
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
