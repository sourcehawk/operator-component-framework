# HorizontalPodAutoscaler (HPA) Primitive

The `hpa` primitive is the framework's built-in integration abstraction for managing Kubernetes
`HorizontalPodAutoscaler` resources (`autoscaling/v2`). It integrates with the component lifecycle as an Operational,
Suspendable resource and provides a structured mutation API for configuring autoscaling behavior.

## Capabilities

| Capability              | Detail                                                                                                      |
| ----------------------- | ----------------------------------------------------------------------------------------------------------- |
| **Operational status**  | Inspects `ScalingActive` and `AbleToScale` conditions to report `Operational`, `Pending`, or `Failing`      |
| **Suspension (delete)** | Deletes the HPA on suspend to prevent it from scaling the target back up; recreated on resume               |
| **Mutation pipeline**   | Typed editors for HPA spec (metrics, scale target, behavior) and object metadata                            |
| **Flavors**             | Preserves externally-managed labels and annotations                                                         |
| **Data extraction**     | Optionally exposes current and desired replica counts via a registered data extractor (`WithDataExtractor`) |

## Building an HPA Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/hpa"

base := &autoscalingv2.HorizontalPodAutoscaler{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-hpa",
        Namespace: owner.Namespace,
    },
    Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
        ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
            APIVersion: "apps/v1",
            Kind:       "Deployment",
            Name:       "web",
        },
        MinReplicas: ptr.To(int32(2)),
        MaxReplicas: 10,
    },
}

