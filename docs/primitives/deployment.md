# Deployment Primitive

The `deployment` primitive is the framework's built-in workload abstraction for managing Kubernetes `Deployment` resources. It integrates fully with the component lifecycle and provides a rich mutation API for managing containers, pod specs, and metadata.

## Capabilities

| Capability            | Detail                                                                                          |
|-----------------------|-------------------------------------------------------------------------------------------------|
| **Health tracking**   | Monitors `ReadyReplicas` and reports `Healthy`, `Creating`, `Updating`, `Scaling`, or `Failing` |
| **Graceful rollouts** | Detects stalled or failing rollouts via configurable grace periods                              |
| **Suspension**        | Scales to zero replicas; reports `Suspending` / `Suspended`                                     |
| **Mutation pipeline** | Typed editors for metadata, deployment spec, pod spec, and containers                           |
| **Flavors**           | Preserves externally-managed fields (labels, annotations, pod template metadata)                |

## Building a Deployment Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"

base := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-server",
        Namespace: owner.Namespace,
    },
    Spec: appsv1.DeploymentSpec{
        // baseline spec
    },
}

resource, err := deployment.NewBuilder(base).
    WithFieldApplicationFlavor(deployment.PreserveCurrentLabels).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `Deployment` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *deployment.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func TracingMutation(version string, enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "tracing",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *deployment.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "jaeger-agent",
                Image: "jaegertracing/jaeger-agent:1.28",
            })
            return nil
        },
    }
}
```

### Version-gated mutations

Pass a `[]feature.VersionConstraint` to gate on a semver range. `VersionConstraint` is an interface — implement it using the `github.com/Masterminds/semver/v3` library or any other mechanism:

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyAuthMutation(version string, enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name: "legacy-auth-header",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ).When(enabled),
        Mutate: func(m *deployment.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("api"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "AUTH_HEADER", Value: "X-Legacy-Token"})
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the order they are recorded. This ensures structural consistency across mutations.

| Step | Category | What it affects |
|---|---|---|
| 1 | Deployment metadata edits | Labels and annotations on the `Deployment` object |
| 2 | DeploymentSpec edits | Replicas, progress deadline, revision history, etc. |
| 3 | Pod template metadata edits | Labels and annotations on the pod template |
| 4 | Pod spec edits | Volumes, tolerations, node selectors, service account, security context |
| 5 | Regular container presence | Adding or removing containers from `spec.containers` |
| 6 | Regular container edits | Env vars, args, resources (snapshot taken after step 5) |
| 7 | Init container presence | Adding or removing containers from `spec.initContainers` |
| 8 | Init container edits | Env vars, args, resources (snapshot taken after step 7) |

Container edits (steps 6 and 8) are evaluated against a snapshot taken *after* presence operations in the same mutation. This means a single mutation can add a container and then configure it without selector resolution issues.

## Editors

### DeploymentSpecEditor

Controls deployment-level settings via `m.EditDeploymentSpec`.

Available methods: `SetReplicas`, `SetPaused`, `SetMinReadySeconds`, `SetRevisionHistoryLimit`, `SetProgressDeadlineSeconds`, `Raw`.

```go
m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
    e.SetReplicas(3)
    e.SetProgressDeadlineSeconds(600)
    return nil
})
```

For fields not covered by the typed API (such as update strategy), use `Raw()`:

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

Available methods: `SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`, `EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`, `SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`.

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

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`. Always used in combination with a [selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`, `RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

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

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `Deployment` object itself, or `m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
// On the Deployment itself
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})

// On the pod template
m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureAnnotation("prometheus.io/scrape", "true")
    return nil
})
```

### Raw Escape Hatch

All editors provide a `.Raw()` method for direct access to the underlying Kubernetes struct when the typed API is insufficient. The mutation remains scoped to the editor's target — you cannot accidentally modify unrelated parts of the spec.

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
|-------------------------------|---------------------------------------------------------------|
| `EnsureReplicas(n)`           | `EditDeploymentSpec` → `SetReplicas(n)`                       |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Full Example: Adding a Sidecar

```go
func LoggingSidecarMutation(version string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "logging-sidecar",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *deployment.Mutator) error {
            // Step 1: ensure the sidecar exists (presence operation, step 5)
            m.EnsureContainer(corev1.Container{
                Name:  "logger",
                Image: "fluent/fluent-bit:3.0",
            })

            // Step 2: configure it (evaluated after step 1, step 6)
            m.EditContainers(selectors.ContainerNamed("logger"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
                // Volume mounts are not in the typed API — use Raw()
                e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
                    Name:      "varlog",
                    MountPath: "/var/log",
                })
                return nil
            })

            // Step 3: add the shared volume to the pod spec (step 4, runs before containers)
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

Note: although `EditPodSpec` is called after `EnsureContainer` in the source, it is applied in step 4 (before container presence in step 5) per the internal ordering. Order your source calls for readability — the framework handles execution order.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use `feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first. The internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in the same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can cause unexpected behavior if sidecar containers are present.