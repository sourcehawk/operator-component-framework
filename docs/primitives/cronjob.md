# CronJob Primitive

The `cronjob` primitive is the framework's built-in integration abstraction for managing Kubernetes `CronJob` resources.
It integrates with the component lifecycle through the Operational and Suspendable concepts, and provides a rich
mutation API for managing the CronJob schedule, job template, pod spec, and containers.

## Capabilities

| Capability               | Detail                                                                                      |
| ------------------------ | ------------------------------------------------------------------------------------------- |
| **Operational tracking** | Reports `OperationPending` (never scheduled) or `Operational` (has scheduled at least once) |
| **Suspension**           | Sets `spec.suspend = true`; reports `Suspending` (active jobs running) / `Suspended`        |
| **Mutation pipeline**    | Typed editors for metadata, CronJob spec, Job spec, pod spec, and containers                |

## Server-Side Apply

The CronJob primitive reconciles resources using **Server-Side Apply** (SSA). Only fields declared by the operator are
sent; server-managed defaults, fields set by other controllers, and values written by webhooks are left untouched. Field
ownership is tracked automatically by the Kubernetes API server.

## Building a CronJob Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/cronjob"

base := &batchv1.CronJob{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "data-cleanup",
        Namespace: owner.Namespace,
    },
    Spec: batchv1.CronJobSpec{
        Schedule: "0 2 * * *",
        JobTemplate: batchv1.JobTemplateSpec{
            Spec: batchv1.JobSpec{
                Template: corev1.PodTemplateSpec{
                    Spec: corev1.PodSpec{
                        Containers: []corev1.Container{
                            {Name: "cleanup", Image: "cleanup:latest"},
                        },
                        RestartPolicy: corev1.RestartPolicyOnFailure,
                    },
                },
            },
        },
    },
}

resource, err := cronjob.NewBuilder(base).
    WithMutation(MyScheduleMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `CronJob` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

```go
func MyScheduleMutation(version string) cronjob.Mutation {
    return cronjob.Mutation{
        Name:    "my-schedule",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *cronjob.Mutator) error {
            m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
                e.SetSchedule("0 */6 * * *")
                e.SetConcurrencyPolicy(batchv1.ForbidConcurrent)
                return nil
            })
            return nil
        },
    }
}
```

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func TimeZoneMutation(version string, enabled bool) cronjob.Mutation {
    return cronjob.Mutation{
        Name:    "timezone",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *cronjob.Mutator) error {
            m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
                e.SetTimeZone("America/New_York")
                return nil
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded.

| Step | Category                    | What it affects                                                                         |
| ---- | --------------------------- | --------------------------------------------------------------------------------------- |
| 1    | CronJob metadata edits      | Labels and annotations on the `CronJob` object                                          |
| 2    | CronJobSpec edits           | Schedule, concurrency policy, time zone, history limits                                 |
| 3    | JobSpec edits               | Completions, parallelism, backoff limit, TTL                                            |
| 4    | Pod template metadata edits | Labels and annotations on the pod template                                              |
| 5    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context                 |
| 6    | Regular container presence  | Adding or removing containers from `spec.jobTemplate.spec.template.spec.containers`     |
| 7    | Regular container edits     | Env vars, args, resources (snapshot taken after step 6)                                 |
| 8    | Init container presence     | Adding or removing containers from `spec.jobTemplate.spec.template.spec.initContainers` |
| 9    | Init container edits        | Env vars, args, resources (snapshot taken after step 8)                                 |

Container edits (steps 7 and 9) are evaluated against a snapshot taken _after_ presence operations in the same mutation.
This means a single mutation can add a container and then configure it without selector resolution issues.

## Relevant Editors

### CronJobSpecEditor

Controls CronJob-level settings via `m.EditCronJobSpec`.

Available methods: `SetSchedule`, `SetConcurrencyPolicy`, `SetStartingDeadlineSeconds`, `SetSuccessfulJobsHistoryLimit`,
`SetFailedJobsHistoryLimit`, `SetTimeZone`, `Raw`.

```go
m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
    e.SetSchedule("0 2 * * *")
    e.SetConcurrencyPolicy(batchv1.ForbidConcurrent)
    e.SetFailedJobsHistoryLimit(1)
    return nil
})
```

Note: no typed helper is provided for `spec.suspend`; it can be set via `Raw()` if needed, but suspension should
typically be handled via the framework's suspend mechanism.

### JobSpecEditor

Controls the embedded job template spec via `m.EditJobSpec`.

Available methods: `SetCompletions`, `SetParallelism`, `SetBackoffLimit`, `SetActiveDeadlineSeconds`,
`SetTTLSecondsAfterFinished`, `SetCompletionMode`, `Raw`.

```go
m.EditJobSpec(func(e *editors.JobSpecEditor) error {
    e.SetBackoffLimit(3)
    e.SetTTLSecondsAfterFinished(3600)
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
    e.SetServiceAccountName("cleanup-sa")
    e.Raw().RestartPolicy = corev1.RestartPolicyOnFailure
    return nil
})
```

### ContainerEditor

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`. Always used in combination with a
[selector](../primitives.md#container-selectors).

Available methods: `EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`,
`RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`.

```go
m.EditContainers(selectors.ContainerNamed("cleanup"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "DRY_RUN", Value: "false"})
    e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("256Mi"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `CronJob` object itself, or
`m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
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

## Operational Status

The CronJob primitive reports operational status based on the CronJob's scheduling history:

| Status             | Condition                        |
| ------------------ | -------------------------------- |
| `OperationPending` | `Status.LastScheduleTime == nil` |
| `Operational`      | `Status.LastScheduleTime != nil` |

Failures are reported on the spawned Job resources, not on the CronJob itself.

## Suspension

When the component is suspended, the CronJob primitive sets `spec.suspend = true`. This prevents the CronJob controller
from creating new Job objects. Existing active jobs continue to run.

| Status       | Condition                                            |
| ------------ | ---------------------------------------------------- |
| `Suspended`  | `spec.suspend == true` and no active jobs            |
| `Suspending` | `spec.suspend == true` but active jobs still running |
| `Suspending` | Waiting for suspend flag to be applied               |

On unsuspend, the desired state (without `spec.suspend = true`) is applied via SSA, allowing the CronJob to resume
scheduling.

The CronJob is never deleted on suspend (`DeleteOnSuspend = false`).

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in
the same mutation resolve correctly.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
