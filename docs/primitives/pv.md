# PersistentVolume Primitive

The `pv` primitive wraps a Kubernetes `PersistentVolume` and integrates with the component lifecycle as an Integration
and Graceful resource, providing a structured mutation API for managing PV spec fields and object metadata.

## Capabilities

| Capability            | Detail                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------ |
| **Operational**       | Maps PV phase to `Operational`, `OperationPending`, or `OperationFailing`                              |
| **Graceful**          | Available/Bound are `Healthy`; Pending is `Degraded`; Released/Failed are `Down`                       |
| **Cluster-scoped**    | No namespace in the identity or builder. PersistentVolumes are cluster-scoped resources                |
| **DataExtractable**   | Reads generated or updated values back from the reconciled PersistentVolume after each sync cycle      |
| **Mutation pipeline** | Typed editors for PV spec fields and object metadata, with a `Raw()` escape hatch for free-form access |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports. For cluster-scoped handling and owner-reference behavior, see
[Cluster-Scoped Primitives](../primitives.md#cluster-scoped-primitives).

## Building a PersistentVolume Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"

base := &corev1.PersistentVolume{
    ObjectMeta: metav1.ObjectMeta{
        Name: "data-volume",
    },
    Spec: corev1.PersistentVolumeSpec{
        Capacity: corev1.ResourceList{
            corev1.ResourceStorage: resource.MustParse("100Gi"),
        },
        AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
        PersistentVolumeSource: corev1.PersistentVolumeSource{
            CSI: &corev1.CSIPersistentVolumeSource{
                Driver:       "ebs.csi.aws.com",
                VolumeHandle: "vol-abc123",
            },
        },
    },
}

resource, err := pv.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

PersistentVolumes are cluster-scoped. The builder validates that `Name` is set and that `Namespace` is empty. Setting a
namespace on the PV object causes `Build()` to return an error.

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example using the `SetStorageClassName` convenience method:

```go
func RetainPolicyMutation(version string, retainEnabled bool) pv.Mutation {
    return pv.Mutation{
        Name:    "retain-policy",
        Feature: feature.NewVersionGate(version, nil).When(retainEnabled),
        Mutate: func(m *pv.Mutator) error {
            m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category       | What it affects                                                    |
| ---- | -------------- | ------------------------------------------------------------------ |
| 1    | Metadata edits | Labels and annotations on the `PersistentVolume`                   |
| 2    | Spec edits     | PV spec fields: storage class, reclaim policy, mount options, etc. |

Within each category, edits run in registration order. Later features observe the PersistentVolume as modified by all
earlier ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

### PVSpecEditor

The primary API for modifying PersistentVolume spec fields. Use `m.EditPVSpec` for full control:

```go
m.EditPVSpec(func(e *editors.PVSpecEditor) error {
    e.SetCapacity(resource.MustParse("200Gi"))
    e.SetAccessModes([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany})
    e.SetPersistentVolumeReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
    return nil
})
```

#### Available methods

| Method                                 | What it sets                           |
| -------------------------------------- | -------------------------------------- |
| `SetCapacity(resource.Quantity)`       | `.spec.capacity[storage]`              |
| `SetAccessModes([]AccessMode)`         | `.spec.accessModes`                    |
| `SetPersistentVolumeReclaimPolicy`     | `.spec.persistentVolumeReclaimPolicy`  |
| `SetStorageClassName(string)`          | `.spec.storageClassName`               |
| `SetMountOptions([]string)`            | `.spec.mountOptions`                   |
| `SetVolumeMode(PersistentVolumeMode)`  | `.spec.volumeMode`                     |
| `SetNodeAffinity(*VolumeNodeAffinity)` | `.spec.nodeAffinity`                   |
| `Raw()`                                | Returns `*corev1.PersistentVolumeSpec` |

#### Raw escape hatch

`Raw()` returns the underlying `*corev1.PersistentVolumeSpec` for free-form editing:

```go
m.EditPVSpec(func(e *editors.PVSpecEditor) error {
    e.Raw().PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("storage-tier", "premium")
    e.EnsureAnnotation("provisioned-by", "my-operator")
    return nil
})
```

## Convenience Methods

The `Mutator` exposes convenience wrappers for the most common PV spec operations:

| Method                      | Equivalent to                                          |
| --------------------------- | ------------------------------------------------------ |
| `SetStorageClassName(name)` | `EditPVSpec` → `e.SetStorageClassName(name)`           |
| `SetReclaimPolicy(policy)`  | `EditPVSpec` → `e.SetPersistentVolumeReclaimPolicy(p)` |
| `SetMountOptions(opts)`     | `EditPVSpec` → `e.SetMountOptions(opts)`               |

Use these for simple, single-operation mutations. Use `EditPVSpec` when you need multiple operations or raw access in a
single edit block.

## Data Extraction

`pv.ExtractInto` declares that this PersistentVolume produces the value of a data cell, such as the claim the volume was
bound to. The function receives a value copy of the reconciled PersistentVolume after each sync cycle:

```go
boundClaim := concepts.NewData[string]("data-volume-claim")

builder := pv.NewBuilder(base)
pv.ExtractInto(builder, boundClaim, func(v corev1.PersistentVolume) (string, error) {
    if v.Spec.ClaimRef == nil {
        return "", nil
    }
    return v.Spec.ClaimRef.Name, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(boundClaim)` or read it
opportunistically with `WithOptionalData(boundClaim)`. See [Declared Data](../component.md#declared-data).

## Operational Status

The PV primitive implements `concepts.Operational`. The default handler maps PV phase to operational status:

| PV Phase  | Status             | Meaning                                |
| --------- | ------------------ | -------------------------------------- |
| Available | `Operational`      | PV is ready for binding                |
| Bound     | `Operational`      | PV is bound to a PersistentVolumeClaim |
| Pending   | `OperationPending` | PV is waiting to become available      |
| Released  | `OperationFailing` | PV was released, not yet reclaimed     |
| Failed    | `OperationFailing` | PV reclamation has failed              |

Override with `WithCustomOperationalStatus` when your PV requires different readiness logic.

## Grace Status

The default grace status handler maps the PV phase to a grace status after the grace period expires:

| PV Phase  | Status     | Meaning                                |
| --------- | ---------- | -------------------------------------- |
| Available | `Healthy`  | PV is ready for binding                |
| Bound     | `Healthy`  | PV is bound to a PersistentVolumeClaim |
| Pending   | `Degraded` | PV is waiting to become available      |
| Released  | `Down`     | PV was released, not yet reclaimed     |
| Failed    | `Down`     | PV reclamation has failed              |

Override with `WithCustomGraceStatus`:

```go
pv.NewBuilder(base).
    WithCustomGraceStatus(func(p *corev1.PersistentVolume) (concepts.GraceStatusWithReason, error) {
        status, err := pv.DefaultGraceStatusHandler(p)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Full Example

```go
func StorageClassMutation(version string) pv.Mutation {
    return pv.Mutation{
        Name:    "storage-class",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pv.Mutator) error {
            m.SetStorageClassName("fast-ssd")
            m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
            return nil
        },
    }
}

func TierLabelMutation(version, tier string) pv.Mutation {
    return pv.Mutation{
        Name:    "tier-label",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pv.Mutator) error {
            m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureLabel("storage-tier", tier)
                return nil
            })
            return nil
        },
    }
}

resource, err := pv.NewBuilder(base).
    WithMutation(StorageClassMutation(owner.Spec.Version)).
    WithMutation(TierLabelMutation(owner.Spec.Version, "premium")).
    Build()
```

## Guidance

**PersistentVolumes are cluster-scoped.** Do not set a namespace on the PV object. The builder rejects namespaced PVs
with a clear error.

**Understand the garbage collection constraint.** The component reconciliation pipeline attempts to set a controller
reference on created/updated resources. Because `PersistentVolume` is cluster-scoped, its controller owner must also be
cluster-scoped. When the owner is namespace-scoped, the framework detects the mismatch and skips setting
`ownerReferences` instead of letting the API server reject the request. Such PVs will not be garbage collected
automatically when the owning component is deleted. Either model the PV under a dedicated cluster-scoped component to
allow a valid controller reference, or accept that PVs managed from a namespace-scoped component require explicit
lifecycle handling.

**Use string status values in conditions.** The operational status values that appear in conditions are the runtime
strings `"Operational"`, `"OperationPending"`, and `"OperationFailing"`, not the Go constant identifiers.

**Register mutations in dependency order.** If mutation B relies on a field set by mutation A, register A first.
