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

## Reconciliation Lifecycle

`comp.Reconcile(ctx, recCtx)` runs a six-phase process on every call:

**Phase 1: Suspension check.** If the component is marked suspended, it calls `Suspend()` on all managed resources that
support suspension (create/update resources, not read-only ones), updates the condition, then processes any pending
deletions and returns. The remaining phases are skipped.

**Phase 2: Resource synchronization.** All managed resources are created or updated to match their desired state. Each
resource gets a controller owner reference pointing to the owner CRD, unless the resource is cluster-scoped and the
owner is namespace-scoped, in which case the reference is automatically skipped (see
[Cluster-Scoped Resources](#cluster-scoped-resources)).

**Phase 3: Read-only resource fetching.** Read-only resources are fetched from the cluster so their current state is
available for health evaluation.

**Phase 4: Data extraction.** Any resource implementing `DataExtractable` has `ExtractData()` called to harvest data
from the synchronized cluster state before condition evaluation.

**Phase 5: Status aggregation and condition update.** The health of each resource is collected, the grace period is
consulted, and a single aggregate condition is written to the owner object's status.

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

Resources that implement none of the above interfaces are considered ready as long as they exist in the cluster.

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

### Condition Priority

When aggregating across multiple resources, the most critical state wins:

1. `Error` / `Down` / `Degraded`: something is wrong
2. Suspension states: the component is intentionally inactive
3. Converging states (`Creating`, `Updating`, `Scaling`, `TaskRunning`, `TaskPending`, `OperationPending`): the
   component is progressing
4. `Healthy` / `Completed` / `Operational`: all resources are in their target state

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
