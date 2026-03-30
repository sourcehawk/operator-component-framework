# Custom Resource Implementation Guide

This guide explains how to implement custom resource wrappers for Kubernetes objects not covered by the
[built-in primitives](primitives.md). The framework provides a set of generic building blocks in `pkg/generic` that
handle reconciliation mechanics, mutation sequencing, suspension, and data extraction. Your custom resource wraps these
generics with type-specific logic.

---

## When to Implement a Custom Resource

The built-in primitives cover common Kubernetes types (Deployments, ConfigMaps, Services, etc.) and are highly
customizable: you can override status handlers, suspension logic, mutation behavior, and data extractors without leaving
the primitive API. Implement a custom resource when the Kubernetes type you need to manage has no corresponding built-in
primitive:

- A **custom CRD** defined by your project or a third-party operator
- A **standard Kubernetes type** not yet covered by the built-in set

---

## Architecture

A custom resource implementation consists of three pieces:

```
Builder            → configures and validates the resource
  └─ Resource      → thin wrapper delegating to a generic resource
       └─ Mutator  → records and applies mutations to the Kubernetes object
```

Each piece wraps a corresponding generic type from `pkg/generic`:

| Your Type  | Wraps                                             |
| ---------- | ------------------------------------------------- |
| `Builder`  | `generic.WorkloadBuilder[T, *Mutator]` (or other) |
| `Resource` | `generic.WorkloadResource[T, *Mutator]`           |
| `Mutator`  | Implements `generic.FeatureMutator`               |

---

## Choosing a Resource Category

The framework defines four resource categories. Each maps to a generic resource type with different lifecycle
interfaces:

| Category        | Generic Type                  | Lifecycle Interfaces                                                     | Use When                                               |
| --------------- | ----------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------ |
| **Workload**    | `generic.WorkloadResource`    | `Alive`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable`       | Long-running processes with replica-based health       |
| **Static**      | `generic.StaticResource`      | `Guardable`, `DataExtractable`                                           | Configuration objects with no runtime health semantics |
| **Task**        | `generic.TaskResource`        | `Completable`, `Suspendable`, `Guardable`, `DataExtractable`             | Run-to-completion workloads                            |
| **Integration** | `generic.IntegrationResource` | `Operational`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable` | External dependency objects (services, ingresses)      |

Pick the category that matches your CRD's lifecycle. A CRD that manages a long-running process and needs health tracking
is a **Workload**. A CRD that represents static configuration is **Static**. The rest of this guide uses Workload as the
primary example; the pattern is identical for other categories with fewer handlers to implement.

For a detailed description of each lifecycle interface and its status values, see [Resource Primitives](primitives.md).

---

## Step-by-Step Implementation

The following sections walk through implementing a custom resource for a hypothetical `GameServer` CRD
(`gameservers.example.io/v1`). This CRD manages a long-running game server process, making it a workload resource.

### 1. Define the Mutation Type Alias

Create a type alias for `feature.Mutation` parameterized on your mutator type. This gives callers a clean name to use
when defining feature mutations.

```go
package gameserver

import "github.com/sourcehawk/operator-component-framework/pkg/feature"

// Mutation defines a feature-gated mutation applied to a GameServer resource.
type Mutation = feature.Mutation[*Mutator]
```

### 2. Implement the Mutator

The mutator is responsible for recording mutation intent and applying it in a single controlled pass. It must implement
`generic.FeatureMutator`:

```go
type FeatureMutator interface {
    Apply() error
    NextFeature()
}
```

`Apply()` executes all recorded mutations against the underlying object. `NextFeature()` advances to a new feature scope
The framework calls it between each registered mutation to maintain per-feature ordering boundaries.

#### The Plan-and-Apply Pattern

Mutator methods **record intent** rather than modifying the object directly. The framework calls `Apply()` once after
all mutations have been recorded. This pattern ensures:

