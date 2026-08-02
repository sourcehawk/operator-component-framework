# PersistentVolumeClaim Primitive

The `pvc` primitive wraps a Kubernetes `PersistentVolumeClaim` and integrates with the component lifecycle as an
Integration, Graceful, and Suspendable resource, providing a structured mutation API for managing storage requests and
object metadata.

## Capabilities

| Capability            | Detail                                                                                    |
| --------------------- | ----------------------------------------------------------------------------------------- |
| **Operational**       | Maps PVC phase to `Operational` (Bound), `OperationPending`, or `OperationFailing` (Lost) |
| **Graceful**          | Bound is `Healthy`; Lost is `Down`; any other phase is `Degraded`                         |
| **Suspendable**       | Immediately suspended (no runtime state to wind down); data is preserved by default       |
| **DataExtractable**   | Reads bound volume name, capacity, or other status fields after each sync cycle           |
| **Mutation pipeline** | Typed editors for PVC spec and object metadata, with a `Raw()` escape hatch               |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports.

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
    WithMutation(MyStorageMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example using the `SetStorageRequest` convenience method:

```go
func MyStorageMutation(version string) pvc.Mutation {
    return pvc.Mutation{
        Name:    "storage-expansion",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("20Gi"))
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category       | What it affects                                       |
| ---- | -------------- | ----------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `PersistentVolumeClaim` |
| 2    | Spec edits     | PVC spec: storage requests, access modes, etc.        |

Within each category, edits run in registration order. Later features observe the PVC as modified by all earlier ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

### PVCSpecEditor

The primary API for modifying PVC spec fields. Use `m.EditPVCSpec` for full control:

```go
m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
    e.SetStorageRequest(resource.MustParse("20Gi"))
    return nil
})
```

Available methods:

| Method                | What it does                                            |
| --------------------- | ------------------------------------------------------- |
| `SetStorageRequest`   | Sets `spec.resources.requests[storage]`                 |
| `SetAccessModes`      | Sets `spec.accessModes` (immutable after creation)      |
| `SetStorageClassName` | Sets `spec.storageClassName` (immutable after creation) |
| `SetVolumeMode`       | Sets `spec.volumeMode` (immutable after creation)       |
| `SetVolumeName`       | Sets `spec.volumeName` (immutable after creation)       |
| `Raw`                 | Returns `*corev1.PersistentVolumeClaimSpec`             |

#### Raw Escape Hatch

`Raw()` returns the underlying `*corev1.PersistentVolumeClaimSpec` for free-form editing:

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

| Method                        | Equivalent to                                   |
| ----------------------------- | ----------------------------------------------- |
| `SetStorageRequest(quantity)` | `EditPVCSpec` → `e.SetStorageRequest(quantity)` |

Use this for simple, single-operation mutations. Use `EditPVCSpec` when you need multiple operations or raw access in a
single edit block.

## Operational Status

The PVC primitive implements `concepts.Operational`. The default handler maps PVC phase to operational status:

| PVC Phase | Status             | Reason                          |
| --------- | ------------------ | ------------------------------- |
| `Bound`   | `Operational`      | PVC is bound to volume `<name>` |
| `Pending` | `OperationPending` | Waiting for PVC to be bound     |
| `Lost`    | `OperationFailing` | PVC has lost its bound volume   |

Override with `WithCustomOperationalStatus` for additional checks.

## Grace Status

The default grace status handler maps the PVC phase to a grace status after the grace period expires:

| PVC Phase | Status     | Meaning                       |
| --------- | ---------- | ----------------------------- |
| `Bound`   | `Healthy`  | PVC is bound to a volume      |
| `Lost`    | `Down`     | PVC has lost its bound volume |
| Other     | `Degraded` | PVC is not yet bound          |

Override with `WithCustomGraceStatus`:

```go
pvc.NewBuilder(base).
    WithCustomGraceStatus(func(p *corev1.PersistentVolumeClaim) (concepts.GraceStatusWithReason, error) {
        status, err := pvc.DefaultGraceStatusHandler(p)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Suspension

PVCs have no runtime state to wind down:

- `DefaultSuspendMutationHandler` is a no-op.
- `DefaultSuspensionStatusHandler` always reports `Suspended`.
- `DefaultDeleteOnSuspendHandler` returns `false` to preserve data.

Override these handlers if you need custom suspension behavior, such as adding annotations when suspended or deleting
PVCs that use ephemeral storage:

```go
resource, err := pvc.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *corev1.PersistentVolumeClaim) bool {
        return true // delete on suspend
    }).
    Build()
```

## Full Example

```go
func StorageRequestMutation(version string) pvc.Mutation {
    return pvc.Mutation{
        Name:    "storage-request",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("10Gi"))
            return nil
        },
    }
}

var v2Constraint = mustSemverConstraint(">= 2.0.0")

func ExpandedStorageMutation(version string) pvc.Mutation {
    return pvc.Mutation{
        Name: "expanded-storage",
        Feature: feature.NewVersionGate(
            version,
            []feature.VersionConstraint{v2Constraint},
        ),
        Mutate: func(m *pvc.Mutator) error {
            m.SetStorageRequest(resource.MustParse("50Gi"))
            return nil
        },
    }
}

boundVolume := concepts.NewData[string]("bound-volume")

builder := pvc.NewBuilder(base).
    WithMutation(StorageRequestMutation(owner.Spec.Version)).
    WithMutation(ExpandedStorageMutation(owner.Spec.Version))

pvc.ExtractInto(builder, boundVolume, func(p corev1.PersistentVolumeClaim) (string, error) {
    return p.Spec.VolumeName, nil
})

resource, err := builder.Build()
```

On versions 2.0.0 and above, `ExpandedStorageMutation` fires and sets the storage request to 50Gi. On earlier versions,
only the base 10Gi request is applied. After each reconcile cycle, the declared extraction captures the bound volume
name into the `bound-volume` cell.

## Guidance

**Register storage expansion mutations carefully.** Kubernetes allows expanding PVC storage but not shrinking it. Ensure
your mutations respect this constraint. The `SetStorageRequest` method does not enforce this; the API server rejects
invalid requests.

**Prefer `WithCustomSuspendDeletionDecision` over deleting PVCs manually.** If you need PVCs to be cleaned up during
suspension, register a deletion decision handler rather than deleting them in a mutation.

**Use `ExtractInto` to read bound volume information.** The bound volume name and actual allocated capacity are
server-assigned. Declare an extraction into a data cell rather than caching them in mutation logic.

**Use string status values in conditions.** The operational status values that appear in conditions are the runtime
strings `"Operational"`, `"OperationPending"`, and `"OperationFailing"`, not the Go constant identifiers.
