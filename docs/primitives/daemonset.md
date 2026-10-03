# DaemonSet Primitive

The `daemonset` primitive wraps a Kubernetes `DaemonSet` and provides health tracking, suspension, and a typed mutation
API for managing pod spec and containers as part of the component lifecycle. A DaemonSet runs one pod per qualifying
node.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                  |
| ------------------------------------------------------------ | ------------------------------------------------------- |
| `Alive`                                                      | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing` |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                           |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`          |
| `Guardable`                                                  | `Blocked`                                               |
| `DataExtractable`                                            | _(side-effecting, no status)_                           |

## Building a DaemonSet Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/daemonset"

base := &appsv1.DaemonSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "log-collector",
        Namespace: owner.Namespace,
    },
    Spec: appsv1.DaemonSetSpec{
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "log-collector"},
        },
        Template: corev1.PodTemplateSpec{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"app": "log-collector"},
            },
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{
                    {Name: "collector"},
                },
            },
        },
    },
}

resource, err := daemonset.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `daemonset.Mutation` that receives a `*daemonset.Mutator` and records edits through typed
editors.

```go
func MonitoringMutation(version string, enabled bool) daemonset.Mutation {
    return daemonset.Mutation{
        Name:    "monitoring",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *daemonset.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "metrics-exporter",
                Image: "prom/node-exporter:v1.8.0",
            })
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
| 1    | Object metadata edits       | Labels and annotations on the `DaemonSet` object                        |
| 2    | DaemonSetSpec edits         | Update strategy, min ready seconds, revision history limit              |
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

### DaemonSetSpecEditor

Controls DaemonSet-level settings via `m.EditDaemonSetSpec`.

Available methods: `SetUpdateStrategy`, `SetMinReadySeconds`, `SetRevisionHistoryLimit`, `Raw`.

```go
m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
    e.SetMinReadySeconds(30)
    e.SetRevisionHistoryLimit(5)
    return nil
})
```

Use `Raw()` for fields the typed API does not cover:

```go
m.EditDaemonSetSpec(func(e *editors.DaemonSetSpecEditor) error {
    e.Raw().UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
        Type: appsv1.RollingUpdateDaemonSetStrategyType,
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
    e.SetServiceAccountName("log-collector-sa")
    e.EnsureVolume(corev1.Volume{
        Name: "varlog",
        VolumeSource: corev1.VolumeSource{
            HostPath: &corev1.HostPathVolumeSource{Path: "/var/log"},
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
m.EditContainers(selectors.ContainerNamed("collector"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("200m"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `DaemonSet` itself or `m.EditPodTemplateMetadata`
for the pod template.

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

!!! note "No `EnsureReplicas` on DaemonSet"

    DaemonSets have no replicas field. Use node selectors, tolerations, and affinities in the pod spec to control which
    nodes run the pods.

## Workload-Kind-Agnostic Mutations

A mutation written against `primitives.WorkloadMutator` can be applied to a DaemonSet builder using
`daemonset.LiftMutation`. This lets one emitter function target DaemonSets, Deployments, and StatefulSets without
duplicating code.

```go
agent.WithMutation(daemonset.LiftMutation(sharedAuthMutation()))
```

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the full pattern.

## Data Extraction

`daemonset.ExtractInto` declares that this DaemonSet produces the value of a data cell, such as how many nodes are
running a ready pod. The function receives a value copy of the reconciled DaemonSet after each sync cycle:

```go
readyNodes := concepts.NewData[int32]("collector-ready-nodes")

builder := daemonset.NewBuilder(base)
daemonset.ExtractInto(builder, readyNodes, func(ds appsv1.DaemonSet) (int32, error) {
    return ds.Status.NumberReady, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(readyNodes)` or read it
opportunistically with `WithOptionalData(readyNodes)`. See [Declared Data](../component.md#declared-data).

## Suspension

DaemonSets have no replicas field, so there is no clean in-place pause mechanism. By default, the DaemonSet is
**deleted** when the component is suspended and recreated when unsuspended.

- `DefaultDeleteOnSuspendHandler` returns `true`.
- `DefaultSuspendMutationHandler` is a no-op (deletion is handled by the framework).
- `DefaultSuspensionStatusHandler` always reports `Suspended` with reason `"DaemonSet deleted on suspend"`.

Override these handlers via `WithCustomSuspendDeletionDecision`, `WithCustomSuspendMutation`, and
`WithCustomSuspendStatus` if a different suspension strategy is needed.

## Status Handlers

The desired pod count is `Status.DesiredNumberScheduled`: the number of nodes that must run a pod of the DaemonSet. Both
default handlers use the same rule for a healthy DaemonSet. All of these conditions must be true:

- The DaemonSet controller has observed the current spec: `Status.ObservedGeneration >= Generation`.
- `Status.NumberReady` equals the desired pod count.
- The rollout is complete: `Status.UpdatedNumberScheduled` is not less than the desired pod count. A DaemonSet with the
  `OnDelete` update strategy skips this check.

When the desired pod count is zero, no nodes match the node selector. This is a valid state, so the DaemonSet is healthy
once the controller has observed the current spec.

The rollout check is necessary when `maxSurge` is set. The controller then keeps the old pod on a node until the new pod
is ready, and it counts only the oldest pod of each node. If the new pods never become ready, `Status.NumberReady` can
stay at the desired count while the rollout does not end. With `OnDelete`, the controller does not replace pods. Pods
move to the new template only when something outside the controller deletes them, so the handlers cannot wait for it. To
track an `OnDelete` rollout, use `WithCustomConvergeStatus` and `WithCustomGraceStatus`.

A DaemonSet has no scale-down to wait for. The controller counts at most one pod for each node that must run one, and
`kubectl rollout status` does not wait for `Status.NumberMisscheduled`, so the handlers do not check it.

### ConvergingStatus

`DefaultConvergingStatusHandler` reports `Healthy` when the DaemonSet is healthy. Otherwise it reports:

| Status                            | Condition                                                    |
| --------------------------------- | ------------------------------------------------------------ |
| `Creating` or `Updating`          | The controller has not observed the current spec.            |
| `Creating`, `Updating`, `Scaling` | `Status.NumberReady` differs from the desired pod count.     |
| `Creating` or `Updating`          | All desired pods are ready, but the rollout is not complete. |

The status follows the operation of the apply. `Created` gives `Creating`. `Updated` gives `Updating`. `None` gives
`Updating` in the first and third rows, and `Scaling` in the second row.

### GraceStatus

`DefaultGraceStatusHandler` categorizes health as:

| Status     | Condition                                                                                                                   |
| ---------- | --------------------------------------------------------------------------------------------------------------------------- |
| `Healthy`  | The DaemonSet is healthy.                                                                                                   |
| `Down`     | The desired pod count is more than zero and no pods are ready.                                                              |
| `Degraded` | All other states: the spec is not observed, the ready count differs from the desired count, or the rollout is not complete. |

Because the two handlers use the same rule, a DaemonSet that does not converge before the grace period expires reports
`Degraded` or `Down`. Override the handlers with `WithCustomConvergeStatus` and `WithCustomGraceStatus`.

## Full Example

```go
func NodeAgentMutation(version string, hostLogPath string) daemonset.Mutation {
    return daemonset.Mutation{
        Name:    "node-agent",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *daemonset.Mutator) error {
            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.SetServiceAccountName("node-agent-sa")
                e.EnsureVolume(corev1.Volume{
                    Name: "host-logs",
                    VolumeSource: corev1.VolumeSource{
                        HostPath: &corev1.HostPathVolumeSource{Path: hostLogPath},
                    },
                })
                return nil
            })

            m.EditContainers(selectors.ContainerNamed("collector"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_PATH", Value: "/host/logs"})
                e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("100m"))
                e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("128Mi"))
                e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
                    Name:      "host-logs",
                    MountPath: "/host/logs",
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

**DaemonSets are node-scoped.** Unlike Deployments, a DaemonSet runs one pod per qualifying node. Use node selectors,
tolerations, and affinities to control which nodes run the pods.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**DaemonSets are deleted on suspend.** There is no in-place scale-to-zero. Override `WithCustomSuspendDeletionDecision`
if you need the resource to remain in the cluster when the component is suspended.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
