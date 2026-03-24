# StatefulSet Primitive

The `statefulset` primitive is the framework's built-in workload abstraction for managing Kubernetes `StatefulSet`
resources. It integrates fully with the component lifecycle and provides a rich mutation API for managing containers,
pod specs, metadata, and volume claim templates.

## Capabilities

| Capability            | Detail                                                                                                                                                                              |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Health tracking**   | Verifies `ObservedGeneration` matches `Generation` before evaluating `ReadyReplicas`; reports `Healthy`, `Creating`, `Updating`, or `Scaling`; grace handler can mark Down/Degraded |
| **Rollout health**    | Surfaces stalled or failing rollouts by transitioning the resource to `Degraded` or `Down` (no grace-period timing)                                                                 |
| **Suspension**        | Scales to zero replicas; reports `Suspending` / `Suspended`                                                                                                                         |
| **Mutation pipeline** | Typed editors for metadata, statefulset spec, pod spec, containers, and volume claim templates                                                                                      |
| **Flavors**           | Preserves externally-managed fields (labels, annotations, pod template metadata)                                                                                                    |

## Building a StatefulSet Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"

base := &appsv1.StatefulSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "db",
        Namespace: owner.Namespace,
    },
    Spec: appsv1.StatefulSetSpec{
        ServiceName: "db-headless",
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "db"},
        },
        Template: corev1.PodTemplateSpec{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"app": "db"},
            },
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{
                    {Name: "db", Image: "postgres:15"},
                },
            },
        },
    },
}

resource, err := statefulset.NewBuilder(base).
    WithFieldApplicationFlavor(statefulset.PreserveCurrentLabels).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current StatefulSet with a deep copy of the desired object, then restores
server-managed metadata (ResourceVersion, UID, etc.), shared-controller fields (OwnerReferences, Finalizers), and the
Status subresource from the original live object. This prevents spec-level reconciliation from clearing status data
written by the API server or other controllers.