- Mutations are applied in a single controlled pass
- Feature boundaries are preserved via `NextFeature()`
- Multiple mutations targeting the same fields resolve predictably

Here is a mutator for the `GameServer` CRD:

```go
package gameserver

import (
    examplev1 "example.io/api/v1"
)

// featurePlan groups all mutation operations recorded by a single feature.
type featurePlan struct {
    replicaOps []func(*examplev1.GameServerSpec)
    configOps  []func(*examplev1.GameServerSpec)
}

// Mutator records mutation intent for a GameServer and applies changes in one pass.
//
// It maintains feature boundaries: each feature's mutations are planned together
// and applied in the order the features were registered.
type Mutator struct {
    current *examplev1.GameServer

    plans  []featurePlan
    active *featurePlan
}

// NewMutator creates a new Mutator for the given GameServer.
// The constructor creates the initial feature scope, so mutations can be
// registered immediately.
func NewMutator(current *examplev1.GameServer) *Mutator {
    m := &Mutator{current: current}
    m.NextFeature()
    return m
}

// NextFeature advances to a new feature planning scope. All subsequent mutation
// registrations are grouped into this scope until NextFeature is called again.
//
// The first scope is created automatically by NewMutator. The framework calls
// this method between mutations to maintain per-feature ordering semantics.
func (m *Mutator) NextFeature() {
    m.plans = append(m.plans, featurePlan{})
    m.active = &m.plans[len(m.plans)-1]
}

// SetMaxPlayers records intent to set the maximum player count.
func (m *Mutator) SetMaxPlayers(count int32) {
    m.active.configOps = append(m.active.configOps, func(spec *examplev1.GameServerSpec) {
        spec.MaxPlayers = count
    })
}

// SetReplicas records intent to set the replica count.
func (m *Mutator) SetReplicas(replicas int32) {
    m.active.replicaOps = append(m.active.replicaOps, func(spec *examplev1.GameServerSpec) {
        spec.Replicas = &replicas
    })
}

// Apply executes all recorded mutations against the GameServer.
// Features are applied in registration order. Within each feature,
// replica operations are applied before config operations.
func (m *Mutator) Apply() error {
    for _, plan := range m.plans {
        for _, op := range plan.replicaOps {
            op(&m.current.Spec)
        }
        for _, op := range plan.configOps {
            op(&m.current.Spec)
        }
    }

    return nil
}
```

#### Mutator Design Guidelines

- **Record, don't mutate.** Methods like `SetMaxPlayers` append to the active feature plan. They do not touch `current`
  directly.
- **Scope per feature.** `NextFeature()` creates a new plan scope. The framework calls it between each registered
  mutation so that each feature's operations are grouped and applied in registration order. `Apply()` iterates over
  plans sequentially, giving each feature a consistent view of the object as modified by all previous features.
- **Keep it typed.** Expose domain-specific methods (`SetMaxPlayers`, `SetReplicas`) rather than generic ones. This
  makes feature mutations self-documenting and prevents callers from bypassing the plan-and-apply sequence.

### 3. Implement Status Handlers

Status handlers translate your CRD's runtime state into framework status types. Which handlers you need depends on your
resource category.

#### Workload Handlers

A workload resource requires a convergence status handler. `Build()` returns an error if it is not set. All other
handlers (grace, suspension status, suspension mutation, delete-on-suspend) default to safe no-ops at the generic layer:
grace defaults to Healthy, suspension status to Suspended, suspension mutation is a no-op, and delete-on-suspend returns
false. Register custom handlers only when your CRD needs domain-specific behavior:

