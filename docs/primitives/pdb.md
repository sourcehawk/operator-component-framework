# PodDisruptionBudget Primitive

The `pdb` primitive wraps `policy/v1 PodDisruptionBudget` and reconciles it to desired state without health tracking or
suspension.

!!! note "PDB is Static"

    Despite sitting in the "Scaling & Availability" nav group alongside HPA, `PodDisruptionBudget` is a
    [Static](../primitives.md#static) primitive. It has no convergence loop, no operational status, and no
    suspension behavior. The resource is applied and considered ready once it exists. Readers expecting an
    `OperationPending` condition will not find one.

## Capabilities

The interfaces below are from [`pkg/component/concepts`](../primitives.md#lifecycle-interfaces). The values in the table
are the runtime strings that appear in conditions.

| Interface         | Reported status values        | Notes                                       |
| ----------------- | ----------------------------- | ------------------------------------------- |
| `Guardable`       | `Blocked`                     | Optional runtime precondition               |
| `DataExtractable` | _(side-effecting, no status)_ | Read generated fields after each sync cycle |

## Building a PDB Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pdb"

minAvailable := intstr.FromString("50%")
base := &policyv1.PodDisruptionBudget{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "backend-pdb",
        Namespace: owner.Namespace,
    },
    Spec: policyv1.PodDisruptionBudgetSpec{
        MinAvailable: &minAvailable,
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "backend"},
        },
    },
}

resource, err := pdb.NewBuilder(base).
    WithMutation(DisruptionPolicyMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are named functions that receive a `*pdb.Mutator` and record edit intent through typed editors. For a full
explanation of the mutation system, boolean-gated mutations, and version-gated mutations see
[The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A concise boolean-gated example that switches from percentage-based `MinAvailable` to absolute `MaxUnavailable`:

```go
func StrictAvailabilityMutation(version string, strict bool) pdb.Mutation {
    return pdb.Mutation{
        Name:    "strict-availability",
        Feature: feature.NewVersionGate(version, nil).When(strict),
        Mutate: func(m *pdb.Mutator) error {
            m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
                e.ClearMinAvailable()
                e.SetMaxUnavailable(intstr.FromInt32(1))
                return nil
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits execute in a fixed category order regardless of the order they are recorded:

| Step | Category       | What it affects                                         |
| ---- | -------------- | ------------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `PodDisruptionBudget`     |
| 2    | Spec edits     | MinAvailable, MaxUnavailable, selector, eviction policy |

Features apply in registration order. Later features observe the PDB as modified by all earlier ones.

## Relevant Editors

For the full method list of any editor see the
[Go API reference](https://pkg.go.dev/github.com/sourcehawk/operator-component-framework/pkg/mutation/editors). The
generic concept is explained in [Mutation Editors](../primitives.md#mutation-editors).

### PodDisruptionBudgetSpecEditor

The primary API for modifying the PDB spec. Access it via `m.EditSpec`.

Available methods: `SetMinAvailable`, `SetMaxUnavailable`, `ClearMinAvailable`, `ClearMaxUnavailable`, `SetSelector`,
`SetUnhealthyPodEvictionPolicy`, `ClearUnhealthyPodEvictionPolicy`, `Raw`.

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.SetMinAvailable(intstr.FromString("50%"))
    e.SetSelector(&metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "backend"},
    })
    return nil
})
```

#### SetMinAvailable and SetMaxUnavailable

Both methods accept `intstr.IntOrString`, either an integer count or a percentage string (e.g. `"50%"`). These fields
are mutually exclusive in the Kubernetes API. When switching between them, clear the opposing field first:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.ClearMinAvailable()
    e.SetMaxUnavailable(intstr.FromInt32(1))
    return nil
})
```

#### SetUnhealthyPodEvictionPolicy

Controls how unhealthy pods are handled during eviction. Valid values are `policyv1.IfHealthyBudget` and
`policyv1.AlwaysAllow`. Use `ClearUnhealthyPodEvictionPolicy` to revert to the cluster default:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.SetUnhealthyPodEvictionPolicy(policyv1.AlwaysAllow)
    return nil
})
```

For fields not covered by the typed API, use `Raw()`:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.Raw().MinAvailable = &customValue
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

`pdb.ExtractInto` declares that this PDB produces the value of a data cell, which is how you read generated or
server-populated fields. The function receives a value copy of the reconciled PDB after each sync cycle:

```go
expectedPods := concepts.NewData[int32]("expected-pods")

builder := pdb.NewBuilder(base)
pdb.ExtractInto(builder, expectedPods, func(p policyv1.PodDisruptionBudget) (int32, error) {
    // Status.ExpectedPods is populated by the Kubernetes PDB controller.
    return p.Status.ExpectedPods, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(expectedPods)` or read it
opportunistically with `WithOptionalData(expectedPods)`. See [Declared Data](../component.md#declared-data).

## Full Example

```go
func BasePDBMutation(version string) pdb.Mutation {
    return pdb.Mutation{
        Name:    "base-pdb",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *pdb.Mutator) error {
            m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureLabel("app.kubernetes.io/version", version)
                return nil
            })
            return nil
        },
    }
}

func StrictAvailabilityMutation(version string, strict bool) pdb.Mutation {
    return pdb.Mutation{
        Name:    "strict-availability",
        Feature: feature.NewVersionGate(version, nil).When(strict),
        Mutate: func(m *pdb.Mutator) error {
            m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
                e.ClearMinAvailable()
                e.SetMaxUnavailable(intstr.FromInt32(1))
                return nil
            })
            return nil
        },
    }
}

minAvailable := intstr.FromString("50%")
base := &policyv1.PodDisruptionBudget{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "backend-pdb",
        Namespace: owner.Namespace,
    },
    Spec: policyv1.PodDisruptionBudgetSpec{
        MinAvailable: &minAvailable,
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "backend"},
        },
    },
}

resource, err := pdb.NewBuilder(base).
    WithMutation(BasePDBMutation(owner.Spec.Version)).
    WithMutation(StrictAvailabilityMutation(owner.Spec.Version, owner.Spec.StrictMode)).
    Build()
```

When `StrictMode` is true the PDB switches from percentage-based `MinAvailable` to an absolute `MaxUnavailable` of 1.
When false only the base mutation runs and the original `MinAvailable` from the baseline is preserved. Neither mutation
needs to know about the other.

## Guidance

**PDB is Static: there is no operational status.** Registering a PDB in a component contributes no `Operational` or
`Alive` condition. It simply exists or does not. If you need lifecycle signals, use the HPA or another Integration
primitive alongside the PDB.

**`MinAvailable` and `MaxUnavailable` are mutually exclusive.** When switching between them, always clear the opposing
field first. The typed API makes this explicit with `ClearMinAvailable` and `ClearMaxUnavailable`.

**Selector and workload labels must stay in sync.** The PDB selector must match the pod labels of the workload it
protects. If a mutation renames pods or changes their labels, update the PDB selector in the same release.

**Register mutations in dependency order.** If mutation B relies on state set by mutation A, register A first.

**Use data extraction to read `Status` fields.** Fields like `Status.ExpectedPods`, `Status.CurrentHealthy`, and
`Status.DisruptionsAllowed` are populated by the Kubernetes PDB controller after reconciliation. Declare an extraction
into a data cell rather than inspecting the baseline object.