resource, err := hpa.NewBuilder(base).
    WithFieldApplicationFlavor(hpa.PreserveCurrentLabels).
    WithMutation(CPUMetricMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current HPA with a deep copy of the desired object, then restores server-managed
metadata (ResourceVersion, UID, etc.), shared-controller fields (OwnerReferences, Finalizers), and the Status
subresource from the original live object. This prevents spec-level reconciliation from clearing status data written by
the API server or other controllers.

Use `WithCustomFieldApplicator` when other controllers manage spec-level fields that should not be overwritten:

```go
resource, err := hpa.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *autoscalingv2.HorizontalPodAutoscaler) error {
        // Preserve the current status and server-managed/shared-controller metadata
        savedStatus := current.Status
        savedMeta := current.ObjectMeta

        // Update spec and other desired fields
        desired.DeepCopyInto(current)

        // Restore server-managed and shared-controller metadata from the live object
        current.ObjectMeta.ResourceVersion = savedMeta.ResourceVersion
        current.ObjectMeta.UID = savedMeta.UID
        current.ObjectMeta.Generation = savedMeta.Generation
        current.ObjectMeta.OwnerReferences = savedMeta.OwnerReferences
        current.ObjectMeta.Finalizers = savedMeta.Finalizers

        // Restore status from the live object
        current.Status = savedStatus
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying an HPA beyond its baseline. Each mutation is a named function that
receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func CPUMetricMutation(version string) hpa.Mutation {
    return hpa.Mutation{
        Name:    "cpu-metric",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *hpa.Mutator) error {
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
func CustomMetricsMutation(version string, enabled bool) hpa.Mutation {
    return hpa.Mutation{
        Name:    "custom-metrics",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *hpa.Mutator) error {
            m.EditHPASpec(func(e *editors.HPASpecEditor) error {
                e.EnsureMetric(autoscalingv2.MetricSpec{
                    Type: autoscalingv2.PodsMetricSourceType,
                    Pods: &autoscalingv2.PodsMetricSource{
                        Metric: autoscalingv2.MetricIdentifier{Name: "requests_per_second"},
                        Target: autoscalingv2.MetricTarget{
                            Type:         autoscalingv2.AverageValueMetricType,
                            AverageValue: ptr.To(resource.MustParse("100")),
                        },
                    },
                })
                return nil
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

func LegacyScalingMutation(version string) hpa.Mutation {
    return hpa.Mutation{
        Name: "legacy-scaling",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *hpa.Mutator) error {
            m.EditHPASpec(func(e *editors.HPASpecEditor) error {
                e.SetMaxReplicas(5) // legacy apps limited to 5 replicas
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
order they are recorded:

| Step | Category       | What it affects                                                |
| ---- | -------------- | -------------------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `HorizontalPodAutoscaler` object |
| 2    | HPA spec edits | Scale target ref, min/max replicas, metrics, behavior          |

## Editors

### HPASpecEditor

Controls HPA-level settings via `m.EditHPASpec`.

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

#### EnsureMetric

`EnsureMetric` upserts a metric based on its full metric identity, not just type and name. Matching rules:

| Metric type       | Match key                                                                                                 |
| ----------------- | --------------------------------------------------------------------------------------------------------- |
| Resource          | `Resource.Name` (e.g. `cpu`, `memory`)                                                                    |
| Pods              | `Pods.Metric.Name` + `Pods.Metric.Selector` (label selector; `nil` is a distinct identity)                |
| Object            | `Object.DescribedObject` (`APIVersion`, `Kind`, `Name`) + `Object.Metric.Name` + `Object.Metric.Selector` |
| ContainerResource | `ContainerResource.Name` + `ContainerResource.Container`                                                  |
| External          | `External.Metric.Name` + `External.Metric.Selector` (label selector; `nil` is a distinct identity)        |

If a matching entry exists it is replaced; otherwise the metric is appended. Be aware that different selectors or
described objects result in different metric identities, even if the metric names are the same.

#### RemoveMetric

`RemoveMetric(type, name)` removes all metrics matching the given type and name. For ContainerResource metrics, all
container variants of the named resource are removed.

#### SetBehavior

`SetBehavior` sets the autoscaling behavior (stabilization windows, scaling policies). Pass `nil` to remove custom
behavior and use Kubernetes defaults.

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
    e.EnsureLabel("app.kubernetes.io/managed-by", "my-operator")
    e.EnsureAnnotation("autoscaling.example.io/policy", "aggressive")
    return nil
})
```

### Raw Escape Hatch

All editors provide a `.Raw()` method for direct access to the underlying Kubernetes struct when the typed API is
insufficient.

## Operational Status

The default operational status handler inspects `Status.Conditions`:

| Status        | Condition                                               |
| ------------- | ------------------------------------------------------- |
| `Operational` | `ScalingActive` is `True`                               |
| `Pending`     | Conditions absent, or `ScalingActive` is `Unknown`      |
| `Failing`     | `ScalingActive` is `False`, or `AbleToScale` is `False` |

`AbleToScale = False` takes precedence over `ScalingActive = True` because an HPA that cannot actually scale is not
operationally healthy regardless of what the scaling-active condition reports.

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

## Suspension

HPA has no native suspend field. The default behavior is **delete on suspend**: the HPA is removed when the component is
suspended (`DefaultDeleteOnSuspendHandler` returns `true`). A retained HPA would conflict with the suspension of its
scale target (e.g. a Deployment scaled to zero) because the Kubernetes HPA controller continuously enforces
`minReplicas` and would scale the target back up. Deleting the HPA prevents this interference. On resume the framework
recreates the HPA with the desired spec.

The default suspension status handler reports `Suspended` immediately with the reason
`"HorizontalPodAutoscaler deleted to prevent scaling interference during suspension"`.

Override with `WithCustomSuspendDeletionDecision` if you want to retain the HPA during suspension (e.g. when the scale
target is managed externally and will not be present during suspension):

```go
hpa.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *autoscalingv2.HorizontalPodAutoscaler) bool {
        return false // keep HPA during suspension
    })
```

## Flavors

| Flavor                       | Effect                                                                         |
| ---------------------------- | ------------------------------------------------------------------------------ |
| `PreserveCurrentLabels`      | Keeps labels from the live object that the desired state does not declare      |
| `PreserveCurrentAnnotations` | Keeps annotations from the live object that the desired state does not declare |

```go
hpa.NewBuilder(base).
    WithFieldApplicationFlavor(hpa.PreserveCurrentLabels).
    WithFieldApplicationFlavor(hpa.PreserveCurrentAnnotations)
```

## Full Example: CPU and Memory Autoscaling

```go
func AutoscalingMutation(version string) hpa.Mutation {
    return hpa.Mutation{
        Name:    "autoscaling-config",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *hpa.Mutator) error {
            m.EditHPASpec(func(e *editors.HPASpecEditor) error {
                e.SetMinReplicas(ptr.To(int32(2)))
                e.SetMaxReplicas(10)

                // CPU-based scaling
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

                // Memory-based scaling
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

                // Conservative scale-down
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
```

Note: although `EditObjectMetadata` is called after `EditHPASpec` in the source, metadata edits are applied first per
the internal ordering. Order your source calls for readability — the framework handles execution order.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a metric added by mutation A, register A first.

**Use `EnsureMetric` for idempotent metric management.** The editor matches by full metric identity (type, name,
selector, and described object where applicable), so repeated calls with the same identity update rather than duplicate.

**HPA deletion on suspend is the default.** The primitive's default `DeleteOnSuspend` decision removes the HPA during
component suspension (matching the "Suspension (delete)" capability). This prevents the Kubernetes HPA controller from
scaling the target back up while it is suspended. On resume the framework recreates the HPA with the desired spec. If
you need the HPA to be retained during suspension — for example, when the scale target is managed externally and will
not be present — override `WithCustomSuspendDeletionDecision` to return `false`.
