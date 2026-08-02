---
name: building-components
description:
  Use when creating or modifying a component built with the operator-component-framework
  (github.com/sourcehawk/operator-component-framework) - covers the component builder, resource registration, feature
  gates, prerequisites, the reconciliation lifecycle, conditions and the status model, grace periods, suspension,
  ReconcileContext, FlushStatus, guards, and declared data cells.
---

# Building Components

## What a component is

A **Component** groups related Kubernetes resources into one behavioral unit. It reconciles those resources, manages
their shared lifecycle (feature gating, prerequisites, suspension, grace periods, guards), and reports their aggregate
health through a single condition on the owner CRD.

Layering: `Controller` -> `Component` (one condition on the owner) -> `Resource Primitive` (Deployment, ConfigMap,
Service, ...) -> `Kubernetes Object`. A controller wires together several components; each component owns exactly one
condition type and one ordered list of resources.

## Building a component

Components are constructed through `component.NewComponentBuilder()`. `Build()` requires `WithName` and
`WithConditionType`; omitting either is a validation error aggregated with `errors.Join` and returned from `Build()`.
Resources are added with `WithResource`. The owner CRD itself is not passed to the builder: it flows into reconciliation
later through `ReconcileContext.Owner`, and resource constructors typically close over it when building the
desired-state object passed to `WithResource`.

```go
comp, err := component.NewComponentBuilder().
    WithName("frontend").
    WithConditionType("FrontendReady").
    WithFeatureGate(frontendFeature).                       // optional: disable to remove all resources
    WithPrerequisite(component.DependsOn("BackendReady")).  // optional: wait for another component
    WithResource(frontendConfig, component.ReadOnly()).
    WithResource(frontendDeployment).
    WithResource(frontendService).
    WithResource(legacyService, component.Delete()).
    WithGracePeriod(5 * time.Minute).
    Suspend(owner.Spec.Suspended).
    Build()
if err != nil {
    return err
}
```

Each `WithResource` call accepts `ResourceOption` values: `component.ReadOnly()`, `component.Delete()` /
`component.DeleteWhen(cond)`, `component.GatedBy(gate)`, `component.OrphanWhen(cond)`, `component.Unowned()`,
`component.Auxiliary()`, `component.BlockOnAbsence()`, `component.IgnoreIfAbsent()`. With no options a resource is
**Managed**: applied via Server-Side Apply, required for the condition. `ReadOnly()` is mutually exclusive with the
deletion and gating options. See `references/component.md` for the full option matrix and `IncludeWhen` vs. `GatedBy`.

## Registration order is execution order

Resources reconcile sequentially in the order they were registered with `WithResource`. A resource registered earlier
can extract a value into a data cell that a later resource's data guard or mutation reads; the reverse never works, and
for declared data `Build()` rejects a registration order that cannot work (see Declared data below). Resources
registered for deletion (`Delete()`, `DeleteWhen()`, or all managed resources when a feature gate is disabled) are
removed from the cluster in that same registration order in the final reconciliation step; the framework does not
reverse it. Design registration order around real dependencies, not convenience.

## Feature gates and prerequisites

A component-level feature gate (`WithFeatureGate`) controls whether the component is active at all. When disabled, the
component deletes every resource it manages and reports condition status `True` with reason `Disabled`. If the gate's
`Enabled()` call itself errors, the component reports reason `FeatureGateError` instead, distinguishing a
gate-evaluation failure from a normal disabled state.

Prerequisites (`WithPrerequisite`, satisfied by the `Prerequisite` interface or the built-in `component.DependsOn`
helper) are initialization barriers, not ongoing health checks. They answer "can this component be created?", not
"should this component keep running?". The barrier is active only while the component's condition reason is `Unknown`,
`PrerequisiteNotMet`, `Disabled`, or `FeatureGateError`; once the reason becomes anything else the barrier is
permanently passed and never re-evaluated, even if the dependency later becomes unhealthy. Multiple prerequisites are
checked in registration order and the first unmet one short-circuits the rest, setting condition status `False` with
reason `PrerequisiteNotMet`. The feature gate check always runs first: a disabled gate skips prerequisite evaluation
entirely.

## Reconciliation lifecycle

`comp.Reconcile(ctx, recCtx)` runs these steps, in order, every call. Before step 1, every declared data cell is
cleared, so no value extracted during a previous reconcile can be observed during this one.

