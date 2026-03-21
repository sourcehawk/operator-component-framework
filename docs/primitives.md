# Resource Primitives

The `primitives` package provides reusable, type-safe wrappers for individual Kubernetes objects. Primitives sit between the [Component layer](component.md) and raw Kubernetes resources — they handle the complexities of state synchronization, mutation, and lifecycle management so operator authors don't have to.

## What a Primitive Is

A primitive wraps a specific Kubernetes kind (e.g., `Deployment`, `ConfigMap`) and encapsulates:

- **Desired state baseline** — the ideal configuration of the resource.
- **Lifecycle integration** — built-in readiness detection, grace handling, and suspension.
- **Mutation surfaces** — typed APIs for modifying the resource based on active features or version constraints.
- **Field application rules** — precise control over which fields are merged or preserved during reconciliation.

Each primitive implements the `component.Resource` interface, and may additionally implement one or more [lifecycle interfaces](#lifecycle-interfaces) to participate in component status aggregation.

## Primitive Categories

The framework categorizes primitives based on their runtime behavior.

### Static

Examples: `ConfigMap`, `Secret`, `ServiceAccount`, RBAC objects, `PodDisruptionBudget`

These resources have a mostly static desired state. They are created or updated based on configuration but have no complex runtime convergence. They are considered `Ready` as long as they exist. They may optionally implement `Alive` or `Operational` for more granular tracking.

### Workload

Examples: `Deployment`, `StatefulSet`, `DaemonSet`

These resources represent long-running processes that require runtime convergence (pods being scheduled and becoming ready). They implement `Alive`, `Graceful`, and `Suspendable` — supporting health tracking, grace periods, and scaling to zero.

### Task

Examples: `Job`

These resources represent short-lived operations that run to completion — database migrations, backups, initialization steps. They implement `Completable` and `Suspendable`. When suspended, tasks can be paused (if the underlying resource supports it) or deleted and recreated when resumed.

### Integration

Examples: `Service`, `Ingress`, `Gateway`, `CronJob`

These resources define integration points with external or cluster-level systems (networking, load balancers, DNS, schedules). Their readiness depends on external controllers and may be delayed or partial. They implement `Operational` and/or `Suspendable`.

## Lifecycle Interfaces

Primitives implement behavioral interfaces that the component layer uses for status aggregation:

| Interface         | Status values reported                                   | Typical use                               |
|-------------------|----------------------------------------------------------|-------------------------------------------|
| `Alive`           | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing`  | Deployments, StatefulSets, DaemonSets     |
| `Graceful`        | `Ready`, `Degraded`, `Down`                              | Workloads with slow or stalled rollouts   |
| `Suspendable`     | `PendingSuspension`, `Suspending`, `Suspended`           | Any resource with a deactivation behavior |
| `Completable`     | `Completed`, `TaskRunning`, `TaskPending`, `TaskFailing` | Jobs and task primitives                  |
| `Operational`     | `Operational`, `OperationPending`, `OperationFailing`    | Services, Ingresses, CronJobs             |
| `DataExtractable` | _(no status, side-effecting)_                            | Resources that expose post-sync data      |

Custom resource wrappers can implement any subset of these interfaces to opt into the corresponding component behaviors.

## Field Application Model

When a primitive is reconciled, it applies changes in a fixed three-stage pipeline:

```
1. Baseline application   →   merge desired state onto current object
2. Flavor adjustments     →   preserve fields managed by external controllers
3. Mutation edits         →   apply feature-specific or version-specific changes
```

This ordering guarantees that mutations always operate on a predictable, fully-formed baseline.

### Flavors

Flavors are reusable merge policies that run after baseline application but before mutations. Their purpose is to preserve fields that may be managed by external controllers or tools — sidecar injectors, autoscalers, annotation-based tooling — that the primitive should not overwrite.

Examples of what flavors can preserve:
- Labels and annotations added by external tools
- Pod template metadata managed by injection webhooks
- Fields managed by the Kubernetes HPA

Flavors allow primitives to coexist in clusters where multiple controllers touch the same resources.

## Mutation System

Workload and Task primitives use a **plan-and-apply pattern**: instead of mutating the Kubernetes object directly, mutations record their intent through typed editors, which are applied in a single controlled pass.

This design:
- **Prevents uncontrolled mutation** — changes are staged before any object is touched
- **Enables composability** — independent features contribute edits without knowing about each other
- **Guarantees ordering** — features apply in registration order; within a feature, categories apply in a fixed sequence
- **Avoids error-prone slice manipulation** — editors handle presence operations and stable selection internally

## Mutation Editors

Editors provide scoped, typed APIs for modifying specific parts of a resource:

| Editor                 | Scope                                                                   |
|------------------------|-------------------------------------------------------------------------|
| `ContainerEditor`      | Environment variables, arguments, resource limits, ports                |
| `PodSpecEditor`        | Volumes, tolerations, node selectors, service account, security context |
| `DeploymentSpecEditor` | Replicas, update strategy, label selectors                              |
| `ObjectMetaEditor`     | Labels and annotations on the object or pod template                    |

Every editor exposes a `.Raw()` method for cases where the typed API is insufficient, giving direct access to the underlying Kubernetes struct while keeping the mutation scoped to that editor's target.

## Container Selectors

Selectors determine which containers an editor targets — important for multi-container pods:

```go
selectors.AllContainers()                    // every container in the pod
selectors.ContainerNamed("app")              // a single container by name
selectors.ContainersNamed("web", "api")      // multiple containers by name
selectors.ContainerAtIndex(0)                // container at a specific index
```

Selectors are evaluated against the container list *after* any presence operations (add/remove) within the same mutation have been applied. This means a single mutation can safely add a container and then configure it.

## Usage Examples

### Creating a primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"

base := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-server",
        Namespace: owner.Namespace,
    },
    // ... spec
}

resource, err := deployment.NewBuilder(base).
    WithFieldApplicationFlavor(deployment.PreserveCurrentLabels).
    Build()
```

### Adding a mutation

```go
import (
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
)

resource, err := deployment.NewBuilder(base).
    WithMutation(deployment.Mutation{
        Name: "add-proxy-sidecar",
        Mutate: func(m *deployment.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "proxy",
                Image: "envoyproxy/envoy:v1.29",
            })
            m.EditContainers(selectors.ContainerNamed("proxy"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "PROXY_ADMIN_PORT", Value: "9901"})
                return nil
            })
            return nil
        },
    }).
    Build()
```

### Targeting multiple containers

```go
m.EditContainers(selectors.ContainersNamed("web", "api"), func(e *editors.ContainerEditor) error {
    e.EnsureArg("--log-format=json")
    return nil
})
```

## Implementing a Custom Resource

When the built-in primitives do not cover your use case, implement the `component.Resource` interface directly:

```go
type Resource interface {
    Object() (client.Object, error)
    Mutate(current client.Object) error
    Identity() string
}
```

Then implement whichever lifecycle interfaces your resource needs (`Alive`, `Suspendable`, etc.). See the [examples directory](../examples/) for complete implementations.

Custom resources are appropriate when:
- You are managing a custom CRD with specialized health or readiness logic
- The resource has unusual lifecycle semantics (e.g., must be deleted and recreated rather than updated in place)
- You need mutation behavior not covered by the standard editors