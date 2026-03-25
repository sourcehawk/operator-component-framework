# RoleBinding Primitive

The `rolebinding` primitive is the framework's built-in static abstraction for managing Kubernetes `RoleBinding`
resources. It integrates with the component lifecycle and provides a structured mutation API for managing subjects and
object metadata.

## Capabilities

| Capability            | Detail                                                                                                   |
| --------------------- | -------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state           |
| **Mutation pipeline** | Typed editors for subjects and object metadata, with a raw escape hatch for free-form access             |
| **Immutable roleRef** | `roleRef` must be set on the base object and cannot be changed after creation (requires delete/recreate) |
| **Data extraction**   | Reads generated or updated values back from the reconciled RoleBinding after each sync cycle             |

## Building a RoleBinding Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/rolebinding"

base := &rbacv1.RoleBinding{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-rolebinding",
        Namespace: owner.Namespace,
    },
    RoleRef: rbacv1.RoleRef{
        APIGroup: "rbac.authorization.k8s.io",
        Kind:     "Role",
        Name:     "app-role",
    },
    Subjects: []rbacv1.Subject{
        {Kind: "ServiceAccount", Name: "app-sa", Namespace: owner.Namespace},
    },
}

resource, err := rolebinding.NewBuilder(base).
    WithMutation(MySubjectMutation(owner.Spec.Version)).
    Build()
```

`roleRef` must be set on the base object passed to `NewBuilder`. It is immutable after creation in Kubernetes and is not
modifiable via the mutation API.

## Mutations

Mutations are the primary mechanism for modifying a `RoleBinding` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func AddServiceAccountMutation(version, saName, saNamespace string) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name:    "add-service-account",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureSubject(rbacv1.Subject{
                    Kind:      "ServiceAccount",
                    Name:      saName,
                    Namespace: saNamespace,
                })
                return nil
            })
            return nil
        },
    }
}
```

### Boolean-gated mutations

```go
func MonitoringSubjectMutation(version string, enabled bool) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name:    "monitoring-subject",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureSubject(rbacv1.Subject{
                    Kind:      "ServiceAccount",
                    Name:      "monitoring-agent",
                    Namespace: "monitoring",
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

func LegacySubjectMutation(version string) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name: "legacy-subject",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureSubject(rbacv1.Subject{
                    Kind: "User",
                    Name: "legacy-admin",
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

| Step | Category       | What it affects                               |
| ---- | -------------- | --------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the RoleBinding     |
| 2    | Subject edits  | `.subjects` entries via BindingSubjectsEditor |

Within each category, edits are applied in their registration order. Later features observe the RoleBinding as modified
by all previous features.

## Relevant Editors

### BindingSubjectsEditor

The primary API for modifying the subjects list. Use `m.EditSubjects` for full control:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureSubject(rbacv1.Subject{
        Kind:      "ServiceAccount",
        Name:      "my-sa",
        Namespace: "default",
    })
    e.RemoveSubject("ServiceAccount", "old-sa", "default")
    return nil
})
```

#### EnsureSubject

`EnsureSubject` upserts a subject by the combination of `Kind`, `Name`, and `Namespace`. If a matching subject already
exists, it is replaced; otherwise the new subject is appended.

#### RemoveSubject

`RemoveSubject` removes a subject identified by kind, name, and namespace. It is a no-op if no matching subject exists.

#### Raw

`Raw()` returns a pointer to the underlying `[]rbacv1.Subject` slice for free-form access when the structured methods
are insufficient:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    raw := e.Raw()
    *raw = append(*raw, rbacv1.Subject{
        Kind: "Group",
        Name: "developers",
    })
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/managed-by", "my-operator")
    e.EnsureAnnotation("operator.example.io/version", version)
    return nil
})
```

## Guidance

**Set `roleRef` on the base object, not via mutations.** Kubernetes makes `roleRef` immutable after creation. To change
a `roleRef`, delete and recreate the RoleBinding.

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `EnsureSubject` for idempotent subject management.** `EnsureSubject` upserts by Kind+Name+Namespace, making it
safe to call on every reconciliation without creating duplicates.

**Register mutations in dependency order.** If mutation B relies on a subject added by mutation A, register A first.
