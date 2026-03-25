# Role Primitive

The `role` primitive is the framework's built-in static abstraction for managing Kubernetes `Role` resources. It
integrates with the component lifecycle and provides a structured mutation API for managing RBAC policy rules and object
metadata.

## Capabilities

| Capability            | Detail                                                                                         |
| --------------------- | ---------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state |
| **Mutation pipeline** | Typed editors for `.rules` and object metadata, with a raw escape hatch for free-form access   |
| **Data extraction**   | Reads generated or updated values back from the reconciled Role after each sync cycle          |

## Building a Role Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/role"

base := &rbacv1.Role{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-role",
        Namespace: owner.Namespace,
    },
    Rules: []rbacv1.PolicyRule{
        {
            APIGroups: []string{""},
            Resources: []string{"pods"},
            Verbs:     []string{"get", "list", "watch"},
        },
    },
}

resource, err := role.NewBuilder(base).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `Role` beyond its baseline. Each mutation is a named function that
receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) role.Mutation {
    return role.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *role.Mutator) error {
            m.EditRules(func(e *editors.PolicyRulesEditor) error {
                e.AddRule(rbacv1.PolicyRule{
                    APIGroups: []string{""},
                    Resources: []string{"configmaps"},
                    Verbs:     []string{"get"},
                })
                return nil
            })
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

```go
func SecretAccessMutation(version string, enabled bool) role.Mutation {
    return role.Mutation{
        Name:    "secret-access",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *role.Mutator) error {
            m.EditRules(func(e *editors.PolicyRulesEditor) error {
                e.AddRule(rbacv1.PolicyRule{
                    APIGroups: []string{""},
                    Resources: []string{"secrets"},
                    Verbs:     []string{"get", "list"},
                })
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

func LegacyRoleMutation(version string) role.Mutation {
    return role.Mutation{
        Name: "legacy-role",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *role.Mutator) error {
            m.EditRules(func(e *editors.PolicyRulesEditor) error {
                e.AddRule(rbacv1.PolicyRule{
                    APIGroups: []string{"extensions"},
                    Resources: []string{"ingresses"},
                    Verbs:     []string{"get", "list"},
                })
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are
recorded:

| Step | Category       | What it affects                    |
| ---- | -------------- | ---------------------------------- |
| 1    | Metadata edits | Labels and annotations on the Role |
| 2    | Rules edits    | `.rules` — SetRules, AddRule, Raw  |

Within each category, edits are applied in their registration order. Later features observe the Role as modified by all
previous features.

## Editors

### PolicyRulesEditor

The primary API for modifying `.rules`. Use `m.EditRules` for full control:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.SetRules([]rbacv1.PolicyRule{
        {APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
    })
    return nil
})
```

#### SetRules

`SetRules` replaces the entire rules slice atomically. Use this when the mutation should define the complete set of
rules, discarding any previously accumulated entries.

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.SetRules(desiredRules)
    return nil
})
```

#### AddRule

`AddRule` appends a single rule to the existing rules slice. Use this when a feature contributes additional permissions
without needing to know about rules from other features.

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.AddRule(rbacv1.PolicyRule{
        APIGroups: []string{""},
        Resources: []string{"configmaps"},
        Verbs:     []string{"get", "watch"},
    })
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns a pointer to the underlying `[]rbacv1.PolicyRule` for direct manipulation when none of the structured
methods are sufficient:

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    raw := e.Raw()
    // Filter out rules that grant write access
    filtered := (*raw)[:0]
    for _, r := range *raw {
        if !containsVerb(r.Verbs, "create") {
            filtered = append(filtered, r)
        }
    }
    *raw = filtered
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

## Full Example: Feature-Composed Permissions

```go
func BaseRuleMutation(version string) role.Mutation {
    return role.Mutation{
        Name:    "base-rules",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *role.Mutator) error {
            m.EditRules(func(e *editors.PolicyRulesEditor) error {
                e.SetRules([]rbacv1.PolicyRule{
                    {APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
                })
                return nil
            })
            return nil
        },
    }
}

func SecretAccessMutation(version string, enabled bool) role.Mutation {
    return role.Mutation{
        Name:    "secret-access",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *role.Mutator) error {
            m.EditRules(func(e *editors.PolicyRulesEditor) error {
                e.AddRule(rbacv1.PolicyRule{
                    APIGroups: []string{""},
                    Resources: []string{"secrets"},
                    Verbs:     []string{"get", "list"},
                })
                return nil
            })
            return nil
        },
    }
}

resource, err := role.NewBuilder(base).
    WithMutation(BaseRuleMutation(owner.Spec.Version)).
    WithMutation(SecretAccessMutation(owner.Spec.Version, owner.Spec.EnableTracing)).
    Build()
```

When `EnableTracing` is true, the final Role will contain both the base pod rules and the secrets rule. When false, only
the base rules are applied. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `AddRule` for composable permissions.** When multiple features need to contribute rules to the same Role,
`AddRule` lets each feature add its permissions independently. Using `SetRules` in multiple features means the last
registration wins — only use that when full replacement is the intended semantics.

**Register mutations in dependency order.** If mutation B relies on rules set by mutation A, register A first.

**PolicyRule has no unique key.** There is no upsert or remove-by-key operation. Use `SetRules` to replace atomically,
`AddRule` to accumulate, or `Raw()` for arbitrary manipulation including filtering.
