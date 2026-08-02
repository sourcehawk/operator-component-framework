# ClusterRole Primitive

The `clusterrole` primitive wraps a Kubernetes `ClusterRole` and manages RBAC policy rules, aggregation rules, and
object metadata within the component lifecycle.

!!! warning "Ownership limitation for namespaced owners"

    When a namespaced owner manages a cluster-scoped resource such as a `ClusterRole`, the framework cannot set a
    controller owner reference (the scopes are incompatible). The owner reference is skipped and the skip is logged.
    The `ClusterRole` is **not** garbage-collected when the owner is deleted. Manage its lifecycle explicitly (for
    example with a finalizer on the owner) or use a cluster-scoped owner if automatic cleanup is required. See
    [Cluster-Scoped Resources](../component.md#cluster-scoped-resources) for the full behavior.

## Capabilities

| Capability           | Interfaces / detail                                                                                                          |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| **Static lifecycle** | `component.Resource`. No health tracking, grace periods, or suspension                                                       |
| **Mutation**         | `PolicyRulesEditor` for `.rules`; `SetAggregationRule` for `.aggregationRule`; `ObjectMetaEditor` for labels and annotations |
| **Cluster-scoped**   | `MarkClusterScoped()` called during construction; `Build()` rejects a non-empty namespace                                    |
| **Guard**            | `concepts.Guardable`: blocks reconciliation when a precondition is not met (`Blocked`)                                       |
| **Data extraction**  | `concepts.DataExtractable`: reads values back after each sync cycle                                                          |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full interface-to-status mapping. For
cluster-scoped builder behavior, see [Cluster-Scoped Primitives](../primitives.md#cluster-scoped-primitives).

## Building a ClusterRole Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrole"

base := &rbacv1.ClusterRole{
    ObjectMeta: metav1.ObjectMeta{
        Name: "my-operator-role",
    },
    Rules: []rbacv1.PolicyRule{
        {
            APIGroups: []string{""},
            Resources: []string{"pods"},
            Verbs:     []string{"get", "list", "watch"},
        },
    },
}

resource, err := clusterrole.NewBuilder(base).
    WithMutation(CRDAccessMutation(owner.Spec.Version, owner.Spec.ManageCRDs)).
    Build()
```

`Build()` returns an error if `Name` is empty or if `Namespace` is non-empty. The constructor calls
`MarkClusterScoped()` internally, so you do not need to call it manually.

Identity format: `rbac.authorization.k8s.io/v1/ClusterRole/<name>`.

## Mutations

Each mutation is a named `clusterrole.Mutation` that receives a `*Mutator` and records edit intent through typed
editors. See [The Mutation System](../primitives.md#the-mutation-system) for the full model.

```go
func CRDAccessMutation(version string, manageCRDs bool) clusterrole.Mutation {
    return clusterrole.Mutation{
        Name:    "crd-access",
        Feature: feature.NewVersionGate(version, nil).When(manageCRDs),
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{"apiextensions.k8s.io"},
                Resources: []string{"customresourcedefinitions"},
                Verbs:     []string{"get", "list", "watch"},
            })
            return nil
        },
    }
}
```

For boolean conditions, chain `.When()` on the gate. See
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations). For version constraints, see
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

## Internal Mutation Ordering

Within a single mutation, edits are applied in this fixed category order regardless of the call order:

| Step | Category         | What it affects                                                               |
| ---- | ---------------- | ----------------------------------------------------------------------------- |
| 1    | Metadata edits   | Labels and annotations on the `ClusterRole`                                   |
| 2    | Rules edits      | `.rules`: `EditRules`, `AddRule`                                              |
| 3    | Aggregation rule | `.aggregationRule`: `SetAggregationRule` (last call wins within each feature) |

Within each category, edits apply in registration order. Later features observe the object as modified by all earlier
ones.

## Relevant Editors

### PolicyRulesEditor

The primary API for modifying `.rules`. Use `m.EditRules` for full control. See
[Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

#### AddRule

`AddRule` appends a `PolicyRule` to the rules slice:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.AddRule(rbacv1.PolicyRule{
        APIGroups: []string{"apps"},
        Resources: []string{"deployments"},
        Verbs:     []string{"get", "list", "watch"},
    })
    return nil
})
```