```go
package gameserver

import (
    "fmt"

    "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
    examplev1 "example.io/api/v1"
)

// DefaultConvergingStatusHandler reports whether the GameServer has reached its desired state.
func DefaultConvergingStatusHandler(
    op concepts.ConvergingOperation, gs *examplev1.GameServer,
) (concepts.AliveStatusWithReason, error) {
    desired := int32(1)
    if gs.Spec.Replicas != nil {
        desired = *gs.Spec.Replicas
    }

    if gs.Status.ReadyReplicas == desired {
        return concepts.AliveStatusWithReason{
            Status: concepts.AliveConvergingStatusHealthy,
            Reason: "All replicas are ready",
        }, nil
    }

    var status concepts.AliveConvergingStatus
    switch op {
    case concepts.ConvergingOperationCreated:
        status = concepts.AliveConvergingStatusCreating
    case concepts.ConvergingOperationUpdated:
        status = concepts.AliveConvergingStatusUpdating
    default:
        status = concepts.AliveConvergingStatusScaling
    }

    return concepts.AliveStatusWithReason{
        Status: status,
        Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", gs.Status.ReadyReplicas, desired),
    }, nil
}

// DefaultGraceStatusHandler reports degraded or down state during convergence.
func DefaultGraceStatusHandler(gs *examplev1.GameServer) (concepts.GraceStatusWithReason, error) {
    if gs.Status.ReadyReplicas > 0 {
        return concepts.GraceStatusWithReason{
            Status: concepts.GraceStatusDegraded,
            Reason: "GameServer partially available",
        }, nil
    }

    return concepts.GraceStatusWithReason{
        Status: concepts.GraceStatusDown,
        Reason: "No replicas are ready",
    }, nil
}

// DefaultSuspensionStatusHandler reports whether the GameServer has been suspended.
func DefaultSuspensionStatusHandler(
    gs *examplev1.GameServer,
) (concepts.SuspensionStatusWithReason, error) {
    if gs.Status.Replicas == 0 {
        return concepts.SuspensionStatusWithReason{
            Status: concepts.SuspensionStatusSuspended,
            Reason: "GameServer scaled to zero",
        }, nil
    }

    return concepts.SuspensionStatusWithReason{
        Status: concepts.SuspensionStatusSuspending,
        Reason: fmt.Sprintf("%d replicas still running", gs.Status.Replicas),
    }, nil
}

// DefaultSuspendMutationHandler scales the GameServer to zero replicas.
func DefaultSuspendMutationHandler(m *Mutator) error {
    m.SetReplicas(0)
    return nil
}

// DefaultDeleteOnSuspendHandler returns false: keep the resource, just scale down.
func DefaultDeleteOnSuspendHandler(_ *examplev1.GameServer) bool {
    return false
}
```

#### Status Constants Reference

| Category              | Status Type                      | Constant Name                   | String Value        |
| --------------------- | -------------------------------- | ------------------------------- | ------------------- |
| Workload              | `concepts.AliveConvergingStatus` | `AliveConvergingStatusHealthy`  | `Healthy`           |
|                       |                                  | `AliveConvergingStatusCreating` | `Creating`          |
|                       |                                  | `AliveConvergingStatusUpdating` | `Updating`          |
|                       |                                  | `AliveConvergingStatusScaling`  | `Scaling`           |
|                       |                                  | `AliveConvergingStatusFailing`  | `Failing`           |
| Workload, Integration | `concepts.GraceStatus`           | `GraceStatusHealthy`            | `Healthy`           |
|                       |                                  | `GraceStatusDegraded`           | `Degraded`          |
|                       |                                  | `GraceStatusDown`               | `Down`              |
| Task                  | `concepts.CompletionStatus`      | `CompletionStatusCompleted`     | `Completed`         |
|                       |                                  | `CompletionStatusRunning`       | `TaskRunning`       |
|                       |                                  | `CompletionStatusPending`       | `TaskPending`       |
|                       |                                  | `CompletionStatusFailing`       | `TaskFailing`       |
| Integration           | `concepts.OperationalStatus`     | `OperationalStatusOperational`  | `Operational`       |
|                       |                                  | `OperationalStatusPending`      | `OperationPending`  |
|                       |                                  | `OperationalStatusFailing`      | `OperationFailing`  |
| All                   | `concepts.SuspensionStatus`      | `SuspensionStatusPending`       | `PendingSuspension` |
|                       |                                  | `SuspensionStatusSuspending`    | `Suspending`        |
|                       |                                  | `SuspensionStatusSuspended`     | `Suspended`         |
| All                   | `concepts.GuardStatus`           | `GuardStatusBlocked`            | `Blocked`           |
|                       |                                  | `GuardStatusUnblocked`          | `Unblocked`         |

