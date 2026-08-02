# Pod Primitive

The `pod` primitive wraps a Kubernetes `Pod` and provides health tracking, suspension, and a typed mutation API for
managing pod spec and containers as part of the component lifecycle.

Most operators do not manage Pod objects directly; higher-level primitives (Deployment, StatefulSet, DaemonSet) own pod
lifecycle. This primitive is provided for operators that explicitly manage individual pods, such as debugging utilities
or node-local agents where a controller-per-pod model applies.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                         |
| ------------------------------------------------------------ | ---------------------------------------------- |
| `Alive`                                                      | `Healthy`, `Creating`, `Updating`, `Failing`   |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                  |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended` |
| `Guardable`                                                  | `Blocked`                                      |
| `DataExtractable`                                            | _(side-effecting, no status)_                  |

## Building a Pod Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"

base := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "agent",
        Namespace: owner.Namespace,
    },
    Spec: corev1.PodSpec{
        Containers: []corev1.Container{
            {Name: "agent", Image: "agent:latest"},
        },
    },
}

resource, err := pod.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `pod.Mutation` that receives a `*pod.Mutator` and records edits through typed editors.

```go
func AgentConfigMutation(version string, debug bool) pod.Mutation {
    return pod.Mutation{
        Name:    "agent-config",
        Feature: feature.NewVersionGate(version, nil).When(debug),
        Mutate: func(m *pod.Mutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
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

| Step | Category                   | What it affects                                                         |
| ---- | -------------------------- | ----------------------------------------------------------------------- |
| 1    | Object metadata edits      | Labels and annotations on the `Pod` object                              |
| 2    | Pod spec edits             | Volumes, tolerations, node selectors, service account, security context |
| 3    | Regular container presence | Adding or removing containers from `spec.containers`                    |
| 4    | Regular container edits    | Env vars, args, resources (snapshot taken after step 3)                 |
| 5    | Init container presence    | Adding or removing containers from `spec.initContainers`                |
| 6    | Init container edits       | Env vars, args, resources (snapshot taken after step 5)                 |

Container edits (steps 4 and 6) are evaluated against a snapshot taken _after_ presence operations in the same feature.

!!! warning "Pod spec is largely immutable after creation"

    Most fields in `Pod.spec` are immutable once the pod exists, including the container list, env vars, args,
    resources, ports, and probes. Presence operations (`EnsureContainer`, `RemoveContainer`) and most field mutations
    are only effective when constructing a new pod or when the pod will be deleted and recreated. The very small set of
    fields that can be updated in-place includes container images and, in some configurations, resource requests. Treat
    pods as effectively immutable and plan on delete-and-recreate when structural changes are needed.

## Relevant Editors

For the generic editor and selector concepts, see [mutation editors](../primitives.md#mutation-editors) and
[container selectors](../primitives.md#container-selectors).

### PodSpecEditor

Manages pod-level configuration via `m.EditPodSpec`.

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`,
`EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`,
`SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`.

```go
m.EditPodSpec(func(e *editors.PodSpecEditor) error {
    e.SetServiceAccountName("agent-sa")
    e.EnsureVolume(corev1.Volume{
        Name: "config",
        VolumeSource: corev1.VolumeSource{
            ConfigMap: &corev1.ConfigMapVolumeSource{
                LocalObjectReference: corev1.LocalObjectReference{Name: "agent-config"},
            },
        },
    })
    return nil
})
```

### ContainerEditor

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`, combined with a
[container selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("agent"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

For fields the typed API does not cover, such as volume mounts, use `Raw()`:

```go
m.EditContainers(selectors.ContainerNamed("agent"), func(e *editors.ContainerEditor) error {
    e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
        Name:      "config",
        MountPath: "/etc/agent",
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

## Convenience Methods

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Suspension

Pods cannot be paused. The default behavior deletes the pod when the component is suspended.

- `DefaultDeleteOnSuspendHandler` returns `true`. The pod is deleted on suspend.
- `DefaultSuspendMutationHandler` is a no-op; deletion is handled by the framework.
- `DefaultSuspensionStatusHandler` always reports `Suspended` with reason `"Pod deleted on suspend"`.

## Full Example

```go
func AgentMutation(version string, cfgName string) pod.Mutation {
    return pod.Mutation{
        Name:    "agent-setup",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pod.Mutator) error {
            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.SetServiceAccountName("agent-sa")
                e.EnsureVolume(corev1.Volume{
                    Name: "config",
                    VolumeSource: corev1.VolumeSource{
                        ConfigMap: &corev1.ConfigMapVolumeSource{
                            LocalObjectReference: corev1.LocalObjectReference{Name: cfgName},
                        },
                    },
                })
                return nil
            })

            m.EditContainers(selectors.ContainerNamed("agent"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "CONFIG_PATH", Value: "/etc/agent/config.yaml"})
                e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("200m"))
                e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("128Mi"))
                e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
                    Name:      "config",
                    MountPath: "/etc/agent",
                    ReadOnly:  true,
                })
                return nil
            })

            return nil
        },
    }
}
```

## Guidance

**Pods are effectively immutable after creation.** Plan the full desired state before the pod is created. Changes to
most spec fields require deleting and recreating the pod. Use the Deployment, StatefulSet, or DaemonSet primitives for
workloads that need rolling updates or scaling without manual recreation.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
