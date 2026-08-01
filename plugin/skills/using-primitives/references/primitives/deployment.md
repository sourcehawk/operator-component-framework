# Deployment Primitive

The `deployment` primitive wraps a Kubernetes `Deployment` and provides health tracking, suspension, and a typed
mutation API for managing replicas, pod spec, and containers as part of the component lifecycle.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                  |
| ------------------------------------------------------------ | ------------------------------------------------------- |
| `Alive`                                                      | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing` |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                           |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`          |
| `Guardable`                                                  | `Blocked`                                               |
| `DataExtractable`                                            | _(side-effecting, no status)_                           |

## Building a Deployment Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"

base := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-server",
        Namespace: owner.Namespace,
    },
    Spec: appsv1.DeploymentSpec{
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{
                    {Name: "web"},
                },
            },
        },
    },
}

resource, err := deployment.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `deployment.Mutation` that receives a `*deployment.Mutator` and records edits through typed
editors.

```go
func ConfigMutation(version string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "config",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *deployment.Mutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
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
| 1    | Object metadata edits       | Labels and annotations on the `Deployment` object                       |
| 2    | DeploymentSpec edits        | Replicas, progress deadline, revision history, etc.                     |
| 3    | Pod template metadata edits | Labels and annotations on the pod template                              |
| 4    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence  | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits     | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence     | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits        | Env vars, args, resources (snapshot taken after step 7)                 |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same feature.
A single mutation can add a container and then configure it without selector resolution issues.

## Relevant Editors

For the generic editor and selector concepts, see [mutation editors](../primitives.md#mutation-editors) and
[container selectors](../primitives.md#container-selectors).

### DeploymentSpecEditor

Controls deployment-level settings via `m.EditDeploymentSpec`.

Available methods: `SetReplicas`, `SetPaused`, `SetMinReadySeconds`, `SetRevisionHistoryLimit`,
`SetProgressDeadlineSeconds`, `Raw`.

```go
m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
    e.SetReplicas(3)
    e.SetProgressDeadlineSeconds(600)
    return nil
})
```

Use `Raw()` for fields the typed API does not cover, such as update strategy:

```go
m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
    e.Raw().Strategy = appsv1.DeploymentStrategy{
        Type: appsv1.RollingUpdateDeploymentStrategyType,
    }
    return nil
})
```

### PodSpecEditor

Manages pod-level configuration via `m.EditPodSpec`.

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`,
`EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`,
`SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`.

```go
m.EditPodSpec(func(e *editors.PodSpecEditor) error {
    e.SetServiceAccountName("web-sa")
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

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`, combined with a
[container selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("web"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

For fields the typed API does not cover, such as volume mounts, use `Raw()`:

```go
m.EditContainers(selectors.ContainerNamed("web"), func(e *editors.ContainerEditor) error {
    e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
        Name:      "config",
        MountPath: "/etc/config",
    })
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `Deployment` itself or `m.EditPodTemplateMetadata`
for the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})
m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureAnnotation("prometheus.io/scrape", "true")
    return nil
})
```

## Convenience Methods

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureReplicas(n)`           | `EditDeploymentSpec` → `SetReplicas(n)`                       |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Workload-Kind-Agnostic Mutations

A mutation written against `primitives.WorkloadMutator` can be applied to a Deployment builder using
`deployment.LiftMutation`. This lets one emitter function target Deployments, StatefulSets, and DaemonSets without
duplicating code.

```go
frontend.WithMutation(deployment.LiftMutation(sharedAuthMutation()))
```

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the full pattern.

## Suspension

When the component is suspended, the Deployment is scaled to zero replicas. The resource is not deleted.

- `DefaultSuspendMutationHandler` calls `EnsureReplicas(0)`.
- `DefaultSuspensionStatusHandler` reports `Suspending` while `Status.Replicas > 0`, then `Suspended`.
- `DefaultDeleteOnSuspendHandler` returns `false`.

Override any handler via `WithCustomSuspendMutation`, `WithCustomSuspendStatus`, or `WithCustomSuspendDeletionDecision`
on the builder.

## Full Example

```go
func LoggingSidecarMutation(version string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "logging-sidecar",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *deployment.Mutator) error {
            // Presence operation runs at step 5
            m.EnsureContainer(corev1.Container{
                Name:  "logger",
                Image: "fluent/fluent-bit:3.0",
            })

            // Container edit runs at step 6 (after presence)
            m.EditContainers(selectors.ContainerNamed("logger"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
                e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
                    Name:      "varlog",
                    MountPath: "/var/log",
                })
                return nil
            })

            // Pod spec edit runs at step 4 (before container presence)
            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.EnsureVolume(corev1.Volume{
                    Name:         "varlog",
                    VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
                })
                return nil
            })

            return nil
        },
    }
}
```

Although `EditPodSpec` is called after `EnsureContainer` in the source, it is applied in step 4 (before container
presence in step 5) per the internal ordering. Order your source calls for readability; the framework handles execution
order.

## Guidance

**Use a Deployment for stateless long-running workloads.** Deployments manage rolling updates and replica counts but do
not guarantee pod identity or stable network addresses. For stateful workloads requiring stable hostnames or persistent
volumes bound to a specific pod, use a StatefulSet.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so selectors in the
same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
