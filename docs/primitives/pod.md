# Pod Primitive

The `pod` primitive is the framework's built-in workload abstraction for managing Kubernetes `Pod` resources directly.
It integrates fully with the component lifecycle and provides a mutation API for managing containers, pod specs, and
metadata.

Pods are rarely managed directly by operators; this primitive is provided for completeness and for operators that manage
pod objects (e.g. debugging utilities, node-local agents).

## Capabilities

| Capability            | Detail                                                                                             |
| --------------------- | -------------------------------------------------------------------------------------------------- |
| **Health tracking**   | Monitors pod phase and container statuses; reports `Healthy`, `Creating`, `Updating`, or `Failing` |
| **Graceful rollouts** | Detects degraded or down states via grace status handler                                           |
| **Suspension**        | Deletes the pod (pods cannot be paused); reports `Suspended`                                       |
| **Mutation pipeline** | Typed editors for metadata, pod spec, and containers                                               |

## Building a Pod Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"

base := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "debug-pod",
        Namespace: owner.Namespace,
    },
    Spec: corev1.PodSpec{
        Containers: []corev1.Container{
            {
                Name:  "debug",
                Image: "busybox:latest",
            },
        },
    },
}

resource, err := pod.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `Pod` beyond its baseline. Each mutation is a named function that
receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) pod.Mutation {
    return pod.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *pod.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func DebugMutation(version string, enabled bool) pod.Mutation {
    return pod.Mutation{
        Name:    "debug-mode",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *pod.Mutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{Name: "DEBUG", Value: "true"})
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded. This ensures structural consistency across mutations.

| Step | Category                   | What it affects                                                         |
| ---- | -------------------------- | ----------------------------------------------------------------------- |
| 1    | Object metadata edits      | Labels and annotations on the `Pod` object                              |
| 2    | Pod spec edits             | Volumes, tolerations, node selectors, service account, security context |
| 3    | Regular container presence | Adding or removing containers from `spec.containers`                    |
| 4    | Regular container edits    | Env vars, args, resources (snapshot taken after step 3)                 |
| 5    | Init container presence    | Adding or removing containers from `spec.initContainers`                |
| 6    | Init container edits       | Env vars, args, resources (snapshot taken after step 5)                 |

Container edits (steps 4 and 6) are evaluated against a snapshot taken _after_ presence operations in the same mutation.
This means a single mutation can add a container and then configure it without selector resolution issues.

**Kubernetes immutability note:** most fields in `Pod.spec` are immutable after creation, including the overall
structure of `spec.containers` and `spec.initContainers` and the majority of per-container fields (such as `env`,
`args`, resources, ports, and probes). Presence operations such as `EnsureContainer` / `RemoveContainer` (and the
corresponding init container operations) are intended for use when constructing a new Pod or when recreating the Pod,
not for in-place updates to an existing Pod. If a mutation attempts to add or remove containers on an existing Pod, the
Kubernetes API server will reject the update. In practice, the set of fields that can be updated in-place on an existing
Pod is very small (primarily container images, plus a few feature-gated fields such as resources with in-place resize);
treat Pods as effectively immutable and use delete-and-recreate when you need to change other container attributes.

## Editors

### PodSpecEditor

Manages pod-level configuration via `m.EditPodSpec`.

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`,
`EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`,
`SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`. `RemoveTolerations` accepts a predicate
function (`match func(corev1.Toleration) bool`) and removes all tolerations for which `match` returns `true`.

```go
m.EditPodSpec(func(e *editors.PodSpecEditor) error {
    e.SetServiceAccountName("my-service-account")
    e.EnsureVolume(corev1.Volume{
        Name: "config",
        VolumeSource: corev1.VolumeSource{
            ConfigMap: &corev1.ConfigMapVolumeSource{
                LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
            },
        },
    })
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
    e.EnsureArg("--metrics-port=9090")
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

For fields not covered by the typed API (such as volume mounts), use `Raw()`:

```go
m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
    e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
        Name:      "config",
        MountPath: "/etc/config",
    })
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})
```

### Raw Escape Hatch

All editors provide a `.Raw()` method for direct access to the underlying Kubernetes struct when the typed API is
insufficient. The mutation remains scoped to the editor's target — you cannot accidentally modify unrelated parts of the
spec.

```go
m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
    e.Raw().SecurityContext = &corev1.SecurityContext{
        ReadOnlyRootFilesystem: ptr.To(true),
    }
    return nil
})
```

## Convenience Methods

The `Mutator` also exposes convenience wrappers that target all containers at once:

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Suspension

Pods cannot be paused. The default behavior deletes the pod when the component is suspended.

- `DefaultDeleteOnSuspendHandler`: returns `true` — pod is deleted on suspend.
- `DefaultSuspendMutationHandler`: no-op (deletion is handled by the framework).
- `DefaultSuspensionStatusHandler`: always returns `{Suspended, "Pod deleted on suspend"}`.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
The internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in
the same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
