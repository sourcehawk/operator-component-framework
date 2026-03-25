# Job Primitive

The `job` primitive is the framework's built-in task abstraction for managing Kubernetes `Job` resources. It integrates
fully with the component lifecycle and provides a rich mutation API for managing containers, pod specs, and metadata —
following the same pod-template mutation pattern as the Deployment primitive.

## Capabilities

| Capability              | Detail                                                                                          |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| **Completion tracking** | Monitors Job conditions and reports `Completed`, `TaskRunning`, `TaskPending`, or `TaskFailing` |
| **Suspension**          | Sets `spec.suspend=true` or deletes the Job (default); reports `Suspending` / `Suspended`       |
| **Mutation pipeline**   | Typed editors for metadata, job spec, pod spec, and containers                                  |

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
                    {Name: "migrate", Image: "my-app-migration:latest"},
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

Mutations are the primary mechanism for modifying a `Job` beyond its baseline. Each mutation is a named function that
receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) job.Mutation {
    return job.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *job.Mutator) error {
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
func TracingMutation(version string, enabled bool) job.Mutation {
    return job.Mutation{
        Name:    "tracing",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *job.Mutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{
                Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
                Value: "http://otel-collector:4317",
            })
            return nil
        },
    }
}
```

### Version-gated mutations

Pass a `[]feature.VersionConstraint` to gate on a semver range:

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyMigrationMutation(version string) job.Mutation {
    return job.Mutation{
        Name: "legacy-migration-format",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *job.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("migrate"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "MIGRATION_FORMAT", Value: "v1"})
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded. This ensures structural consistency across mutations.

| Step | Category                    | What it affects                                                         |
| ---- | --------------------------- | ----------------------------------------------------------------------- |
| 1    | Job metadata edits          | Labels and annotations on the `Job` object                              |
| 2    | JobSpec edits               | Completions, parallelism, backoff limit, deadline, etc.                 |
| 3    | Pod template metadata edits | Labels and annotations on the pod template                              |
| 4    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence  | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits     | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence     | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits        | Env vars, args, resources (snapshot taken after step 7)                 |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same mutation.
This means a single mutation can add a container and then configure it without selector resolution issues.

## Editors

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

For fields not covered by the typed API, use `Raw()`:

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

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`. Always used in combination with a
[selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("migrate"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "DB_HOST", Value: "postgres:5432"})
    e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("500m"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `Job` object itself, or
`m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})
```

## Convenience Methods

The `Mutator` exposes convenience wrappers that target all containers at once:

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |

## Suspension

Jobs use the Task lifecycle for suspension, which differs from Workloads:

- **Default behavior**: `DefaultDeleteOnSuspendHandler` returns `true`, meaning the Job is deleted from the cluster
  during suspension.
- **Suspend mutation**: `DefaultSuspendMutationHandler` sets `spec.suspend=true`, which prevents the Job controller from
  creating new pods while allowing existing pods to complete.
- **Suspension status**: `DefaultSuspensionStatusHandler` checks if `spec.suspend=true` and `status.active=0`.

Override any of these via the Builder:

```go
resource, err := job.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(j *batchv1.Job) bool {
        return false // Keep the Job in the cluster when suspended
    }).
    Build()
```

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
The internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in
the same mutation resolve correctly and reconciliation remains idempotent.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if init containers or sidecar containers are present.

**Jobs are deleted on suspend by default.** Unlike Deployments which scale to zero, Jobs are deleted during suspension.
Override `WithCustomSuspendDeletionDecision` if you need to keep the Job resource in the cluster.
