# StatefulSet Primitive

The `statefulset` primitive wraps a Kubernetes `StatefulSet` and provides health tracking, suspension, volume claim
template management, and a typed mutation API for managing replicas, pod spec, and containers as part of the component
lifecycle.

## Capabilities

| [Lifecycle interface](../primitives.md#lifecycle-interfaces) | Reported status values                                  |
| ------------------------------------------------------------ | ------------------------------------------------------- |
| `Alive`                                                      | `Healthy`, `Creating`, `Updating`, `Scaling`, `Failing` |
| `Graceful`                                                   | `Healthy`, `Degraded`, `Down`                           |
| `Suspendable`                                                | `PendingSuspension`, `Suspending`, `Suspended`          |
| `Guardable`                                                  | `Blocked`                                               |
| `DataExtractable`                                            | _(side-effecting, no status)_                           |

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
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Each mutation is a named `statefulset.Mutation` that receives a `*statefulset.Mutator` and records edits through typed
editors.

```go
func StorageMutation(version string) statefulset.Mutation {
    return statefulset.Mutation{
        Name:    "storage-backend",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *statefulset.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("db"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "PGDATA", Value: "/var/lib/postgresql/data"})
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

| Step | Category                         | What it affects                                                         |
| ---- | -------------------------------- | ----------------------------------------------------------------------- |
| 1    | Object metadata edits            | Labels and annotations on the `StatefulSet` object                      |
| 2    | StatefulSetSpec edits            | Replicas, service name, update strategy, etc.                           |
| 3    | Pod template metadata edits      | Labels and annotations on the pod template                              |
| 4    | Pod spec edits                   | Volumes, tolerations, node selectors, service account, security context |
| 5    | Regular container presence       | Adding or removing containers from `spec.template.spec.containers`      |
| 6    | Regular container edits          | Env vars, args, resources (snapshot taken after step 5)                 |
| 7    | Init container presence          | Adding or removing containers from `spec.template.spec.initContainers`  |
| 8    | Init container edits             | Env vars, args, resources (snapshot taken after step 7)                 |
| 9    | Volume claim template operations | Adding or removing entries from `spec.volumeClaimTemplates`             |

Container edits (steps 6 and 8) are evaluated against a snapshot taken _after_ presence operations in the same feature.

## Relevant Editors

For the generic editor and selector concepts, see [mutation editors](../primitives.md#mutation-editors) and
[container selectors](../primitives.md#container-selectors).

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

Use `Raw()` for fields the typed API does not cover:

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

Modifies individual containers via `m.EditContainers` or `m.EditInitContainers`, combined with a
[container selector](../primitives.md#container-selectors).

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

Modifies labels and annotations. Use `m.EditObjectMetadata` for the `StatefulSet` itself or `m.EditPodTemplateMetadata`
for the pod template.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    return nil
})
m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureAnnotation("prometheus.io/scrape", "true")
    return nil
})
```

## Convenience Methods

| Method                        | Equivalent to                                                 |
| ----------------------------- | ------------------------------------------------------------- |
| `EnsureReplicas(n)`           | `EditStatefulSetSpec` → `SetReplicas(n)`                      |
| `EnsureContainerEnvVar(ev)`   | `EditContainers(AllContainers(), ...)` → `EnsureEnvVar(ev)`   |
| `RemoveContainerEnvVar(name)` | `EditContainers(AllContainers(), ...)` → `RemoveEnvVar(name)` |
| `EnsureContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `EnsureArg(arg)`     |
| `RemoveContainerArg(arg)`     | `EditContainers(AllContainers(), ...)` → `RemoveArg(arg)`     |

## Workload-Kind-Agnostic Mutations

A mutation written against `primitives.WorkloadMutator` can be applied to a StatefulSet builder using
`statefulset.LiftMutation`. This lets one emitter function target StatefulSets, Deployments, and DaemonSets without
duplicating code.

```go
backend.WithMutation(statefulset.LiftMutation(sharedAuthMutation()))
```

See [workload-kind-agnostic mutations](../primitives.md#workload-kind-agnostic-mutations) for the full pattern.

## Volume Claim Templates

`EnsureVolumeClaimTemplate` and `RemoveVolumeClaimTemplate` manage persistent storage templates:

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

!!! warning "VolumeClaimTemplates are immutable after creation"

    `spec.volumeClaimTemplates` cannot be changed once the StatefulSet exists in the cluster; the API server rejects
    such updates. The mutator silently skips these operations on existing StatefulSets (identified by a non-empty
    `ResourceVersion`). Plan your storage layout before the first creation.

## Suspension

When the component is suspended, the StatefulSet is scaled to zero replicas. The resource is not deleted.

- `DefaultSuspendMutationHandler` calls `EnsureReplicas(0)`.
- `DefaultSuspensionStatusHandler` reports `Suspending` while `Status.Replicas > 0`, then `Suspended`.
- `DefaultDeleteOnSuspendHandler` returns `false`.

Override any handler via `WithCustomSuspendMutation`, `WithCustomSuspendStatus`, or `WithCustomSuspendDeletionDecision`
on the builder.

## Full Example

```go
func DatabaseMutation(version string) statefulset.Mutation {
    return statefulset.Mutation{
        Name:    "database-storage",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *statefulset.Mutator) error {
            m.EditStatefulSetSpec(func(e *editors.StatefulSetSpecEditor) error {
                e.SetReplicas(3)
                e.SetPodManagementPolicy(appsv1.OrderedReadyPodManagement)
                return nil
            })

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

**Use a StatefulSet for stateful workloads requiring pod identity.** StatefulSets provide stable network identities
(`pod-0`, `pod-1`, ...) and support VolumeClaimTemplates. For stateless workloads where pod identity does not matter, a
Deployment is simpler.

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for runtime boolean
conditions.

**Register mutations in dependency order.** If mutation B relies on a container added by mutation A, register A first.
Internal ordering within each mutation handles intra-mutation dependencies automatically.

**Prefer `EnsureContainer` over direct slice manipulation.** The mutator tracks presence operations so selectors in the
same mutation resolve correctly and reconciliation remains idempotent.

**VolumeClaimTemplates are immutable.** Plan your storage layout before the first creation. Changing the templates
requires recreating the StatefulSet.
