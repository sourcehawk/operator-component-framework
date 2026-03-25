# ReplicaSet Primitive

The `replicaset` primitive is the framework's workload abstraction for managing Kubernetes `ReplicaSet` resources. It
integrates fully with the component lifecycle and provides a rich mutation API for managing containers, pod specs, and
metadata.

ReplicaSets are rarely managed directly — operators typically use Deployments. This primitive is provided for operators
that own ReplicaSets explicitly (e.g. custom rollout controllers).

## Capabilities

| Capability            | Detail                                                                                                                                        |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| **Health tracking**   | Verifies `ObservedGeneration` matches `Generation` before evaluating `ReadyReplicas`; reports `Healthy`, `Creating`, `Updating`, or `Scaling` |
| **Suspension**        | Scales to zero replicas; reports `Suspending` / `Suspended`                                                                                   |
| **Mutation pipeline** | Typed editors for metadata, replicaset spec, pod spec, and containers                                                                         |

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
        // baseline spec
    },
}

resource, err := replicaset.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ReplicaSet` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) replicaset.Mutation {
    return replicaset.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *replicaset.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder.

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func TracingMutation(version string, enabled bool) replicaset.Mutation {
    return replicaset.Mutation{
        Name:    "tracing",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *replicaset.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "jaeger-agent",
                Image: "jaegertracing/jaeger-agent:1.28",
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded:

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

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same mutation.

## Relevant Editors

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

Note: `spec.selector` is immutable after creation and is not exposed by this editor. Set it via the desired object
passed to `NewBuilder`.

### PodSpecEditor

Manages pod-level configuration via `m.EditPodSpec`.

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`,
`EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`,
`SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`.

```go
m.EditPodSpec(func(e *editors.PodSpecEditor) error {
    e.SetServiceAccountName("my-service-account")
    return nil
})
```

### ContainerEditor

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`. Always used in combination with a
[selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `ReplicaSet` object itself, or
`m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

## Convenience Methods

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureReplicas(n)`           | `EditReplicaSetSpec` → `SetReplicas(n)`                       |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in
the same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