### 4. Implement the Builder

The builder wraps the generic builder, registers default handlers, and exposes a fluent configuration API.

```go
package gameserver

import (
    "fmt"

    "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "github.com/sourcehawk/operator-component-framework/pkg/generic"
    examplev1 "example.io/api/v1"
)

// Builder configures and validates a GameServer resource.
type Builder struct {
    base *generic.WorkloadBuilder[*examplev1.GameServer, *Mutator]
}

// NewBuilder creates a Builder with the provided GameServer as the desired base state.
//
// The object must have Name and Namespace set.
func NewBuilder(gs *examplev1.GameServer) *Builder {
    identityFunc := func(gs *examplev1.GameServer) string {
        return fmt.Sprintf("gameservers.example.io/v1/GameServer/%s/%s", gs.Namespace, gs.Name)
    }

    base := generic.NewWorkloadBuilder[*examplev1.GameServer, *Mutator](
        gs,
        identityFunc,
        NewMutator,
    )

    // Register default handlers.
    base.
        WithCustomConvergeStatus(DefaultConvergingStatusHandler).
        WithCustomGraceStatus(DefaultGraceStatusHandler).
        WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
        WithCustomSuspendMutation(DefaultSuspendMutationHandler).
        WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

    return &Builder{base: base}
}

// WithMutation registers a feature-gated mutation.
func (b *Builder) WithMutation(m Mutation) *Builder {
    b.base.WithMutation(feature.Mutation[*Mutator](m))
    return b
}

// WithDataExtractor registers a data extractor to run after reconciliation.
func (b *Builder) WithDataExtractor(extractor func(*examplev1.GameServer) error) *Builder {
    b.base.WithDataExtractor(extractor)
    return b
}

// WithCustomConvergeStatus overrides the default convergence status handler.
func (b *Builder) WithCustomConvergeStatus(
    handler func(concepts.ConvergingOperation, *examplev1.GameServer) (concepts.AliveStatusWithReason, error),
) *Builder {
    b.base.WithCustomConvergeStatus(handler)
    return b
}

// WithCustomGraceStatus overrides the default grace status handler.
func (b *Builder) WithCustomGraceStatus(
    handler func(*examplev1.GameServer) (concepts.GraceStatusWithReason, error),
) *Builder {
    b.base.WithCustomGraceStatus(handler)
    return b
}

// WithCustomSuspendStatus overrides the default suspension status handler.
func (b *Builder) WithCustomSuspendStatus(
    handler func(*examplev1.GameServer) (concepts.SuspensionStatusWithReason, error),
) *Builder {
    b.base.WithCustomSuspendStatus(handler)
    return b
}

// WithCustomSuspendMutation overrides the default suspension mutation handler.
func (b *Builder) WithCustomSuspendMutation(handler func(*Mutator) error) *Builder {
    b.base.WithCustomSuspendMutation(handler)
    return b
}

// WithCustomSuspendDeletionDecision overrides the default delete-on-suspend decision.
func (b *Builder) WithCustomSuspendDeletionDecision(handler func(*examplev1.GameServer) bool) *Builder {
    b.base.WithCustomSuspendDeletionDecision(handler)
    return b
}

// WithGuard registers a guard precondition that is evaluated before the object
// is applied. If the guard returns Blocked, the resource and all resources after
// it in the component are skipped. Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
    handler func(*examplev1.GameServer) (concepts.GuardStatusWithReason, error),
) *Builder {
    b.base.WithGuard(handler)
    return b
}

// Build validates the configuration and returns the initialized Resource.
func (b *Builder) Build() (*Resource, error) {
    genericRes, err := b.base.Build()
    if err != nil {
        return nil, err
    }
    return &Resource{base: genericRes}, nil
}
```

