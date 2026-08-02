# HorizontalPodAutoscaler Primitive

The `hpa` primitive wraps `autoscaling/v2 HorizontalPodAutoscaler` and integrates it with the component lifecycle as an
Operational, Graceful, and Suspendable resource.

## Capabilities

The interfaces below are from [`pkg/component/concepts`](../primitives.md#lifecycle-interfaces). The values in the table
are the runtime strings that appear in conditions.

| Interface         | Reported status values                                | Notes                                       |
| ----------------- | ----------------------------------------------------- | ------------------------------------------- |
| `Operational`     | `Operational`, `OperationPending`, `OperationFailing` | Inspects `ScalingActive` and `AbleToScale`  |
| `Graceful`        | `Healthy`, `Degraded`, `Down`                         | Same HPA conditions, evaluated post-grace   |
| `Suspendable`     | `PendingSuspension`, `Suspending`, `Suspended`        | Delete-on-suspend by default                |
| `Guardable`       | `Blocked`                                             | Optional runtime precondition               |
| `DataExtractable` | _(side-effecting, no status)_                         | Read generated fields after each sync cycle |

## Building an HPA Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/hpa"

base := &autoscalingv2.HorizontalPodAutoscaler{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "backend-hpa",
        Namespace: owner.Namespace,
    },
    Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
        ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
            APIVersion: "apps/v1",
            Kind:       "Deployment",
            Name:       "backend",
        },
        MinReplicas: ptr.To(int32(2)),
        MaxReplicas: 10,
    },
}

