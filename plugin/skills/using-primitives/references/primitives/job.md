# Job Primitive

The `job` primitive wraps a Kubernetes `Job` and provides completion tracking, suspension, and a typed mutation API for
managing job spec, pod spec, and containers as part of the component lifecycle.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                   |
| ------------------------------------------------------------ | -------------------------------------------------------- |
| `Completable`                                                | `Completed`, `TaskRunning`, `TaskPending`, `TaskFailing` |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`           |
| `Guardable`                                                  | `Blocked`                                                |
| `DataExtractable`                                            | _(side-effecting, no status)_                            |

## Building a Job Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/job"

base := &batchv1.Job{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "db-migration",
        Namespace: owner.Namespace,
    },
    Spec: batchv1.JobSpec{
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                RestartPolicy: corev1.RestartPolicyOnFailure,
                Containers: []corev1.Container{
                    {Name: "migrate", Image: "migration-tool:latest"},
                },
            },
        },
    },
}

resource, err := job.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `job.Mutation` that receives a `*job.Mutator` and records edits through typed editors.

```go
func MigrationConfigMutation(version string) job.Mutation {
    return job.Mutation{
        Name:    "migration-config",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *job.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("migrate"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "DB_HOST", Value: "db:5432"})
                return nil
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
| 1    | Object metadata edits       | Labels and annotations on the `Job` object                              |
| 2    | JobSpec edits               | Completions, parallelism, backoff limit, deadline, etc.                 |
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

### JobSpecEditor

Controls job-level settings via `m.EditJobSpec`.

Available methods: `SetCompletions`, `SetParallelism`, `SetBackoffLimit`, `SetActiveDeadlineSeconds`,
`SetTTLSecondsAfterFinished`, `SetCompletionMode`, `Raw`.

```go
m.EditJobSpec(func(e *editors.JobSpecEditor) error {
    e.SetBackoffLimit(3)
    e.SetActiveDeadlineSeconds(600)
    return nil
})
```

Use `Raw()` for fields the typed API does not cover:

```go
m.EditJobSpec(func(e *editors.JobSpecEditor) error {
    e.Raw().Suspend = ptr.To(true)
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
    e.SetServiceAccountName("migration-sa")
    e.EnsureVolume(corev1.Volume{
        Name: "config",
        VolumeSource: corev1.VolumeSource{
            ConfigMap: &corev1.ConfigMapVolumeSource{
                LocalObjectReference: corev1.LocalObjectReference{Name: "migration-config"},
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
m.EditContainers(selectors.ContainerNamed("migrate"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "DB_HOST", Value: "db:5432"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `Job` itself or `m.EditPodTemplateMetadata` for the
pod template.

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

## Workload-Kind-Agnostic Mutations

The `job.Mutator` does not implement `primitives.WorkloadMutator` and therefore does not have a `LiftMutation` adapter.
The `WorkloadMutator` interface targets Deployment, StatefulSet, and DaemonSet. Write shared mutation logic as a plain
function accepting `*job.Mutator` and call it directly.

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the cross-kind pattern.

## Suspension

Jobs use the `Completable` lifecycle rather than `Alive`. The suspension behavior differs from Workload primitives:

- **Default behavior**: `DefaultDeleteOnSuspendHandler` returns `true`, meaning the Job is deleted from the cluster
  during suspension.
- **Suspend mutation**: `DefaultSuspendMutationHandler` sets `spec.suspend=true`, which prevents the Job controller from
  creating new pods while allowing existing pods to complete.
- **Suspension status**: `DefaultSuspensionStatusHandler` reports `Suspending` if `spec.suspend=true` but active pods
  remain, and `Suspended` once `spec.suspend=true` and `status.active==0`.

Override any handler via `WithCustomSuspendDeletionDecision`, `WithCustomSuspendMutation`, or `WithCustomSuspendStatus`
on the builder:

```go
resource, err := job.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(j *batchv1.Job) bool {
        return false // keep the Job in the cluster when suspended
    }).
    Build()
```

## Full Example

```go
func MigrationMutation(version string, dbHost string) job.Mutation {
    return job.Mutation{
        Name:    "migration",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *job.Mutator) error {
            m.EditJobSpec(func(e *editors.JobSpecEditor) error {
                e.SetBackoffLimit(3)
                e.SetActiveDeadlineSeconds(300)
                return nil
            })

            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.SetServiceAccountName("migration-sa")
                return nil
            })

            m.EditContainers(selectors.ContainerNamed("migrate"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "DB_HOST", Value: dbHost})
                e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
                e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("256Mi"))
                return nil
            })

            return nil
        },
    }
}
```

## Guidance

**Jobs are deleted on suspend by default.** Unlike Deployments which scale to zero, Jobs are deleted during suspension.
Override `WithCustomSuspendDeletionDecision` if you need the Job resource to remain in the cluster.

**Set `RestartPolicy` in the baseline.** Kubernetes requires `spec.template.spec.restartPolicy` to be `OnFailure` or
`Never` for Jobs. Set it in the desired object passed to `NewBuilder`.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if init containers or sidecar containers are present.