#### Builder Pattern Guidelines

- **Only the convergence handler is required.** The generic builder's `Build()` returns an error if the convergence
  status handler is not set (`ConvergingStatus` for workload/task, `OperationalStatus` for integration). Grace and
  suspension handlers default to safe no-ops at the generic layer, so you only need to override them if your CRD has
  domain-specific behavior for those lifecycle phases.
- **Register domain-specific defaults in the constructor.** Override the generic defaults where your CRD has meaningful
  semantics (e.g., a grace handler that inspects replica counts, a suspension handler that scales to zero).
- **Return `*Builder` from every method.** This enables the fluent chaining pattern used throughout the framework.
- **Validate in `Build()`.** The generic builder's `Build()` validates that the object has a name, namespace (for
  namespaced resources), identity function, mutator factory, and convergence handler. Add any custom validation after
  calling the generic build.

### 5. Implement the Resource

The resource is a thin wrapper that delegates every interface method to the generic resource. This layer exists so that
your package exports concrete types rather than generic ones.

```go
package gameserver

import (
    "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
    "github.com/sourcehawk/operator-component-framework/pkg/generic"
    examplev1 "example.io/api/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource manages a GameServer within a component's reconciliation loop.
//
// It implements:
//   - component.Resource (Identity, Object, Mutate)
//   - concepts.Alive (ConvergingStatus)
//   - concepts.Graceful (GraceStatus)
//   - concepts.Suspendable (DeleteOnSuspend, Suspend, SuspensionStatus)
//   - concepts.Guardable (GuardStatus)
//   - concepts.DataExtractable (ExtractData)
type Resource struct {
    base *generic.WorkloadResource[*examplev1.GameServer, *Mutator]
}

func (r *Resource) Identity() string {
    return r.base.Identity()
}

func (r *Resource) Object() (client.Object, error) {
    return r.base.Object()
}

func (r *Resource) Mutate(current client.Object) error {
    return r.base.Mutate(current)
}

func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
    return r.base.ConvergingStatus(op)
}

func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
    return r.base.GraceStatus()
}

func (r *Resource) DeleteOnSuspend() bool {
    return r.base.DeleteOnSuspend()
}

func (r *Resource) Suspend() error {
    return r.base.Suspend()
}

func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
    return r.base.SuspensionStatus()
}

func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
    return r.base.GuardStatus()
}

func (r *Resource) ExtractData() error {
    return r.base.ExtractData()
}
```

Which methods to include depends on your resource category:

| Category    | Typical Methods                                                                                                                                   |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Workload    | `Identity`, `Object`, `Mutate`, `ConvergingStatus`, `GraceStatus`, `DeleteOnSuspend`, `Suspend`, `SuspensionStatus`, `GuardStatus`, `ExtractData` |
| Static      | `Identity`, `Object`, `Mutate`, `GuardStatus`, `ExtractData`                                                                                      |
| Task        | `Identity`, `Object`, `Mutate`, `ConvergingStatus`, `DeleteOnSuspend`, `Suspend`, `SuspensionStatus`, `GuardStatus`, `ExtractData`                |
| Integration | `Identity`, `Object`, `Mutate`, `ConvergingStatus`, `GraceStatus`, `DeleteOnSuspend`, `Suspend`, `SuspensionStatus`, `GuardStatus`, `ExtractData` |

### 6. Define Feature Mutations

Feature mutations use the `Mutation` type alias you defined earlier. Each mutation declares a name, an optional feature
gate, and a function that calls mutator methods to record intent.