It also preserves `spec.volumeClaimTemplates` from the live object when the resource already exists (i.e., has a
non-empty `ResourceVersion`). This is necessary because `spec.volumeClaimTemplates` is immutable after creation in
Kubernetes — the API server rejects any update that changes it.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := statefulset.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *appsv1.StatefulSet) error {
        // Custom merge logic
        current.Spec.Template = *desired.Spec.Template.DeepCopy()
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `StatefulSet` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) statefulset.Mutation {
    return statefulset.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *statefulset.Mutator) error {
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
func TracingMutation(version string, enabled bool) statefulset.Mutation {
    return statefulset.Mutation{
        Name:    "tracing",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *statefulset.Mutator) error {
            m.EnsureInitContainer(corev1.Container{
                Name:  "init-config",
                Image: "config-init:latest",
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

func LegacyStorageMutation(version string) statefulset.Mutation {
    return statefulset.Mutation{
        Name: "legacy-storage",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *statefulset.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("db"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "STORAGE_BACKEND", Value: "legacy"})
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

| Step | Category                         | What it affects                                                         |
| ---- | -------------------------------- | ----------------------------------------------------------------------- |
| 1    | StatefulSet metadata edits       | Labels and annotations on the `StatefulSet` object                      |
| 2    | StatefulSetSpec edits            | Replicas, service name, update strategy, etc.                           |
| 3    | Pod template metadata edits      | Labels and annotations on the pod template                              |
| 4    | Pod spec edits                   | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence       | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits          | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence          | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits             | Env vars, args, resources (snapshot taken after step 7)                 |
| 9    | Volume claim template operations | Adding or removing entries from `spec.volumeClaimTemplates`             |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same mutation.
This means a single mutation can add a container and then configure it without selector resolution issues.

## Editors

### StatefulSetSpecEditor

Controls statefulset-level settings via `m.EditStatefulSetSpec`.

Available methods: `SetReplicas`, `SetServiceName`, `SetPodManagementPolicy`, `SetUpdateStrategy`,
`SetRevisionHistoryLimit`, `SetMinReadySeconds`, `SetPersistentVolumeClaimRetentionPolicy`, `Raw`.

```go
m.EditStatefulSetSpec(func(e *editors.StatefulSetSpecEditor) error {
    e.SetReplicas(3)
    e.SetServiceName("db-headless")
    e.SetPodManagementPolicy(appsv1.ParallelPodManagement)
    return nil
})
```

For fields not covered by the typed API, use `Raw()`:

```go
m.EditStatefulSetSpec(func(e *editors.StatefulSetSpecEditor) error {
    e.Raw().UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
        Type: appsv1.OnDeleteStatefulSetStrategyType,
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
    e.SetServiceAccountName("db-sa")
    e.EnsureVolume(corev1.Volume{
        Name: "config",
        VolumeSource: corev1.VolumeSource{
            ConfigMap: &corev1.ConfigMapVolumeSource{
                LocalObjectReference: corev1.LocalObjectReference{Name: "db-config"},
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
m.EditContainers(selectors.ContainerNamed("db"), func(e *editors.ContainerEditor) error {
    e.EnsureEnvVar(corev1.EnvVar{Name: "PGDATA", Value: "/var/lib/postgresql/data"})
    e.SetResourceLimit(corev1.ResourceMemory, resource.MustParse("2Gi"))
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations. Use `m.EditObjectMetadata` to target the `StatefulSet` object itself, or
`m.EditPodTemplateMetadata` to target the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
// On the StatefulSet itself
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

## Volume Claim Templates

The mutator provides `EnsureVolumeClaimTemplate` and `RemoveVolumeClaimTemplate` for managing persistent storage:

```go
m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
    ObjectMeta: metav1.ObjectMeta{Name: "data"},
    Spec: corev1.PersistentVolumeClaimSpec{
        AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
        Resources: corev1.VolumeResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceStorage: resource.MustParse("10Gi"),
            },
        },
    },
})
```

**Important:** `spec.volumeClaimTemplates` is immutable after creation in Kubernetes. The `DefaultFieldApplicator`
preserves the live VolumeClaimTemplates to avoid API server rejections. These mutation methods are primarily useful for
constructing the initial desired state or when recreating a StatefulSet.

## Convenience Methods

The `Mutator` also exposes convenience wrappers:

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureReplicas(n)`           | `EditStatefulSetSpec` → `SetReplicas(n)`                      |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external
controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on
overlap.

### PreserveCurrentPodTemplateLabels

Preserves labels present on the live object's pod template but absent from the applied desired state's pod template.
Applied labels win on overlap.

### PreserveCurrentPodTemplateAnnotations

Preserves annotations present on the live object's pod template but absent from the applied desired state's pod
template. Applied annotations win on overlap.

```go
resource, err := statefulset.NewBuilder(base).
    WithFieldApplicationFlavor(statefulset.PreserveCurrentLabels).
    WithFieldApplicationFlavor(statefulset.PreserveCurrentAnnotations).
    WithFieldApplicationFlavor(statefulset.PreserveCurrentPodTemplateLabels).
    WithFieldApplicationFlavor(statefulset.PreserveCurrentPodTemplateAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Full Example: Database StatefulSet with Storage

```go
func DatabaseMutation(version string) statefulset.Mutation {
    return statefulset.Mutation{
        Name:    "database-storage",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *statefulset.Mutator) error {
            // Configure the StatefulSet spec
            m.EditStatefulSetSpec(func(e *editors.StatefulSetSpecEditor) error {
                e.SetReplicas(3)
                e.SetPodManagementPolicy(appsv1.OrderedReadyPodManagement)
                return nil
            })

            // Add a volume claim template for persistent data
            m.EnsureVolumeClaimTemplate(corev1.PersistentVolumeClaim{
                ObjectMeta: metav1.ObjectMeta{Name: "data"},
                Spec: corev1.PersistentVolumeClaimSpec{
                    AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
                    Resources: corev1.VolumeResourceRequirements{
                        Requests: corev1.ResourceList{
                            corev1.ResourceStorage: resource.MustParse("50Gi"),
                        },
                    },
                },
            })

            // Mount the volume in the database container
            m.EditContainers(selectors.ContainerNamed("db"), func(e *editors.ContainerEditor) error {
                e.Raw().VolumeMounts = append(e.Raw().VolumeMounts, corev1.VolumeMount{
                    Name:      "data",
                    MountPath: "/var/lib/postgresql/data",
                })
                return nil
            })

            return nil
        },
    }
}
```

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
The internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so that selectors in
the same mutation resolve correctly and reconciliation remains idempotent.

**VolumeClaimTemplates are immutable.** Plan your storage layout before the first creation. The `DefaultFieldApplicator`
preserves live VolumeClaimTemplates to prevent API server rejections on updates.

**Use selectors for precision.** Targeting `AllContainers()` when you only mean to modify the primary container can
cause unexpected behavior if sidecar containers are present.
