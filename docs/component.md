# Component System

The `component` package provides a structured way to manage logical features in a Kubernetes operator by grouping related resources into **Components**.

A Component acts as a single behavioral unit: it reconciles multiple resources, manages their shared lifecycle, and reports their aggregate health through one condition on the owner CRD.

## Why Components Exist

In complex operators, reconciliation logic tends to become fragmented:

- Controllers coordinate dozens of unrelated resources in a single function
- Lifecycle logic (rollouts, suspension, degradation) is reimplemented for every feature
- Status reporting varies across features, making it hard to reason about overall health

Components address this by providing a consistent pattern: one component per logical feature, one condition per component.

## Building a Component

Components are constructed through a builder. The builder collects resource registrations, configuration, and lifecycle flags, then produces an immutable `Component` ready for reconciliation.

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

| Option                                                           | Behavior                                                                                                                                  |
|------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| `ResourceOptions{}` (default)                                    | **Managed** — created or updated; health contributes to condition                                                                         |
| `ResourceOptions{ReadOnly: true}`                                | **Read-only** — fetched but never modified; health still contributes                                                                      |
| `ResourceOptions{Delete: true}`                                  | **Delete-only** — removed from the cluster if present; does not contribute to health                                                      |
| `ResourceOptions{ParticipationMode: ParticipationModeAuxiliary}` | The resource's health does not contribute to the component condition — the component can become Ready regardless of this resource's state |

## Reconciliation Lifecycle

`comp.Reconcile(ctx, recCtx)` runs a six-phase process on every call:

**Phase 1 — Suspension check**
If the component is marked suspended, it calls `Suspend()` on all managed resources that support suspension (create/update resources, not read-only ones), updates the condition, then processes any pending deletions and returns. The remaining phases are skipped.

**Phase 2 — Resource synchronization**
All managed resources are created or updated to match their desired state.

**Phase 3 — Read-only resource fetching**
Read-only resources are fetched from the cluster so their current state is available for health evaluation.

**Phase 4 — Data extraction**
Any resource implementing `DataExtractable` has `ExtractData()` called to harvest data from the synchronized cluster state before condition evaluation.

**Phase 5 — Status aggregation and condition update**
The health of each resource is collected, the grace period is consulted, and a single aggregate condition is written to the owner object's status.

**Phase 6 — Resource deletion**
Resources registered for deletion are removed from the cluster.

## Status Model

The status values a component reports depend on which lifecycle interfaces its resources implement. The component aggregates across all registered resources and surfaces the most critical state.

### Alive Resources (`Alive` interface)

Reported by long-running workloads (Deployments, StatefulSets, DaemonSets):

| State      | Meaning                                                  |
|------------|----------------------------------------------------------|
| `Healthy`  | The resource has reached its desired state               |
| `Creating` | The resource is being provisioned for the first time     |
| `Updating` | The resource is being modified with new configuration    |
| `Scaling`  | The resource is changing its replica count               |
| `Failing`  | The resource is failing to converge to its desired state |

### Completable Resources (`Completable` interface)

Reported by run-to-completion resources (Jobs, tasks):

| State         | Meaning                             |
|---------------|-------------------------------------|
| `Completed`   | The resource finished successfully  |
| `TaskRunning` | The resource is currently executing |
| `TaskPending` | The resource is waiting to start    |
| `TaskFailing` | The resource finished with an error |

### Operational Resources (`Operational` interface)

Reported by integration resources whose readiness depends on external systems (Services, Ingresses, Gateways, CronJobs):

| State              | Meaning                                           |
|--------------------|---------------------------------------------------|
| `Operational`      | The resource is fully operational                 |
| `OperationPending` | The resource is waiting on an external dependency |
| `OperationFailing` | The resource failed to reach an operational state |

### Static Resources (no interface)

Resources that implement none of the above interfaces are considered ready as long as they exist in the cluster.

### Grace States

When a component has a grace period configured and a `Graceful` resource has not reached its target state within that period, the `Graceful` interface determines the post-expiry severity:

| State      | Meaning                                                                            |
|------------|------------------------------------------------------------------------------------|
| `Healthy`  | The resource is healthy (grace period expired without issue)                       |
| `Degraded` | The resource is partially functional or convergence is taking longer than expected |
| `Down`     | The resource is completely non-functional                                          |

### Suspension States

Reported during intentional deactivation:

| State               | Meaning                                                |
|---------------------|--------------------------------------------------------|
| `PendingSuspension` | Suspension is acknowledged but has not started         |
| `Suspending`        | Resources are actively being scaled down or cleaned up |
| `Suspended`         | All resources have reached their suspended state       |

### Condition Priority

When aggregating across multiple resources, the most critical state wins:

1. `Error` / `Down` / `Degraded` — something is wrong
2. Suspension states — the component is intentionally inactive
3. Converging states (`Creating`, `Updating`, `Scaling`, `TaskRunning`, `TaskPending`, `OperationPending`) — the component is progressing
4. `Healthy` / `Completed` / `Operational` — all resources are in their target state

## Grace Period

The grace period defines how long a component may remain in a converging state (`Creating`, `Updating`, `Scaling`) before transitioning to `Degraded` or `Down`.

```go
component.NewComponentBuilder().
    WithGracePeriod(5 * time.Minute).
    // ...
```

During the grace period the component reports its real converging state, not a failure. After the period expires, if the component is still not `Ready`, the framework escalates to `Degraded` or `Down` based on resource health.

This prevents spurious failure alerts during normal operations like rolling updates.

## Suspension Lifecycle

Suspension allows a component to be intentionally deactivated without deleting its configuration. When `Suspend(true)` is set on the builder:

1. The component calls `Suspend()` on all `Suspendable` resources.
2. Each resource performs its suspension behavior — typically scaling to zero replicas.
3. The component polls `SuspensionStatus()` on each resource.
4. Once all resources report `Suspended`, the condition transitions to `Suspended`.

Resources that do not yet exist in the cluster are created in their suspended state (with suspension mutations already applied). For example, a Deployment is created with zero replicas. This ensures the resource is immediately available when suspension ends.

Resources with `DeleteOnSuspend` enabled (e.g., DaemonSets) are **not** created if they are already absent — their absence is treated as already suspended. This avoids a create→delete churn loop on every reconcile while the component remains suspended.

Resources that are not `Suspendable` are left in place.

## ReconcileContext

`ReconcileContext` carries all dependencies for a reconciliation pass. Pass it from your controller on each call:

```go
recCtx := component.ReconcileContext{
    Client:   r.Client,    // sigs.k8s.io/controller-runtime/pkg/client
    Scheme:   r.Scheme,    // *runtime.Scheme
    Recorder: r.Recorder,  // record.EventRecorder
    Owner:    owner,       // the CRD that owns this component
}

err = comp.Reconcile(ctx, recCtx)
```

Dependencies are passed explicitly so components remain testable and decoupled from global state.

## Best Practices

**Keep controllers thin.** The controller's job is to fetch the owner CRD, decide which components should exist, and call `Reconcile` on each. Resource-level logic belongs in the component and its primitives.

**One component per user-visible feature.** If you want a `WebInterfaceReady` and a `DatabaseReady` condition on your CRD, those are two separate components.

**Group by lifecycle.** Resources that must live and die together belong in the same component. If they have independent lifecycles, split them.

**Use `ParticipationModeAuxiliary` for non-critical resources.** A metrics exporter sidecar should not block your primary component from becoming `Ready`. All resource types default to `ParticipationModeRequired` — set `ParticipationModeAuxiliary` explicitly when a resource's health should not gate the component condition.