1. **Feature gate check.** Disabled -> delete all managed resources, condition `True/Disabled`. Gate error ->
   `FeatureGateError`, no further steps.
2. **Prerequisite check.** Only while the initialization barrier is active. Unmet -> condition
   `False/PrerequisiteNotMet`, no resources touched.
3. **Suspension check.** If suspended, `Suspend()` runs on managed resources, the condition reflects suspension
   progress, pending deletions are processed, and reconciliation stops there. Guards are not evaluated during
   suspension.
4. **Resource reconciliation.** Non-delete resources are processed sequentially in registration order: each resource's
   guard (if any) is checked, a blocked guard halts that resource and every later one, then the resource is applied
   (managed) or fetched (read-only), and its declared data extractions run immediately so later resources' data guards
   and mutations can see the extracted values.
5. **Status aggregation.** The converging status of every processed resource (including a blocked-guard result) is
   collected.
6. **Condition update.** A new condition is derived from the aggregate status, the previous condition, and the grace
   period, then written to the owner **in memory only**. `Reconcile` never calls the Kubernetes status API.
7. **Resource deletion.** Resources registered for deletion are removed from the cluster.

Every resource defaults to `ParticipationModeRequired`: its health must reach a ready state for the component condition
to go `True`. Register a resource with `component.Auxiliary()` to exclude its health from aggregation (a blocked guard
on it still halts the pipeline and still contributes, because a blocked guard stops everything after it).

## Status model

A component reports one condition whose reason is a `component.Status` value. Reachable states depend on which lifecycle
interface a resource implements: long-running workloads report `Alive` states (`Creating`, `Updating`, `Scaling`,
`Healthy`, `AliveFailing`), run-to-completion resources report `Completable` states (`CompletionPending`,
`CompletionRunning`, `Completed`, `CompletionFailing`), externally-dependent resources report `Operational` states
(`OperationPending`, `Operational`, `OperationFailing`), and resources implementing none of these are ready as long as
they exist. When a component aggregates several resources into one condition, `Status.Priority()` picks the
highest-priority reason: `Error` and `FeatureGateError` outrank everything, then grace-expired states (`Down`,
`Degraded`), then suspension states (`PendingSuspension`, `Suspending`, `Suspended`), then `Disabled`, then the various
failing/converging/pending states, then the ready states (`Healthy`, `Operational`, `Completed`) at the bottom.
`Unknown` and any unrecognized reason are priority `0` and never influence aggregation. A resource registered with
`component.Auxiliary()` does not contribute its converging health to this aggregation, but a blocked guard on it still
does.

`Reconcile` only stages the condition on the in-memory owner object; it is not the writer of record to the cluster.

## Grace period and suspension

`WithGracePeriod` defines how long a component may remain in a converging state (`Creating`, `Updating`, `Scaling`)
before escalating. This is a **convergence-time budget, not an error budget**: during the grace period the component
reports its real converging state, not a failure, so rolling updates and normal scale-ups do not trip false alerts. Once
the period expires and the component is still not ready, a `Graceful` resource's `GraceStatus()` determines the
post-expiry severity: `Healthy` (no issue), `Degraded` (partially functional), or `Down` (non-functional). Exceeding the
grace period does not by itself mean failure; it means the component's own `GraceStatus()` is now consulted to decide
whether the still-not-ready state is degraded, down, or actually fine.

Suspension (`Suspend(true)` on the builder) intentionally deactivates a component without deleting its configuration.
The component calls `Suspend()` on every `Suspendable` resource, polls `SuspensionStatus()`, and progresses the
condition through `PendingSuspension` -> `Suspending` -> `Suspended` (all condition status `True`). While the
prerequisite barrier is active, suspension is a no-op, since no resources exist yet to suspend. Resources not yet in the
cluster are created directly in their suspended state (for example, a Deployment created with zero replicas), so they
are ready the instant suspension ends.

## Declared data

Resources inside one component pass observed values to each other through typed, presence-aware **data cells**, created
in the component assembly function with `concepts.NewData[T]("name")`. The producer declares an extraction with the
primitive package's `ExtractInto` function (package-level, because a Go method cannot introduce the value type
parameter); it runs immediately after the resource is applied (managed) or fetched (read-only) and stores the result in
the cell. Consumers declare their reads on the builder: `WithDataGuard(cells...)` blocks the resource until every listed
cell is set, `WithOptionalData(cells...)` never gates and suits mutations that enrich the object only when the value is
there. Read a cell with `Require()` (errors with `concepts.ErrDataNotExtracted` when unset) or `Get()` (value plus
presence).

