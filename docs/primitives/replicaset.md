# ReplicaSet Primitive

The `replicaset` primitive wraps a Kubernetes `ReplicaSet` and provides health tracking, suspension, and a typed
mutation API for managing replicas, pod spec, and containers as part of the component lifecycle.

ReplicaSets are rarely managed directly by operators. Deployments own and manage ReplicaSets automatically. This
primitive is intended for operators that explicitly own ReplicaSet objects, such as custom rollout controllers that
manage sets of pods without Deployment's rollout semantics.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                  |
| ------------------------------------------------------------ | ------------------------------------------------------- |
| `Alive`                                                      | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing` |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                           |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`          |
| `Guardable`                                                  | `Blocked`                                               |
| `DataExtractable`                                            | _(side-effecting, no status)_                           |

## Building a ReplicaSet Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/replicaset"

base := &appsv1.ReplicaSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "worker",
        Namespace: owner.Namespace,
    },
    Spec: appsv1.ReplicaSetSpec{
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "worker"},
        },
        Template: corev1.PodTemplateSpec{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"app": "worker"},
            },
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{
                    {Name: "worker"},
                },
            },
        },
    },
}

resource, err := replicaset.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `replicaset.Mutation` that receives a `*replicaset.Mutator` and records edits through typed
editors.

```go
func WorkerConfigMutation(version string) replicaset.Mutation {
    return replicaset.Mutation{
        Name:    "worker-config",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *replicaset.Mutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{Name: "WORKER_THREADS", Value: "4"})
            return nil
        },
    }
}
```

See [the mutation system](../primitives.md#the-mutation-system),
[boolean gating](../primitives.md#boolean-gated-mutations), and
[version gating](../primitives.md#version-gated-mutations).

## Internal Mutation Ordering

Within each feature, edits run in this fixed category order:

| Step | Category                    | What it affects                                                         |
| ---- | --------------------------- | ----------------------------------------------------------------------- |
| 1    | Object metadata edits       | Labels and annotations on the `ReplicaSet` object                       |
| 2    | ReplicaSetSpec edits        | Replicas, min ready seconds                                             |
| 3    | Pod template metadata edits | Labels and annotations on the pod template                              |
| 4    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence  | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits     | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence     | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits        | Env vars, args, resources (snapshot taken after step 7)                 |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same feature.

## Relevant Editors

For the generic editor and selector concepts, see [mutation editors](../primitives.md#mutation-editors) and
[container selectors](../primitives.md#container-selectors).

### ReplicaSetSpecEditor

Controls replicaset-level settings via `m.EditReplicaSetSpec`.

Available methods: `SetReplicas`, `SetMinReadySeconds`, `Raw`.

```go
m.EditReplicaSetSpec(func(e *editors.ReplicaSetSpecEditor) error {
    e.SetReplicas(3)
    e.SetMinReadySeconds(10)
    return nil
})
```

!!! note "`spec.selector` is immutable"

    `spec.selector` cannot be changed after the ReplicaSet is created. Set it in the desired object passed to
    `NewBuilder`; it is not exposed by `ReplicaSetSpecEditor`.

### PodSpecEditor

Manages pod-level configuration via `m.EditPodSpec`.

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`,
`EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`,
`SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`.

```go
m.EditPodSpec(func(e *editors.PodSpecEditor) error {
    e.SetServiceAccountName("worker-sa")
    return nil
})
```

### ContainerEditor

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`, combined with a
[container selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("worker"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `ReplicaSet` itself or `m.EditPodTemplateMetadata`
for the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

## Convenience Methods

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureReplicas(n)`           | `EditReplicaSetSpec` → `SetReplicas(n)`                       |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Workload-Kind-Agnostic Mutations

The `replicaset.Mutator` does not implement `primitives.WorkloadMutator` and therefore does not have a `LiftMutation`
adapter. Workload-kind-agnostic mutations target the Deployment, StatefulSet, and DaemonSet mutators. If you need to
share container or env-var mutations across those kinds and a ReplicaSet, write the shared logic as a plain function
that accepts `*replicaset.Mutator` and call it directly from a `replicaset.Mutation`.

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the cross-kind pattern.

## Data Extraction

`replicaset.ExtractInto` declares that this ReplicaSet produces the value of a data cell, such as the number of pods
reporting ready. The function receives a value copy of the reconciled ReplicaSet after each sync cycle:

```go
readyReplicas := concepts.NewData[int32]("worker-ready-replicas")

builder := replicaset.NewBuilder(base)
replicaset.ExtractInto(builder, readyReplicas, func(rs appsv1.ReplicaSet) (int32, error) {
    return rs.Status.ReadyReplicas, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(readyReplicas)` or read it
opportunistically with `WithOptionalData(readyReplicas)`. See [Declared Data](../component.md#declared-data).

## Suspension

When the component is suspended, the ReplicaSet is scaled to zero replicas. The resource is not deleted.

- `DefaultSuspendMutationHandler` calls `EnsureReplicas(0)`.
- `DefaultSuspensionStatusHandler` reports `Suspending` while `Status.Replicas > 0`, then `Suspended`.
- `DefaultDeleteOnSuspendHandler` returns `false`.

Override any handler via `WithCustomSuspendMutation`, `WithCustomSuspendStatus`, or `WithCustomSuspendDeletionDecision`
on the builder.

## Full Example

```go
func WorkerMutation(version string, replicas int32) replicaset.Mutation {
    return replicaset.Mutation{
        Name:    "worker-sizing",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *replicaset.Mutator) error {
            m.EnsureReplicas(replicas)

            m.EditContainers(selectors.ContainerNamed("worker"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "WORKER_THREADS", Value: "4"})
                e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
                e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("256Mi"))
                return nil
            })

            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.SetServiceAccountName("worker-sa")
                return nil
            })

            return nil
        },
    }
}
```

## Guidance

**Prefer Deployments over direct ReplicaSet management.** Deployments add rolling-update semantics and revision history.
Use this primitive only when you are building a custom rollout controller or you have a specific reason to own
ReplicaSet objects directly.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so selectors in the
same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
