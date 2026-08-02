# CronJob Primitive

The `cronjob` primitive wraps a Kubernetes `CronJob` and provides operational status tracking, grace handling,
suspension, and a typed mutation API for managing the schedule, job template, pod spec, and containers as part of the
component lifecycle.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                |
| ------------------------------------------------------------ | ----------------------------------------------------- |
| `Operational`                                                | `Operational`, `OperationPending`, `OperationFailing` |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                         |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`        |
| `Guardable`                                                  | `Blocked`                                             |
| `DataExtractable`                                            | _(side-effecting, no status)_                         |

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
                        RestartPolicy: corev1.RestartPolicyOnFailure,
                        Containers: []corev1.Container{
                            {Name: "cleanup", Image: "cleanup-tool:latest"},
                        },
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

Each mutation is a named `cronjob.Mutation` that receives a `*cronjob.Mutator` and records edits through typed editors.

```go
func ScheduleMutation(version string) cronjob.Mutation {
    return cronjob.Mutation{
        Name:    "schedule",
        Feature: feature.NewVersionGate(version, nil),
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

See [the mutation system](../primitives.md#the-mutation-system),
[boolean gating](../primitives.md#boolean-gated-mutations), and
[version gating](../primitives.md#version-gated-mutations).

For all primitives, desired state is reconciled via [Server-Side Apply](../primitives.md#server-side-apply).

## Internal Mutation Ordering

Within each feature, edits run in this fixed category order:

| Step | Category                    | What it affects                                                                         |
| ---- | --------------------------- | --------------------------------------------------------------------------------------- |
| 1    | Object metadata edits       | Labels and annotations on the `CronJob` object                                          |
| 2    | CronJobSpec edits           | Schedule, concurrency policy, time zone, history limits                                 |
| 3    | JobSpec edits               | Completions, parallelism, backoff limit, TTL                                            |
| 4    | Pod template metadata edits | Labels and annotations on the pod template                                              |
| 5    | Pod spec edits              | Volumes, tolerations, node selectors, service account, security context                 |
| 6    | Regular container presence  | Adding or removing containers from `spec.jobTemplate.spec.template.spec.containers`     |
| 7    | Regular container edits     | Env vars, args, resources (snapshot taken after step 6)                                 |
| 8    | Init container presence     | Adding or removing containers from `spec.jobTemplate.spec.template.spec.initContainers` |
| 9    | Init container edits        | Env vars, args, resources (snapshot taken after step 8)                                 |

Container edits (steps 7 and 9) are evaluated against a snapshot taken _after_ presence operations in the same feature.

## Relevant Editors

For the generic editor and selector concepts, see [mutation editors](../primitives.md#mutation-editors) and
[container selectors](../primitives.md#container-selectors).

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

!!! note "No typed helper for `spec.suspend`"

    `spec.suspend` is not exposed by the typed API. Use `Raw()` if you need to set it directly, but prefer the
    framework's suspend mechanism instead.

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
    return nil
})
```

### ContainerEditor

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`, combined with a
[container selector](../primitives.md#container-selectors).

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

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `CronJob` itself or `m.EditPodTemplateMetadata` for
the pod template.

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

## Workload-Kind-Agnostic Mutations

The `cronjob.Mutator` does not implement `primitives.WorkloadMutator` and therefore does not have a `LiftMutation`
adapter. The `WorkloadMutator` interface targets Deployment, StatefulSet, and DaemonSet. Write shared mutation logic as
a plain function accepting `*cronjob.Mutator` and call it directly.

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the cross-kind pattern.

## Data Extraction

`cronjob.ExtractInto` declares that this CronJob produces the value of a data cell, which is how scheduling state
observed on the object reaches the resources that consume it. The function receives a value copy of the reconciled
CronJob after each sync cycle:

```go
lastSchedule := concepts.NewData[metav1.Time]("cleanup-last-schedule")

builder := cronjob.NewBuilder(base)
cronjob.ExtractInto(builder, lastSchedule, func(cj batchv1.CronJob) (metav1.Time, error) {
    if cj.Status.LastScheduleTime == nil {
        return metav1.Time{}, nil
    }
    return *cj.Status.LastScheduleTime, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(lastSchedule)` or read it
opportunistically with `WithOptionalData(lastSchedule)`. See [Declared Data](../component.md#declared-data).

## Operational Status

`DefaultOperationalStatusHandler` always reports `Operational`. A CronJob is a passive scheduler: once it exists in the
cluster it is functioning correctly regardless of whether it has fired yet. Schedule intervals may be longer than the
component's grace period, so treating a never-scheduled CronJob as pending would produce false degradation signals.
Failures are reported on the spawned Job resources, not on the CronJob itself.

Override with `WithCustomOperationalStatus` if you need visibility into whether the CronJob has executed:

```go
cronjob.NewBuilder(base).
    WithCustomOperationalStatus(func(_ concepts.ConvergingOperation, cj *batchv1.CronJob) (concepts.OperationalStatusWithReason, error) {
        if cj.Status.LastScheduleTime == nil {
            return concepts.OperationalStatusWithReason{
                Status: concepts.OperationalStatusPending,
                Reason: "CronJob has not fired yet",
            }, nil
        }
        return concepts.OperationalStatusWithReason{
            Status: concepts.OperationalStatusOperational,
            Reason: "CronJob has fired at least once",
        }, nil
    })
```

## Grace Status

`DefaultGraceStatusHandler` always reports `Healthy`. A CronJob is considered healthy once it exists and is not
suspended. Override with `WithCustomGraceStatus` if your CronJob has specific health requirements.

## Suspension

When the component is suspended, the CronJob sets `spec.suspend = true`, preventing new Jobs from being created.
Existing active jobs continue running.

| Status       | Condition                                            |
| ------------ | ---------------------------------------------------- |
| `Suspending` | `spec.suspend == true` but active jobs still running |
| `Suspended`  | `spec.suspend == true` and no active jobs            |
| `Suspending` | Waiting for the suspend flag to be applied           |

The CronJob is never deleted on suspend (`DefaultDeleteOnSuspendHandler` returns `false`). On unsuspend, the desired
state without `spec.suspend = true` is reapplied via Server-Side Apply, and the CronJob resumes scheduling.

## Full Example

```go
func CleanupMutation(version string, schedule string) cronjob.Mutation {
    return cronjob.Mutation{
        Name:    "cleanup-schedule",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *cronjob.Mutator) error {
            // CronJob spec: schedule and concurrency
            m.EditCronJobSpec(func(e *editors.CronJobSpecEditor) error {
                e.SetSchedule(schedule)
                e.SetConcurrencyPolicy(batchv1.ForbidConcurrent)
                e.SetFailedJobsHistoryLimit(3)
                e.SetSuccessfulJobsHistoryLimit(1)
                return nil
            })

            // Job spec: backoff and TTL
            m.EditJobSpec(func(e *editors.JobSpecEditor) error {
                e.SetBackoffLimit(2)
                e.SetTTLSecondsAfterFinished(3600)
                return nil
            })

            // Pod spec: service account
            m.EditPodSpec(func(e *editors.PodSpecEditor) error {
                e.SetServiceAccountName("cleanup-sa")
                return nil
            })

            // Container: configuration
            m.EditContainers(selectors.ContainerNamed("cleanup"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "DRY_RUN", Value: "false"})
                e.SetResourceLimit(corev1.ResourceCPU, resource.MustParse("200m"))
                e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("256Mi"))
                return nil
            })

            return nil
        },
    }
}
```

The four nesting levels mirror the object structure: `CronJobSpec` -> `JobSpec` -> `PodSpec` -> `ContainerEditor`. Each
editor targets one level of that nesting.

## Guidance

**CronJobs are passive schedulers.** They do not run actively; the CronJob controller creates Job objects on schedule.
Model health around the spawned Jobs, not the CronJob resource itself.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Set `RestartPolicy` in the baseline.** Kubernetes requires `spec.jobTemplate.spec.template.spec.restartPolicy` to be
`OnFailure` or `Never`. Set it in the desired object passed to `NewBuilder`.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