```go
dbHost := concepts.NewData[string]("db-host")

cmBuilder := configmap.NewBuilder(dbConfig(app))
configmap.ExtractInto(cmBuilder, dbHost, func(cm corev1.ConfigMap) (string, error) {
    return cm.Data["db-host"], nil
})
```

`Build()` validates the topology: every cell a resource reads must have a producer registered **strictly earlier** (a
producer never satisfies its own read, since extraction runs after mutations on the same resource), and no two distinct
cells may share a name. Optional reads are validated too, so declare them even though they never gate. Sharing a cell
across components is unsupported; validation and the reconcile-start reset are per component. The built component
reports the declared flow through `DataTopology()` (`concepts.DataInspector`) without running any extraction.

Two caveats. During suspension, declared extractions still run for managed resources, but cells produced by read-only
resources or by resources with `DeleteOnSuspend` stay absent, so a mutation depending on one of those must use `Get()`
rather than `Require()` if the component can ever be suspended. `Preview()` runs no extraction either, so a mutation
calling `Require()` fails the preview unless the test seeds the cell with `Set` first; return cells from the assembly
function so tests can reach them.

## Guards and ReconcileContext

A guard is a precondition evaluated before a resource is applied. If it reports `GuardStatusBlocked`, that resource and
every resource registered after it are skipped for the cycle, and the condition reports status `False` with reason
`Blocked`. A guard evaluation error is treated as a reconciliation failure (`Error`). Guards are not evaluated during
suspension.

There are two forms. A **data guard**, declared with `WithDataGuard(cells...)`, blocks until every listed data cell
holds a value; the framework generates both the guard and its reason (`waiting for data "db-host"`), so the message a
user reads cannot drift from the real dependency. A **custom guard**, registered with `WithGuard`, receives a copy of
the resource's object and returns a `concepts.GuardStatusWithReason` from arbitrary logic; keep it for preconditions
that are not "a value exists", such as a status phase reaching a specific value. A resource may use both: data guards
are evaluated first, and the custom guard is consulted only once every guarded cell is set.

`ReconcileContext` carries everything a reconcile pass needs: `Client`, `Scheme`, `Recorder`, an optional `Metrics`
recorder, and `Owner` (the CRD instance that owns the component). Build one per reconcile from your controller and pass
it into `comp.Reconcile(ctx, recCtx)`.

`Component.Reconcile` mutates the owner's status conditions only in memory. The controller persists them by calling
`component.FlushStatus(ctx, recCtx)` once per reconcile, typically deferred so conditions set on error paths are still
written. `FlushStatus` performs a single `Status().Update`, wrapped in `retry.RetryOnConflict`, that writes every
condition currently staged on the owner; conditions owned by other writers on the same object are preserved because
`meta.SetStatusCondition` merges by condition type. This split lets a controller with several components stage several
conditions in one reconcile and persist them in one write, instead of racing a write per component.

## Anti-patterns

- Putting version-dependent values directly in a resource's desired state instead of behind a versioned mutation makes
  the resource diverge unpredictably across cluster versions; gate the value with a mutation instead.
- Giving one component more than one logical condition collapses distinct failure modes into a single status, hiding
  which underlying concern actually broke; split it into one component per condition instead.
- Registering resources without regard to their real dependency order breaks guards and data flow that assume an earlier
  resource already ran. `Build()` turns the mistake into an error where the flow is declared with data cells, but custom
  guards still break silently; registration order must match the actual dependency order.

## Ground truth

The consumer's resolved module version is the source of truth, not these docs. Before asserting an exact signature,
method name, or option:

1. Read the framework version from the consumer's `go.mod` entry for
   `github.com/sourcehawk/operator-component-framework`.
2. Verify the symbol with `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

The reference files bundled with this skill match the framework version this plugin shipped with. When they disagree
with `go doc`, `go doc` wins.

## References

- `references/component.md`: full component documentation. Read when you need exact builder signatures, status
  constants, lifecycle phase details, or guard semantics.
