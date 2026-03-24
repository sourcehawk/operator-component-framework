# ClusterRole Primitive

The `clusterrole` primitive is the framework's built-in static abstraction for managing Kubernetes `ClusterRole`
resources. It integrates with the component lifecycle and provides a structured mutation API for managing `.rules`,
`.aggregationRule`, and object metadata.

ClusterRole is cluster-scoped: it has no namespace. The builder validates that the Name is set and that Namespace is
empty — setting a namespace on a cluster-scoped resource is rejected.

> **Ownership limitation:** During reconciliation, the framework attempts to set a controller reference on managed
> objects, but only when the owner and dependent scopes are compatible. When a namespaced owner manages a cluster-scoped
> resource such as a `ClusterRole`, the owner reference is skipped (and this is logged) instead of causing the reconcile
> to fail. In this case, the `ClusterRole` is **not** owned by the custom resource for Kubernetes garbage-collection or
> ownership semantics, so it will not be automatically deleted when the owner is removed; you must handle its lifecycle
> explicitly or use a cluster-scoped owner if automatic cleanup is required.

## Capabilities

| Capability            | Detail                                                                                                    |
| --------------------- | --------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state            |
| **Mutation pipeline** | Typed editors for `.rules` and object metadata, with aggregation rule support and a raw escape hatch      |
| **Cluster-scoped**    | No namespace required — identity format is `rbac.authorization.k8s.io/v1/ClusterRole/<name>`              |
| **Flavors**           | Preserves externally-managed fields — labels, annotations, and `.rules` entries not owned by the operator |
| **Data extraction**   | Reads generated or updated values back from the reconciled ClusterRole after each sync cycle              |

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
    WithFieldApplicationFlavor(clusterrole.PreserveExternalRules).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current ClusterRole with a deep copy of the desired object, then restores
server-managed metadata (ResourceVersion, UID, etc.) and shared-controller fields (OwnerReferences, Finalizers) from the
original live object. ClusterRole has no Status subresource, so no status preservation is needed.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := clusterrole.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *rbacv1.ClusterRole) error {
        // Only synchronise owned rules; leave other fields untouched.
        current.Rules = desired.Rules
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ClusterRole` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally:

```go
func PodReadMutation() clusterrole.Mutation {
    return clusterrole.Mutation{
        Name: "pod-read",
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{""},
                Resources: []string{"pods"},
                Verbs:     []string{"get", "list", "watch"},
            })
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder.

### Boolean-gated mutations

```go
func SecretAccessMutation(version string, needsSecrets bool) clusterrole.Mutation {
    return clusterrole.Mutation{
        Name:    "secret-access",
        Feature: feature.NewResourceFeature(version, nil).When(needsSecrets),
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{""},
                Resources: []string{"secrets"},
                Verbs:     []string{"get", "list"},
            })
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyRBACMutation(version string) clusterrole.Mutation {
    return clusterrole.Mutation{
        Name: "legacy-rbac",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *clusterrole.Mutator) error {
            m.AddRule(rbacv1.PolicyRule{
                APIGroups: []string{"extensions"},
                Resources: []string{"deployments"},
                Verbs:     []string{"get", "list"},
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

The Mutator maintains feature boundaries: each feature's mutations are planned together and applied in the order the
features were registered. Within each feature, edits are applied in a fixed category order:

| Step | Category         | What it affects                             |
| ---- | ---------------- | ------------------------------------------- |
| 1    | Metadata edits   | Labels and annotations on the `ClusterRole` |
| 2    | Rules edits      | `.rules` entries — EditRules, AddRule       |
| 3    | Aggregation rule | `.aggregationRule` — SetAggregationRule     |

Within each category, edits are applied in their registration order. For aggregation rules, the last
`SetAggregationRule` call wins within each feature. Later features observe the ClusterRole as modified by all previous
features.

## Editors

### PolicyRulesEditor

The primary API for modifying `.rules` entries. Use `m.EditRules` for full control:

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

#### AddRule

`AddRule` appends a PolicyRule to the rules slice:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.AddRule(rbacv1.PolicyRule{
        APIGroups: []string{""},
        Resources: []string{"configmaps"},
        Verbs:     []string{"get", "list"},
    })
    return nil
})
```

#### RemoveRuleByIndex

`RemoveRuleByIndex` removes the rule at the given index. It is a no-op if the index is out of bounds:

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

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("managed-by", "my-operator")
    return nil
})
```

## Convenience Methods

The `Mutator` exposes a convenience wrapper for the most common `.rules` operation:

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

Setting the aggregation rule to nil clears it. Within a single feature, the last `SetAggregationRule` call wins.

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external
controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := clusterrole.NewBuilder(base).
    WithFieldApplicationFlavor(clusterrole.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on
overlap.

```go
resource, err := clusterrole.NewBuilder(base).
    WithFieldApplicationFlavor(clusterrole.PreserveCurrentAnnotations).
    Build()
```

### PreserveExternalRules

Preserves `.rules` entries present on the live object but absent from the applied desired state. Rules are compared
using all `PolicyRule` fields (APIGroups, Resources, Verbs, ResourceNames, NonResourceURLs), treating these slice fields
as sets so that order differences are ignored.

Use this when other controllers or admission webhooks inject rules into the ClusterRole that your operator does not own:

```go
resource, err := clusterrole.NewBuilder(base).
    WithFieldApplicationFlavor(clusterrole.PreserveExternalRules).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Full Example: Feature-Composed RBAC

```go
func CoreRulesMutation(version string) clusterrole.Mutation {
    return clusterrole.Mutation{
        Name:    "core-rules",
        Feature: feature.NewResourceFeature(version, nil),
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
        Feature: feature.NewResourceFeature(version, nil).When(manageCRDs),
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
    WithMutation(CoreRulesMutation(owner.Spec.Version)).
    WithMutation(CRDAccessMutation(owner.Spec.Version, owner.Spec.ManageCRDs)).
    Build()
```

When `ManageCRDs` is true, the final rules include both core and CRD access rules. When false, only the core rules are
written. Neither mutation needs to know about the other.

> **Note:** Do not combine `PreserveExternalRules` with feature-gated mutations that add and remove rules. Because
> flavors run before mutations and preserve rules from the live object, previously-added rules will be retained even
> after a feature gate is disabled, and rules can be duplicated if a mutation re-adds a rule already present on the live
> object. Use `PreserveExternalRules` only when external controllers or admission webhooks manage rules that your
> operator does not own.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `PreserveExternalRules` when sharing a ClusterRole.** If admission webhooks, external controllers, or manual
operations add rules to a ClusterRole your operator manages, this flavor prevents your operator from silently deleting
those rules each reconcile cycle.

**Use `SetAggregationRule` for composite roles.** When you want the API server to aggregate rules from multiple
ClusterRoles based on label selectors, use `SetAggregationRule` instead of managing `.rules` directly. The two
approaches are mutually exclusive in the Kubernetes API — the API server ignores `.rules` when `.aggregationRule` is
set.

**Register mutations in dependency order.** If mutation B relies on a rule added by mutation A, register A first.
