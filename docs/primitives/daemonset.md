# DaemonSet Primitive

The `daemonset` primitive is the framework's built-in workload abstraction for managing Kubernetes `DaemonSet`
resources. It integrates fully with the component lifecycle and provides a rich mutation API for managing containers,
pod specs, and metadata.

## Capabilities

| Capability            | Detail                                                                                                                                      |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| **Health tracking**   | Verifies `ObservedGeneration` matches `Generation` before evaluating `NumberReady`; reports `Healthy`, `Creating`, `Updating`, or `Scaling` |
| **Graceful rollouts** | Reports rollout progress via `GraceStatus` for use with component-level grace periods (for example, configured with `WithGracePeriod`)      |
| **Suspension**        | Deletes the DaemonSet on suspend; reports `Suspended`                                                                                       |
| **Mutation pipeline** | Typed editors for metadata, DaemonSet spec, pod spec, and containers                                                                        |
| **Flavors**           | Preserves externally-managed fields (labels, annotations, pod template metadata)                                                            |

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
            // baseline pod template
        },
    },
}

resource, err := daemonset.NewBuilder(base).
    WithFieldApplicationFlavor(daemonset.PreserveCurrentLabels).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `DaemonSet` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) daemonset.Mutation {
    return daemonset.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *daemonset.Mutator) error {
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
func MonitoringMutation(version string, enabled bool) daemonset.Mutation {
    return daemonset.Mutation{
        Name:    "monitoring",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *daemonset.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "metrics-exporter",
                Image: "prom/node-exporter:v1.3.1",
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded. This ensures structural consistency across mutations.

| Step | Category                    | What it affects                                                         |
| ---- | --------------------------- | ----------------------------------------------------------------------- |
| 1    | DaemonSet metadata edits    | Labels and annotations on the `DaemonSet` object                        |
| 2    | DaemonSetSpec edits         | Update strategy, min ready seconds, revision history limit              |
| 3    | Pod template metadata edits | Labels and annotations on the pod template                              |
| 4    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence  | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits     | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence     | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits        | Env vars, args, resources (snapshot taken after step 7)                 |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same mutation.
This means a single mutation can add a container and then configure it without selector resolution issues.

## Editors

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

For fields not covered by the typed API, use `Raw()`:

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

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`. Always used in combination with a
[selector](../primitives.md#container-selectors).

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

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `DaemonSet` object itself, or
`m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})
```

### Raw Escape Hatch

All editors provide a `.Raw()` method for direct access to the underlying Kubernetes struct when the typed API is
insufficient.

## Convenience Methods

The `Mutator` also exposes convenience wrappers that target all containers at once:

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Suspension

DaemonSets have no replicas field, so there is no clean in-place pause mechanism. By default, the DaemonSet is
**deleted** when the component is suspended and recreated when unsuspended.

- `DefaultDeleteOnSuspendHandler` returns `true`
- `DefaultSuspendMutationHandler` is a no-op
- `DefaultSuspensionStatusHandler` always reports `Suspended` with reason `"DaemonSet deleted on suspend"`

Override these handlers via `WithCustomSuspendDeletionDecision`, `WithCustomSuspendMutation`, and
`WithCustomSuspendStatus` if a different suspension strategy is required.

## Status Handlers

### ConvergingStatus

`DefaultConvergingStatusHandler` considers a DaemonSet ready when `Status.NumberReady >= Status.DesiredNumberScheduled`
and `DesiredNumberScheduled > 0`. When `DesiredNumberScheduled` is zero (no matching nodes) and the controller has
observed the current generation (`ObservedGeneration >= Generation`), the DaemonSet is considered converged with the
reason "No nodes match the DaemonSet node selector".

### GraceStatus

`DefaultGraceStatusHandler` categorizes health as:

| Status     | Condition                                                                                                                                              |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Healthy`  | `DesiredNumberScheduled == 0` and `ObservedGeneration >= Generation` — no nodes match the selector                                                     |
| `Degraded` | `DesiredNumberScheduled == 0` but controller has not observed latest generation, or `DesiredNumberScheduled > 0 && NumberReady >= 1` but below desired |
| `Down`     | `DesiredNumberScheduled > 0 && NumberReady == 0`                                                                                                       |

The `Healthy` status for zero desired pods reflects that having no matching nodes is a valid configuration state, not a
failure. The generation check ensures the controller has observed the latest spec before declaring health.

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

**DaemonSets are node-scoped.** Unlike Deployments, DaemonSets run one pod per qualifying node. Use node selectors,
tolerations, and affinities in the pod spec to control which nodes run the DaemonSet pods.
