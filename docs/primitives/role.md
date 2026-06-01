# Role Primitive

The `role` primitive wraps a Kubernetes `Role` and manages RBAC policy rules and object metadata within the component
lifecycle.

## Capabilities

| Capability           | Interfaces / detail                                                                     |
| -------------------- | --------------------------------------------------------------------------------------- |
| **Static lifecycle** | `component.Resource`. No health tracking, grace periods, or suspension                  |
| **Mutation**         | `PolicyRulesEditor` for `.rules`; `ObjectMetaEditor` for labels and annotations         |
| **Guard**            | `concepts.Guardable` — blocks reconciliation when a precondition is not met (`Blocked`) |
| **Data extraction**  | `concepts.DataExtractable` — reads values back after each sync cycle                    |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full interface-to-status mapping.

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
    WithMutation(SecretAccessMutation(owner.Spec.Version, owner.Spec.EnableSecretAccess)).
    Build()
```

`Build()` returns an error if `Name` or `Namespace` is empty.

Identity format: `rbac.authorization.k8s.io/v1/Role/<namespace>/<name>`.

## Mutations

Each mutation is a named `role.Mutation` that receives a `*Mutator` and records edit intent through typed editors. See
[The Mutation System](../primitives.md#the-mutation-system) for the full model.

```go
func SecretAccessMutation(version string, enabled bool) role.Mutation {
    return role.Mutation{
        Name:    "secret-access",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
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

For boolean conditions, chain `.When()` on the gate — see
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations). For version constraints, see
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

## Internal Mutation Ordering

Within a single mutation, edits are applied in this fixed category order regardless of the call order:

| Step | Category       | What it affects                        |
| ---- | -------------- | -------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the Role     |
| 2    | Rules edits    | `.rules`: `SetRules`, `AddRule`, `Raw` |

Within each category, edits apply in registration order. Later features observe the object as modified by all earlier
ones.

## Relevant Editors

### PolicyRulesEditor

The primary API for modifying `.rules`. Use `m.EditRules` for full control. See
[Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

#### SetRules

`SetRules` replaces the entire rules slice atomically. Use this when a mutation should define the complete set of rules,
discarding any previously accumulated entries.

```go
m.EditRules(func(e *editors.PolicyRulesEditor) error {
    e.SetRules([]rbacv1.PolicyRule{
        {APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
    })
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

Modifies labels and annotations via `m.EditObjectMetadata`. Available methods: `EnsureLabel`, `RemoveLabel`,
`EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("managed-by", "my-operator")
    return nil
})
```

## Data Extraction

`WithDataExtractor` runs a callback after successful reconciliation with a value copy of the reconciled Role. Use it to
surface the applied rules or metadata to other resources:

```go
resource, err := role.NewBuilder(base).
    WithDataExtractor(func(r rbacv1.Role) error {
        sharedState.RoleName = r.Name
        return nil
    }).
    Build()
```

## Full Example

```go
func BaseRuleMutation(version string) role.Mutation {
    return role.Mutation{
        Name:    "base-rules",
        Feature: feature.NewVersionGate(version, nil),
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
        Feature: feature.NewVersionGate(version, nil).When(enabled),
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
    WithMutation(SecretAccessMutation(owner.Spec.Version, owner.Spec.EnableSecretAccess)).
    Build()
```

When `EnableSecretAccess` is true, the final Role contains both the base pod rules and the secrets rule. When false,
only the base rules are applied. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that always run. Use
`feature.NewVersionGate(version, constraints)` when version gating is needed, and chain `.When(bool)` for boolean
conditions.

**Use `AddRule` for composable permissions.** When multiple features contribute rules to the same Role, `AddRule` lets
each feature add its permissions independently. `SetRules` in multiple features means the last registration wins; only
use that when full replacement is the intended semantics.

**PolicyRule has no unique key.** There is no upsert or remove-by-key operation on rules. Use `SetRules` to replace
atomically, `AddRule` to accumulate, or `Raw()` for arbitrary manipulation including filtering.

**Register mutations in dependency order.** If mutation B relies on rules set by mutation A, register A first.