```go
package features

import (
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "example.io/gameserver"
)

// HighCapacityMode increases the max player count for versions >= 2.0.0.
func HighCapacityMode(version string) gameserver.Mutation {
    return gameserver.Mutation{
        Name:    "high-capacity-mode",
        Feature: feature.NewVersionGate(version, myVersionConstraints),
        Mutate: func(m *gameserver.Mutator) error {
            m.SetMaxPlayers(200)
            return nil
        },
    }
}

// CompetitiveMode enables competitive settings when the flag is set.
func CompetitiveMode(version string, enabled bool) gameserver.Mutation {
    return gameserver.Mutation{
        Name:    "competitive-mode",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *gameserver.Mutator) error {
            m.SetMaxPlayers(10)
            return nil
        },
    }
}
```

Mutations are applied in registration order. When a mutation's `Feature` is nil or reports `Enabled() == true`, its
`Mutate` function is called with the mutator. When `Enabled()` returns false, the mutation is skipped.

### 7. Register with a Component

Use your custom resource with the component builder exactly like a built-in primitive:

```go
func buildGameComponent(owner *MyOperatorCR) (*component.Component, error) {
    gs := &examplev1.GameServer{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "main-server",
            Namespace: owner.Namespace,
        },
        Spec: examplev1.GameServerSpec{
            Replicas:   ptr.To(int32(3)),
            MaxPlayers: 100,
        },
    }

    res, err := gameserver.NewBuilder(gs).
        WithMutation(features.HighCapacityMode(owner.Spec.Version)).
        WithMutation(features.CompetitiveMode(owner.Spec.Version, owner.Spec.Competitive)).
        Build()
    if err != nil {
        return nil, err
    }

    return component.NewComponentBuilder().
        WithName("game-server").
        WithConditionType("GameServerReady").
        WithResource(res, component.ResourceOptions{}).
        WithGracePeriod(5 * time.Minute).
        Suspend(owner.Spec.Suspended).
        Build()
}
```

---

## Cluster-Scoped Resources

For cluster-scoped CRDs, call `MarkClusterScoped()` on the generic builder before building. This changes validation to
reject a non-empty namespace instead of requiring one.

The generic builder exposes this through the embedded `BaseBuilder`:

```go
func NewBuilder(gs *examplev1.GameServer) *Builder {
    base := generic.NewWorkloadBuilder[*examplev1.GameServer, *Mutator](gs, identityFunc, NewMutator)
    base.MarkClusterScoped()
    // ... register handlers ...
    return &Builder{base: base}
}
```

---

## Category-Specific Notes

### Static Resources

Static resources have the simplest implementation. They do not participate in convergence or suspension reporting. The
builder uses `generic.NewStaticBuilder` and only supports `WithMutation` and `WithDataExtractor`. The resource wrapper
only needs `Identity`, `Object`, `Mutate`, and `ExtractData`.

### Task Resources

Task resources use `concepts.CompletionStatusWithReason` instead of `AliveStatusWithReason` for convergence. The
converging status handler reports `Completed`, `Running`, `Pending`, or `Failing`.

### Integration Resources

Integration resources use `concepts.OperationalStatusWithReason` for convergence. The status handler reports
`Operational`, `Pending`, or `Failing`. They also implement `Graceful` for health assessment after grace period expiry,
with a default handler that reports Healthy. The resource wrapper should include `GraceStatus` alongside the other
methods.

---

## Reference

| Package                  | Contains                                                  |
| ------------------------ | --------------------------------------------------------- |
| `pkg/generic`            | Generic resource types, builders, `ApplyMutations` helper |
| `pkg/feature`            | `Mutation`, `Gate`, `VersionGate`, `NewVersionGate`       |
| `pkg/component/concepts` | Lifecycle interfaces and status type constants            |
| `pkg/component`          | Component builder, `ResourceOptions`, reconciliation      |
| `pkg/primitives/*`       | Built-in implementations to use as reference              |
