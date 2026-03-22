# PersistentVolumeClaim Primitive

The `pvc` primitive is the framework's built-in integration abstraction for managing Kubernetes `PersistentVolumeClaim` resources. It integrates with the component lifecycle and provides a structured mutation API for managing storage requests and object metadata.

## Capabilities

| Capability                | Detail                                                                                         |
|---------------------------|------------------------------------------------------------------------------------------------|
| **Operational tracking**  | Monitors PVC phase — reports `Operational` (Bound), `Pending`, or `Failing` (Lost)             |
| **Immutable field safety**| DefaultFieldApplicator preserves `accessModes`, `storageClassName`, `volumeMode`, `volumeName` |
| **Suspension**            | PVCs are immediately suspended (no runtime state to wind down); data is preserved by default   |
| **Mutation pipeline**     | Typed editors for PVC spec and object metadata, with a raw escape hatch for free-form access   |
| **Flavors**               | Preserves externally-managed fields — labels and annotations not owned by the operator         |
| **Data extraction**       | Reads bound volume name, capacity, or other status fields after each sync cycle                |

## Building a PVC Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pvc"

base := &corev1.PersistentVolumeClaim{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-data",
        Namespace: owner.Namespace,
    },
    Spec: corev1.PersistentVolumeClaimSpec{
        AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
        Resources: corev1.VolumeResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceStorage: resource.MustParse("10Gi"),
            },
        },
    },
}

resource, err := pvc.NewBuilder(base).
    WithFieldApplicationFlavor(pvc.PreserveCurrentLabels).
    WithMutation(MyStorageMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current PVC with a deep copy of the desired object, but preserves immutable fields on existing PVCs.

Kubernetes marks several PVC spec fields as immutable after creation. The default applicator detects existing PVCs (via `ResourceVersion`) and restores these fields from the cluster object:

| Immutable Field      | Preserved On Existing |
|----------------------|-----------------------|
| `accessModes`        | Yes                   |
| `storageClassName`   | Yes                   |
| `volumeMode`         | Yes                   |
| `volumeName`         | Yes                   |

Use `WithCustomFieldApplicator` when you need different merge semantics:

```go
resource, err := pvc.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *corev1.PersistentVolumeClaim) error {
        // Only update the storage request; leave everything else untouched.
        current.Spec.Resources.Requests = desired.Spec.Resources.Requests
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `PersistentVolumeClaim` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature with no version constraints and no `When()` conditions is also always enabled:

```go
func MyStorageMutation(version string) pvc.Mutation {
    return pvc.Mutation{
        Name:    "storage-expansion",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("20Gi"))
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

```go
func LargeStorageMutation(version string, needsLargeStorage bool) pvc.Mutation {
    return pvc.Mutation{
        Name:    "large-storage",
        Feature: feature.NewResourceFeature(version, nil).When(needsLargeStorage),
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("100Gi"))
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var v2Constraint = mustSemverConstraint(">= 2.0.0")

func V2StorageMutation(version string) pvc.Mutation {
    return pvc.Mutation{
        Name: "v2-storage",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{v2Constraint},
        ),
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("50Gi"))
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are recorded:

| Step | Category          | What it affects                                    |
|------|-------------------|----------------------------------------------------|
| 1    | Metadata edits    | Labels and annotations on the `PersistentVolumeClaim` |
| 2    | Spec edits        | PVC spec — storage requests, access modes, etc.    |

Within each category, edits are applied in their registration order. Later features observe the PVC as modified by all previous features.

## Editors

### PVCSpecEditor

The primary API for modifying PVC spec fields. Use `m.EditPVCSpec` for full control:

```go
m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
    e.SetStorageRequest(resource.MustParse("20Gi"))
    return nil
})
```

Available methods:

| Method                | What it does                                      |
|-----------------------|---------------------------------------------------|
| `SetStorageRequest`   | Sets `spec.resources.requests[storage]`            |
| `SetAccessModes`      | Sets `spec.accessModes` (immutable after creation) |
| `SetStorageClassName` | Sets `spec.storageClassName` (immutable after creation) |
| `SetVolumeMode`       | Sets `spec.volumeMode` (immutable after creation)  |
| `SetVolumeName`       | Sets `spec.volumeName` (immutable after creation)  |
| `Raw`                 | Returns `*corev1.PersistentVolumeClaimSpec`        |

#### Raw Escape Hatch

`Raw()` returns the underlying `*corev1.PersistentVolumeClaimSpec` for free-form editing when none of the structured methods are sufficient:

```go
m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
    raw := e.Raw()
    raw.Selector = &metav1.LabelSelector{
        MatchLabels: map[string]string{"type": "fast"},
    }
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("storage/class-hint", "fast-ssd")
    return nil
})
```

## Convenience Methods

The `Mutator` exposes a convenience wrapper for the most common PVC operation:

| Method                         | Equivalent to                                 |
|--------------------------------|-----------------------------------------------|
| `SetStorageRequest(quantity)`  | `EditPVCSpec` → `e.SetStorageRequest(quantity)` |

Use this for simple, single-operation mutations. Use `EditPVCSpec` when you need multiple operations or raw access in a single edit block.

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := pvc.NewBuilder(base).
    WithFieldApplicationFlavor(pvc.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on overlap.

```go
resource, err := pvc.NewBuilder(base).
    WithFieldApplicationFlavor(pvc.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Status Handlers

### Operational Status

The default handler (`DefaultOperationalStatusHandler`) maps PVC phase to operational status:

| PVC Phase  | Status        | Reason                           |
|------------|---------------|----------------------------------|
| `Bound`    | Operational   | PVC is bound to volume \<name\>  |
| `Pending`  | Pending       | Waiting for PVC to be bound      |
| `Lost`     | Failing       | PVC has lost its bound volume    |

Override with `WithCustomOperationalStatus` for additional checks (e.g. verifying specific annotations or volume attributes).

### Suspension

PVCs have no runtime state to wind down, so:

- `DefaultSuspendMutationHandler` is a no-op.
- `DefaultSuspensionStatusHandler` always reports `Suspended`.
- `DefaultDeleteOnSuspendHandler` returns `false` to preserve data.

Override these handlers if you need custom suspension behavior, such as adding annotations when suspended or deleting PVCs that use ephemeral storage.

## Guidance

**Use `DefaultFieldApplicator` unless you have a specific reason not to.** It handles the immutable field preservation that Kubernetes requires for PVCs, preventing rejected update requests.

**Register mutations for storage expansion carefully.** Kubernetes only allows expanding PVC storage (not shrinking). Ensure your mutations respect this constraint. The `SetStorageRequest` method does not enforce this — the API server will reject invalid requests.

**Prefer `WithCustomSuspendDeletionDecision` over deleting PVCs manually.** If you need PVCs to be cleaned up during suspension, register a deletion decision handler rather than deleting them in a mutation.

**Use flavors for externally-managed metadata.** If admission webhooks, external controllers, or GitOps tools add labels or annotations to your PVCs, use `PreserveCurrentLabels` and `PreserveCurrentAnnotations` to prevent your operator from removing them.