resource, err := hpa.NewBuilder(base).
    WithMutation(CPUScalingMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are named functions that receive a `*hpa.Mutator` and record edit intent through typed editors. For a full
explanation of the mutation system, boolean-gated mutations, and version-gated mutations see
[The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A concise version-gated example:

```go
var newScalingConstraint = semver.MustConstraint(">= 2.0.0")

func AggressiveScalingMutation(version string, enabled bool) hpa.Mutation {
    return hpa.Mutation{
        Name: "aggressive-scaling",
        Feature: feature.NewVersionGate(version, []feature.VersionConstraint{newScalingConstraint}).
            When(enabled),
        Mutate: func(m *hpa.Mutator) error {
            m.EditHPASpec(func(e *editors.HPASpecEditor) error {
                e.SetMaxReplicas(20)
                e.SetBehavior(&autoscalingv2.HorizontalPodAutoscalerBehavior{
                    ScaleDown: &autoscalingv2.HPAScalingRules{
                        StabilizationWindowSeconds: ptr.To(int32(60)),
                    },
                })
                return nil
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits execute in a fixed category order regardless of the order they are recorded:

| Step | Category       | What it affects                                                |
| ---- | -------------- | -------------------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `HorizontalPodAutoscaler` object |
| 2    | HPA spec edits | Scale target ref, min/max replicas, metrics, behavior          |

Features apply in registration order. Later features observe the HPA as modified by all earlier ones.

## Relevant Editors

For the full method list of any editor see the
[Go API reference](https://pkg.go.dev/github.com/sourcehawk/operator-component-framework/pkg/mutation/editors). The
generic concept is explained in [Mutation Editors](../primitives.md#mutation-editors).

### HPASpecEditor

Controls the HPA spec via `m.EditHPASpec`.

Available methods: `SetScaleTargetRef`, `SetMinReplicas`, `SetMaxReplicas`, `EnsureMetric`, `RemoveMetric`,
`SetBehavior`, `Raw`.

```go
m.EditHPASpec(func(e *editors.HPASpecEditor) error {
    e.SetMinReplicas(ptr.To(int32(2)))
    e.SetMaxReplicas(10)
    e.EnsureMetric(autoscalingv2.MetricSpec{
        Type: autoscalingv2.ResourceMetricSourceType,
        Resource: &autoscalingv2.ResourceMetricSource{
            Name: corev1.ResourceCPU,
            Target: autoscalingv2.MetricTarget{
                Type:               autoscalingv2.UtilizationMetricType,
                AverageUtilization: ptr.To(int32(80)),
            },
        },
    })
    return nil
})
```

#### EnsureMetric identity rules

`EnsureMetric` upserts by full metric identity. If a matching entry exists it is replaced; otherwise the metric is
appended.

| Metric type         | Match key                                                                                                 |
| ------------------- | --------------------------------------------------------------------------------------------------------- |
| `Resource`          | `Resource.Name` (e.g. `cpu`, `memory`)                                                                    |
| `Pods`              | `Pods.Metric.Name` + `Pods.Metric.Selector` (`nil` is a distinct identity)                                |
| `Object`            | `Object.DescribedObject` (`APIVersion`, `Kind`, `Name`) + `Object.Metric.Name` + `Object.Metric.Selector` |
| `ContainerResource` | `ContainerResource.Name` + `ContainerResource.Container`                                                  |
| `External`          | `External.Metric.Name` + `External.Metric.Selector` (`nil` is a distinct identity)                        |

#### RemoveMetric

`RemoveMetric(type, name)` removes all metrics matching the given type and name. For `ContainerResource` metrics all
container variants of the named resource are removed. For fine-grained removal of a single identity, use `Raw()` and
modify the slice directly.

#### SetBehavior

`SetBehavior` sets the autoscaling behavior (stabilization windows, scaling policies). Pass `nil` to remove custom
behavior and revert to Kubernetes defaults.

```go
m.EditHPASpec(func(e *editors.HPASpecEditor) error {
    e.SetBehavior(&autoscalingv2.HorizontalPodAutoscalerBehavior{
        ScaleDown: &autoscalingv2.HPAScalingRules{
            StabilizationWindowSeconds: ptr.To(int32(300)),
        },
    })
    return nil
})
```

For fields not covered by the typed API, use `Raw()`:

```go
m.EditHPASpec(func(e *editors.HPASpecEditor) error {
    e.Raw().MinReplicas = ptr.To(int32(1))
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

## Data Extraction

`hpa.ExtractInto` declares that this HPA produces the value of a data cell, which is how the replica count the
autoscaler settled on reaches the resources that consume it. The function receives a value copy of the reconciled HPA
after each sync cycle:

```go
currentReplicas := concepts.NewData[int32]("backend-current-replicas")

builder := hpa.NewBuilder(base)
hpa.ExtractInto(builder, currentReplicas, func(h autoscalingv2.HorizontalPodAutoscaler) (int32, error) {
    return h.Status.CurrentReplicas, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(currentReplicas)` or read it
opportunistically with `WithOptionalData(currentReplicas)`. See [Declared Data](../component.md#declared-data).

## Operational Status

The default handler inspects `Status.Conditions`:

| Status             | Condition                                               |
| ------------------ | ------------------------------------------------------- |
| `Operational`      | `ScalingActive` is `True`                               |
| `OperationPending` | Conditions absent, or `ScalingActive` is `Unknown`      |
| `OperationFailing` | `ScalingActive` is `False`, or `AbleToScale` is `False` |

`AbleToScale = False` takes precedence over `ScalingActive = True` because an HPA that cannot scale is not healthy
regardless of what the scaling-active condition reports.

Override with `WithCustomOperationalStatus`:

```go
hpa.NewBuilder(base).
    WithCustomOperationalStatus(func(op concepts.ConvergingOperation, h *autoscalingv2.HorizontalPodAutoscaler) (concepts.OperationalStatusWithReason, error) {
        status, err := hpa.DefaultOperationalStatusHandler(op, h)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Grace Status

The default grace handler applies the same condition inspection after the grace period expires:

| Status     | Condition                                               |
| ---------- | ------------------------------------------------------- |
| `Healthy`  | `ScalingActive` is `True`                               |
| `Degraded` | Conditions absent, or `ScalingActive` is `Unknown`      |
| `Down`     | `ScalingActive` is `False`, or `AbleToScale` is `False` |

Override with `WithCustomGraceStatus`:

```go
hpa.NewBuilder(base).
    WithCustomGraceStatus(func(h *autoscalingv2.HorizontalPodAutoscaler) (concepts.GraceStatusWithReason, error) {
        status, err := hpa.DefaultGraceStatusHandler(h)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Suspension

HPA has no native suspend field. The default behavior is **delete on suspend**: the HPA is removed when the component
suspends and recreated on resume.

The reason this is necessary is the sequencing interaction with the HPA's scale target. When a `Deployment` (or other
workload) is suspended, the framework scales it to zero. A retained HPA would continuously enforce `minReplicas` and
scale the target back up, fighting the suspension. By deleting the HPA first, the target is free to scale down cleanly.
On resume the framework recreates the HPA before bringing the workload back.

The default suspension status handler reports `Suspended` immediately because the deletion is handled by the framework
and no additional convergence is required.

Override the deletion decision with `WithCustomSuspendDeletionDecision`:

```go
hpa.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *autoscalingv2.HorizontalPodAutoscaler) bool {
        return false // keep the HPA during suspension
    })
```

!!! note "When to keep the HPA"

    Retaining the HPA during suspension is only appropriate when the scale target is managed externally and will not
    be present during the component's suspension period. In the normal case where the HPA and its target are both
    managed by the same component, use the default delete behavior.

Override the suspension reason with `WithCustomSuspendStatus` if you need a message that reflects a non-default deletion
decision:

```go
hpa.NewBuilder(base).
    WithCustomSuspendStatus(func(_ *autoscalingv2.HorizontalPodAutoscaler) (concepts.SuspensionStatusWithReason, error) {
        return concepts.SuspensionStatusWithReason{
            Status: concepts.SuspensionStatusSuspended,
            Reason: "HPA retained; scale target managed externally",
        }, nil
    })
```

## Full Example

```go
func AutoscalingMutation(version string) hpa.Mutation {
    return hpa.Mutation{
        Name:    "autoscaling-config",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *hpa.Mutator) error {
            m.EditHPASpec(func(e *editors.HPASpecEditor) error {
                e.SetMinReplicas(ptr.To(int32(2)))
                e.SetMaxReplicas(10)

                // CPU-based scaling target
                e.EnsureMetric(autoscalingv2.MetricSpec{
                    Type: autoscalingv2.ResourceMetricSourceType,
                    Resource: &autoscalingv2.ResourceMetricSource{
                        Name: corev1.ResourceCPU,
                        Target: autoscalingv2.MetricTarget{
                            Type:               autoscalingv2.UtilizationMetricType,
                            AverageUtilization: ptr.To(int32(70)),
                        },
                    },
                })

                // Memory-based scaling target
                e.EnsureMetric(autoscalingv2.MetricSpec{
                    Type: autoscalingv2.ResourceMetricSourceType,
                    Resource: &autoscalingv2.ResourceMetricSource{
                        Name: corev1.ResourceMemory,
                        Target: autoscalingv2.MetricTarget{
                            Type:               autoscalingv2.UtilizationMetricType,
                            AverageUtilization: ptr.To(int32(80)),
                        },
                    },
                })

                // Conservative scale-down to avoid thrashing
                e.SetBehavior(&autoscalingv2.HorizontalPodAutoscalerBehavior{
                    ScaleDown: &autoscalingv2.HPAScalingRules{
                        StabilizationWindowSeconds: ptr.To(int32(300)),
                    },
                })

                return nil
            })

            m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureLabel("app.kubernetes.io/version", version)
                return nil
            })

            return nil
        },
    }
}

resource, err := hpa.NewBuilder(base).
    WithMutation(AutoscalingMutation(owner.Spec.Version)).
    Build()
```

Although `EditObjectMetadata` is called after `EditHPASpec` in source, metadata edits are applied first per the internal
ordering. Call order inside `Mutate` is for readability only; the framework enforces the correct execution sequence.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that always run. Use
`feature.NewVersionGate(version, constraints)` for version gating and chain `.When(bool)` for boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a metric or field set by mutation A, register A
first.

**Use `EnsureMetric` for idempotent metric management.** The editor matches by full metric identity so repeated calls
with the same identity update rather than duplicate.

**Delete on suspend is the correct default.** The HPA is removed during component suspension to prevent it from fighting
a scale-to-zero workload. Only override the deletion decision when the scale target is managed externally.

**Pair the suspension status handler with the deletion decision.** The default suspension reason is intentionally
deletion-agnostic. If you override `WithCustomSuspendDeletionDecision` to retain the HPA, also override
`WithCustomSuspendStatus` so the reason accurately describes what is happening.
