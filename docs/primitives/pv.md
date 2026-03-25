# PersistentVolume Primitive

The `pv` primitive is the framework's built-in integration abstraction for managing Kubernetes `PersistentVolume`
resources. It integrates with the component lifecycle and provides a structured mutation API for managing PV spec fields
and object metadata.

## Capabilities

| Capability                | Detail                                                                                             |
| ------------------------- | -------------------------------------------------------------------------------------------------- |
| **Integration lifecycle** | Reports `concepts.OperationalStatusOperational`, `concepts.OperationalStatusPending`, or `concepts.OperationalStatusFailing` based on the PV's phase |
| **Cluster-scoped**        | No namespace in the identity or builder — PersistentVolumes are cluster-scoped resources           |
| **Mutation pipeline**     | Typed editors for PV spec fields and object metadata, with a raw escape hatch for free-form access |
| **Data extraction**       | Reads generated or updated values back from the reconciled PersistentVolume after each sync cycle  |

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

PersistentVolumes are cluster-scoped. The builder validates that Name is set and that Namespace is empty. Setting a
namespace on the PV object will cause `Build()` to return an error.

## Mutations

Mutations are the primary mechanism for modifying a `PersistentVolume` beyond its baseline. Each mutation is a named
function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) pv.Mutation {
    return pv.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *pv.Mutator) error {
            m.SetStorageClassName("fast-ssd")
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

```go
func RetainPolicyMutation(version string, retainEnabled bool) pv.Mutation {
    return pv.Mutation{
        Name:    "retain-policy",
        Feature: feature.NewResourceFeature(version, nil).When(retainEnabled),
        Mutate: func(m *pv.Mutator) error {
            m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyStorageClassMutation(version string) pv.Mutation {
    return pv.Mutation{
        Name: "legacy-storage-class",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *pv.Mutator) error {
            m.SetStorageClassName("legacy-hdd")
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are
recorded:

| Step | Category       | What it affects                                                     |
| ---- | -------------- | ------------------------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `PersistentVolume`                    |
| 2    | Spec edits     | PV spec fields — storage class, reclaim policy, mount options, etc. |

Within each category, edits are applied in their registration order. Later features observe the PersistentVolume as
modified by all previous features.

## Editors

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

`Raw()` returns the underlying `*corev1.PersistentVolumeSpec` for free-form editing when none of the structured methods
are sufficient:

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

## Operational Status

The PV primitive uses the Integration lifecycle. The default operational status handler maps PV phases to framework
status:

| PV Phase  | Operational Status | Meaning                                |
| --------- | ------------------ | -------------------------------------- |
| Available | OperationalStatusOperational | PV is ready for binding                |
| Bound     | OperationalStatusOperational | PV is bound to a PersistentVolumeClaim |
| Pending   | OperationalStatusPending   | PV is waiting to become available      |
| Released  | OperationalStatusFailing   | PV was released, not yet reclaimed     |
| Failed    | OperationalStatusFailing   | PV reclamation has failed              |

Override with `WithCustomOperationalStatus` when your PV requires different readiness logic.

## Full Example: Storage-Tier PersistentVolume

```go
func StorageClassMutation(version string) pv.Mutation {
    return pv.Mutation{
        Name:    "storage-class",
        Feature: feature.NewResourceFeature(version, nil),
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
        Feature: feature.NewResourceFeature(version, nil),
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

**Use the Integration lifecycle for status.** PVs report `OperationalStatusOperational`, `OperationalStatusPending`, or `OperationalStatusFailing` based
on their phase. Override with `WithCustomOperationalStatus` only when phase-based readiness is insufficient.

**Controller references and garbage collection.** The component reconciliation pipeline attempts to set a controller
reference on created/updated resources. Because `PersistentVolume` is cluster-scoped, its controller owner must also be
cluster-scoped. When the owner is namespace-scoped and the PV is cluster-scoped, the framework detects this mismatch and
**skips setting `ownerReferences`** (logging an informational message) instead of letting the API server reject the
request. As a result, such PVs will **not** be garbage collected automatically when the owning component is deleted. If
you need garbage collection for PVs, either:

- Model the PV as owned by a dedicated **cluster-scoped** controller/component so a valid controller reference can be
  set, or
- Accept that PVs managed from a **namespace-scoped** component will not have `ownerReferences` and handle their
  lifecycle explicitly (for example, by deleting them in custom logic when appropriate).

**Register mutations in dependency order.** If mutation B relies on a field set by mutation A, register A first.
