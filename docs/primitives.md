# Resource Primitives

The `primitives` package provides reusable, type-safe wrappers for individual Kubernetes objects. Primitives sit between
the [Component layer](component.md) and raw Kubernetes resources. They handle the complexities of state synchronization,
mutation, and lifecycle management so operator authors don't have to.

## What a Primitive Is

A primitive wraps a specific Kubernetes kind (e.g., `Deployment`, `ConfigMap`) and encapsulates:

- **Desired state baseline**: the ideal configuration of the resource.
- **Lifecycle integration**: built-in readiness detection, grace handling, and suspension.
- **Mutation surfaces**: typed APIs for modifying the resource based on active features or version constraints.
- **Server-Side Apply**: desired state is applied via SSA, preserving server defaults and fields managed by external
  controllers.

Each primitive implements the `component.Resource` interface, and may additionally implement one or more
[lifecycle interfaces](#lifecycle-interfaces) to participate in component status aggregation.

## Primitive Categories

The framework categorizes primitives based on their runtime behavior.

### Static

Examples: `ConfigMap`, `Secret`, `ServiceAccount`, RBAC objects, `PodDisruptionBudget`

These resources have a mostly static desired state. They are created or updated based on configuration but have no
complex runtime convergence. They are considered `Ready` as long as they exist. They may optionally implement `Alive` or
`Operational` for more granular tracking.

### Workload

Examples: `Deployment`, `StatefulSet`, `DaemonSet`

These resources represent long-running processes that require runtime convergence (pods being scheduled and becoming
ready). They implement `Alive`, `Graceful`, and `Suspendable`, supporting health tracking, grace periods, and scaling to
zero.

### Task

Examples: `Job`

These resources represent short-lived operations that run to completion (database migrations, backups, initialization
steps). They implement `Completable` and `Suspendable`. When suspended, tasks can be paused (if the underlying resource
supports it) or deleted and recreated when resumed.

### Integration

Examples: `Service`, `Ingress`, `Gateway`, `CronJob`

These resources define integration points with external or cluster-level systems (networking, load balancers, DNS,
schedules). Their readiness depends on external controllers and may be delayed or partial. They implement `Operational`,
`Graceful`, and/or `Suspendable`.

## Cluster-Scoped Primitives

Some Kubernetes resources are cluster-scoped: they have no namespace. Examples include `ClusterRole`,
`ClusterRoleBinding`, and `PersistentVolume`.

When implementing a primitive for a cluster-scoped kind, the primitive's builder must explicitly call
`MarkClusterScoped()` on its internal `BaseBuilder` during construction. This changes `ValidateBase()` behavior: instead
of requiring a non-empty namespace, it rejects a non-empty namespace. The primitive's builder is also responsible for
providing an identity function that formats the identity string appropriately, typically omitting the namespace segment
(e.g., `rbac.authorization.k8s.io/v1/ClusterRole/my-role` rather than including a namespace).

At reconcile time, the component framework automatically detects scope incompatibilities between the owner CRD and
managed resources using the cluster's REST mapper. See [Cluster-Scoped Resources](component.md#cluster-scoped-resources)
in the component documentation for details on owner reference behavior and garbage collection.

## Lifecycle Interfaces

Primitives implement behavioral interfaces that the component layer uses for status aggregation:

| Interface         | Status values reported                                   | Typical use                                      |
| ----------------- | -------------------------------------------------------- | ------------------------------------------------ |
| `Alive`           | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing`  | Deployments, StatefulSets, DaemonSets            |
| `Graceful`        | `Healthy`, `Degraded`, `Down`                            | Workloads and integrations with slow convergence |
| `Suspendable`     | `PendingSuspension`, `Suspending`, `Suspended`           | Any resource with a deactivation behavior        |
| `Completable`     | `Completed`, `TaskRunning`, `TaskPending`, `TaskFailing` | Jobs and task primitives                         |
| `Operational`     | `Operational`, `OperationPending`, `OperationFailing`    | Services, Ingresses, CronJobs                    |
| `DataExtractable` | _(no status, side-effecting)_                            | Resources that expose post-sync data             |
| `Guardable`       | `Blocked`, `Unblocked`                                   | Resources with runtime preconditions             |

Custom resource wrappers can implement any subset of these interfaces to opt into the corresponding component behaviors.

## Server-Side Apply

The framework reconciles resources using **Server-Side Apply** (SSA). Each primitive builds the desired state (the
baseline object with all registered mutations applied) and patches it to the cluster using `client.Apply`. Only fields
the operator declares are sent; server-managed defaults, fields set by other controllers (HPAs, sidecar injectors,
annotation-based tooling), and values written by webhooks are left untouched.

Field ownership is tracked automatically by the Kubernetes API server. The field manager name is derived from the owner
and component: `"{Owner.GetKind()}/{componentName}"`. The framework applies with forced ownership, meaning it will take
control of any conflicting fields from other managers. Fields that the operator does not include in its desired state
are left to their current owners.

This approach removes the perpetual-update problem that arises when an operator strips server defaults every reconcile
cycle, and it allows primitives to coexist naturally in clusters where multiple controllers touch the same resources.

## Mutation System

Primitives use a **plan-and-apply pattern**: instead of mutating the Kubernetes object directly, mutations record their
intent through typed editors, which are applied in a single controlled pass.

This design:

- **Prevents uncontrolled mutation**: changes are staged before any object is touched
- **Enables composability**: independent features contribute edits without knowing about each other
- **Guarantees ordering**: features apply in registration order; within a feature, categories apply in a fixed sequence
- **Avoids error-prone slice manipulation**: editors handle presence operations and stable selection internally

## Mutation Editors

Editors provide scoped, typed APIs for modifying specific parts of a resource. Every editor exposes a `.Raw()` method
for cases where the typed API is insufficient, giving direct access to the underlying Kubernetes struct while keeping
the mutation scoped to that editor's target.

Each primitive documents its available editors in its own [Relevant Editors](#built-in-primitives) section.

## Container Selectors

Selectors determine which containers an editor targets. This is important for multi-container pods:

```go
selectors.AllContainers()                    // every container in the pod
selectors.ContainerNamed("app")              // a single container by name
selectors.ContainersNamed("web", "api")      // multiple containers by name
selectors.ContainerAtIndex(0)                // container at a specific index
```

Selectors are evaluated against the container list _after_ any presence operations (add/remove) within the same mutation
have been applied. This means a single mutation can safely add a container and then configure it.

## Built-in Primitives

| Primitive                           | Category    | Documentation                                             |
| ----------------------------------- | ----------- | --------------------------------------------------------- |
| `pkg/primitives/deployment`         | Workload    | [deployment.md](primitives/deployment.md)                 |
| `pkg/primitives/statefulset`        | Workload    | [statefulset.md](primitives/statefulset.md)               |
| `pkg/primitives/replicaset`         | Workload    | [replicaset.md](primitives/replicaset.md)                 |
| `pkg/primitives/daemonset`          | Workload    | [daemonset.md](primitives/daemonset.md)                   |
| `pkg/primitives/pod`                | Workload    | [pod.md](primitives/pod.md)                               |
| `pkg/primitives/job`                | Task        | [job.md](primitives/job.md)                               |
| `pkg/primitives/cronjob`            | Integration | [cronjob.md](primitives/cronjob.md)                       |
| `pkg/primitives/configmap`          | Static      | [configmap.md](primitives/configmap.md)                   |
| `pkg/primitives/secret`             | Static      | [secret.md](primitives/secret.md)                         |
| `pkg/primitives/role`               | Static      | [role.md](primitives/role.md)                             |
| `pkg/primitives/rolebinding`        | Static      | [rolebinding.md](primitives/rolebinding.md)               |
| `pkg/primitives/pdb`                | Static      | [pdb.md](primitives/pdb.md)                               |
| `pkg/primitives/clusterrole`        | Static      | [clusterrole.md](primitives/clusterrole.md)               |
| `pkg/primitives/clusterrolebinding` | Static      | [clusterrolebinding.md](primitives/clusterrolebinding.md) |
| `pkg/primitives/serviceaccount`     | Static      | [serviceaccount.md](primitives/serviceaccount.md)         |
| `pkg/primitives/service`            | Integration | [service.md](primitives/service.md)                       |
| `pkg/primitives/pv`                 | Integration | [pv.md](primitives/pv.md)                                 |
| `pkg/primitives/pvc`                | Integration | [pvc.md](primitives/pvc.md)                               |
| `pkg/primitives/hpa`                | Integration | [hpa.md](primitives/hpa.md)                               |
| `pkg/primitives/ingress`            | Integration | [ingress.md](primitives/ingress.md)                       |
| `pkg/primitives/networkpolicy`      | Static      | [networkpolicy.md](primitives/networkpolicy.md)           |

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
    Build()
```

### Adding a mutation

```go
import (
    corev1 "k8s.io/api/core/v1"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
)

resource, err := deployment.NewBuilder(base).
    WithMutation(deployment.Mutation{
        Name:    "add-proxy-sidecar",
        Feature: feature.NewVersionGate(version, nil),
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

### Adding a guard

Guards block a resource from being applied until a precondition is met. Combined with data extraction, they enable
runtime dependencies between resources: an earlier resource extracts data after it is applied, and a later resource's
guard checks that data before proceeding.

```go
resource, err := deployment.NewBuilder(base).
    WithGuard(func(_ appsv1.Deployment) (concepts.GuardStatusWithReason, error) {
        if roleARN == "" {
            return concepts.GuardStatusWithReason{
                Status: concepts.GuardStatusBlocked,
                Reason: "waiting for IAM role ARN",
            }, nil
        }
        return concepts.GuardStatusWithReason{Status: concepts.GuardStatusUnblocked}, nil
    }).
    Build()
```

See [Guards](component.md#guards) in the component documentation for the full behavioral contract and a complete example
showing data extraction feeding into a guard.

## Unstructured Primitives

| Primitive                                 | Category    | Documentation                                 |
| ----------------------------------------- | ----------- | --------------------------------------------- |
| `pkg/primitives/unstructured/static`      | Static      | [unstructured.md](primitives/unstructured.md) |
| `pkg/primitives/unstructured/workload`    | Workload    | [unstructured.md](primitives/unstructured.md) |
| `pkg/primitives/unstructured/integration` | Integration | [unstructured.md](primitives/unstructured.md) |
| `pkg/primitives/unstructured/task`        | Task        | [unstructured.md](primitives/unstructured.md) |

The unstructured primitives are an escape hatch for managing arbitrary Kubernetes objects that have no Go type, for
example, Crossplane resources, external CRDs, or any object known only at runtime. One variant exists per
[lifecycle category](#primitive-categories), each implementing the corresponding interfaces.

Because the framework cannot know the semantics of an unstructured object, it does **not infer any semantic or
domain-specific defaults**. The builders instead configure generic safe defaults: if you omit a grace handler, the
primitive treats the resource as Healthy; if you omit suspension handlers, the primitive reports Suspended and the
suspend mutation is a no-op. Only the converge/operational status handler is required at build time.

The unstructured primitives share a single `Mutator` and use an `UnstructuredContentEditor` for manipulating nested
fields in the object's content map. See [unstructured.md](primitives/unstructured.md) for full details.

## Implementing a Custom Resource

When the built-in primitives do not cover your use case, you can implement custom resource wrappers for any Kubernetes
object, including custom CRDs. The framework provides generic building blocks in `pkg/generic` that handle
reconciliation mechanics, mutation sequencing, and suspension, so you only need to provide type-specific logic.

See the [Custom Resource Implementation Guide](custom-resource.md) for a complete walkthrough covering mutator design,
status handlers, builders, and component registration.
