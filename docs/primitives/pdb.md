# PodDisruptionBudget Primitive

The `pdb` primitive is the framework's built-in static abstraction for managing Kubernetes `PodDisruptionBudget` resources. It integrates with the component lifecycle and provides a structured mutation API for managing disruption policies and object metadata.

## Capabilities

| Capability            | Detail                                                                                          |
|-----------------------|-------------------------------------------------------------------------------------------------|
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state  |
| **Mutation pipeline** | Typed editors for PDB spec and object metadata, with a raw escape hatch for free-form access    |
| **Flavors**           | Preserves externally-managed fields — labels and annotations not owned by the operator          |
| **Data extraction**   | Reads generated or updated values back from the reconciled PDB after each sync cycle            |

## Building a PDB Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/pdb"

minAvailable := intstr.FromString("50%")
base := &policyv1.PodDisruptionBudget{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-server-pdb",
        Namespace: owner.Namespace,
    },
    Spec: policyv1.PodDisruptionBudgetSpec{
        MinAvailable: &minAvailable,
        Selector: &metav1.LabelSelector{
            MatchLabels: map[string]string{"app": "web-server"},
        },
    },
}

resource, err := pdb.NewBuilder(base).
    WithFieldApplicationFlavor(pdb.PreserveCurrentLabels).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current PodDisruptionBudget with a deep copy of the desired object. This ensures every reconciliation cycle produces a clean, predictable state and avoids any drift from the desired baseline.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := pdb.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *policyv1.PodDisruptionBudget) error {
        // Only synchronise the spec; leave metadata untouched.
        current.Spec = *desired.Spec.DeepCopy()
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `PodDisruptionBudget` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) pdb.Mutation {
    return pdb.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *pdb.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

```go
func StrictAvailabilityMutation(version string, enabled bool) pdb.Mutation {
    return pdb.Mutation{
        Name:    "strict-availability",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
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

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyPDBMutation(version string) pdb.Mutation {
    return pdb.Mutation{
        Name: "legacy-pdb",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *pdb.Mutator) error {
            m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
                e.SetMinAvailable(intstr.FromInt32(1))
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are recorded:

| Step | Category       | What it affects                                    |
|------|----------------|----------------------------------------------------|
| 1    | Metadata edits | Labels and annotations on the `PodDisruptionBudget` |
| 2    | Spec edits     | MinAvailable, MaxUnavailable, selector, eviction policy |

Within each category, edits are applied in their registration order. Later features observe the PodDisruptionBudget as modified by all previous features.

## Editors

### PodDisruptionBudgetSpecEditor

The primary API for modifying the PDB spec. Use `m.EditSpec` for full control:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.SetMinAvailable(intstr.FromString("50%"))
    e.SetSelector(&metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "web"},
    })
    return nil
})
```

#### SetMinAvailable and SetMaxUnavailable

`SetMinAvailable` sets the minimum number of pods that must remain available during a disruption. `SetMaxUnavailable` sets the maximum number of pods that can be unavailable. Both accept `intstr.IntOrString` — either an integer count or a percentage string (e.g. `"50%"`).

These fields are mutually exclusive in the Kubernetes API. Use `ClearMinAvailable` or `ClearMaxUnavailable` to remove the opposing constraint when switching between them:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.ClearMinAvailable()
    e.SetMaxUnavailable(intstr.FromInt32(1))
    return nil
})
```

#### SetSelector

`SetSelector` replaces the pod selector used by the PDB:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.SetSelector(&metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "web", "tier": "frontend"},
    })
    return nil
})
```

#### SetUnhealthyPodEvictionPolicy

`SetUnhealthyPodEvictionPolicy` controls how unhealthy pods are handled during eviction. Valid values are `policyv1.IfHealthyBudget` and `policyv1.AlwaysAllow`. Use `ClearUnhealthyPodEvictionPolicy` to revert to the cluster default:

```go
m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
    e.SetUnhealthyPodEvictionPolicy(policyv1.AlwaysAllow)
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns the underlying `*policyv1.PodDisruptionBudgetSpec` for direct access when the typed API is insufficient:

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
    e.EnsureAnnotation("pdb.example.io/policy", "strict")
    return nil
})
```

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := pdb.NewBuilder(base).
    WithFieldApplicationFlavor(pdb.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on overlap.

```go
resource, err := pdb.NewBuilder(base).
    WithFieldApplicationFlavor(pdb.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Full Example: Feature-Gated Disruption Policy

```go
func BasePDBMutation(version string) pdb.Mutation {
    return pdb.Mutation{
        Name:    "base-pdb",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *pdb.Mutator) error {
            m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureLabel("app.kubernetes.io/version", version)
                return nil
            })
            return nil
        },
    }
}

func StrictAvailabilityMutation(version string, enabled bool) pdb.Mutation {
    return pdb.Mutation{
        Name:    "strict-availability",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
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

resource, err := pdb.NewBuilder(base).
    WithFieldApplicationFlavor(pdb.PreserveCurrentLabels).
    WithMutation(BasePDBMutation(owner.Spec.Version)).
    WithMutation(StrictAvailabilityMutation(owner.Spec.Version, owner.Spec.StrictMode)).
    Build()
```

When `StrictMode` is true, the PDB switches from percentage-based `MinAvailable` to an absolute `MaxUnavailable` of 1. When false, only the base mutation runs and the original `MinAvailable` from the baseline is preserved. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use `feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for boolean conditions.

**`MinAvailable` and `MaxUnavailable` are mutually exclusive.** When switching between them, always clear the opposing field first. The typed API makes this explicit with `ClearMinAvailable` and `ClearMaxUnavailable`.

**Register mutations in dependency order.** If mutation B relies on state set by mutation A, register A first.