#### RemoveRuleByIndex

`RemoveRuleByIndex` removes the rule at the given index. No-op if the index is out of bounds:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.RemoveRuleByIndex(0) // remove the first rule
    return nil
})
```

#### Clear

`Clear` removes all rules:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.Clear()
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns a pointer to the underlying `[]rbacv1.PolicyRule` for free-form editing:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    raw := e.Raw()
    *raw = append(*raw, customRules...)
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`. Available methods: `EnsureLabel`, `RemoveLabel`,
`EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("managed-by", "my-operator")
    return nil
})
```

## Convenience Methods

The `*Mutator` exposes a direct convenience method for the most common `.rules` operation:

| Method          | Equivalent to                   |
| --------------- | ------------------------------- |
| `AddRule(rule)` | `EditRules` → `e.AddRule(rule)` |

Use `AddRule` for simple, single-rule mutations. Use `EditRules` when you need multiple operations or raw access in a
single edit block.

## SetAggregationRule

`SetAggregationRule` sets the ClusterRole's `.aggregationRule` field. An aggregation rule causes the API server to
combine rules from ClusterRoles whose labels match the provided selectors, instead of using `.rules` directly:

```go
m.SetAggregationRule(&rbacv1.AggregationRule{
    ClusterRoleSelectors: []metav1.LabelSelector{
        {MatchLabels: map[string]string{"rbac.example.com/aggregate-to-admin": "true"}},
    },
})
```

Pass `nil` to clear the aggregation rule. Within a single feature, the last `SetAggregationRule` call wins.

!!! note

    The Kubernetes API ignores `.rules` when `.aggregationRule` is set. The two approaches are mutually exclusive.

## Data Extraction

`clusterrole.ExtractInto` declares that this ClusterRole produces the value of a data cell. The function receives a
value copy of the reconciled ClusterRole and runs immediately after each sync cycle:

```go
roleName := concepts.NewData[string]("viewer-cluster-role")

builder := clusterrole.NewBuilder(base)
clusterrole.ExtractInto(builder, roleName, func(cr rbacv1.ClusterRole) (string, error) {
    return cr.Name, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(roleName)` or read it
opportunistically with `WithOptionalData(roleName)`. See [Declared Data](../component.md#declared-data).

## Full Example

```go
func CoreRulesMutation() clusterrole.Mutation {
    return clusterrole.Mutation{
        Name: "core-rules",
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{""},
                Resources: []string{"pods", "services", "configmaps"},
                Verbs:     []string{"get", "list", "watch"},
            })
            return nil
        },
    }
}

func CRDAccessMutation(version string, manageCRDs bool) clusterrole.Mutation {
    return clusterrole.Mutation{
        Name:    "crd-access",
        Feature: feature.NewVersionGate(version, nil).When(manageCRDs),
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{"apiextensions.k8s.io"},
                Resources: []string{"customresourcedefinitions"},
                Verbs:     []string{"get", "list", "watch"},
            })
            return nil
        },
    }
}

resource, err := clusterrole.NewBuilder(base).
    WithMutation(CoreRulesMutation()).
    WithMutation(CRDAccessMutation(owner.Spec.Version, owner.Spec.ManageCRDs)).
    Build()
```

When `ManageCRDs` is true, the final rules include both core and CRD access rules. When false, only the core rules are
written. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that always run. Use
`feature.NewVersionGate(version, constraints)` when version gating is needed, and chain `.When(bool)` for boolean
conditions.

**Use `AddRule` for composable permissions.** `AddRule` lets each feature contribute rules without knowing about others.
Using `SetRules` (via `Raw`) in multiple features means the last write wins; use that only when full replacement is the
intended semantics.

**Use `SetAggregationRule` for composite roles.** When you want the API server to aggregate rules from multiple
ClusterRoles via label selectors, call `SetAggregationRule` instead of managing `.rules` directly. Do not mix both
approaches on the same role.

**Cluster-scoped resources are not garbage-collected by namespaced owners.** A namespaced custom resource cannot own a
cluster-scoped `ClusterRole`. Handle deletion explicitly, for example by adding a finalizer on the owner that deletes
the `ClusterRole` before the owner is removed.
