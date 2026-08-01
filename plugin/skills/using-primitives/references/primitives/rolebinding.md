# RoleBinding Primitive

The `rolebinding` primitive wraps a Kubernetes `RoleBinding` and manages the subjects list and object metadata within
the component lifecycle.

## Capabilities

| Capability            | Interfaces / detail                                                                    |
| --------------------- | -------------------------------------------------------------------------------------- |
| **Static lifecycle**  | `component.Resource`. No health tracking, grace periods, or suspension                 |
| **Mutation**          | `BindingSubjectsEditor` for `.subjects`; `ObjectMetaEditor` for labels and annotations |
| **Immutable roleRef** | `roleRef` must be set on the base object and cannot be changed after creation          |
| **Guard**             | `concepts.Guardable`: blocks reconciliation when a precondition is not met (`Blocked`) |
| **Data extraction**   | `concepts.DataExtractable`: reads values back after each sync cycle                    |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full interface-to-status mapping.

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
    WithMutation(MonitoringSubjectMutation(owner.Spec.Version, owner.Spec.EnableMonitoring)).
    Build()
```

`Build()` returns an error if `Name` or `Namespace` is empty, or if `roleRef.APIGroup`, `roleRef.Kind`, or
`roleRef.Name` is empty.

`roleRef` must be set on the base object passed to `NewBuilder`. It is immutable after creation in Kubernetes and is not
modifiable via the mutation API. To change a `roleRef`, delete and recreate the RoleBinding.

Identity format: `rbac.authorization.k8s.io/v1/RoleBinding/<namespace>/<name>`.

## Mutations

Each mutation is a named `rolebinding.Mutation` that receives a `*Mutator` and records edit intent through typed
editors. See [The Mutation System](../primitives.md#the-mutation-system) for the full model.

```go
func MonitoringSubjectMutation(version string, enabled bool) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name:    "monitoring-subject",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount("monitoring-agent", "monitoring")
                return nil
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

| Step | Category       | What it affects                                 |
| ---- | -------------- | ----------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the RoleBinding       |
| 2    | Subject edits  | `.subjects` entries via `BindingSubjectsEditor` |

Within each category, edits apply in registration order. Later features observe the object as modified by all earlier
ones.

## Relevant Editors

### BindingSubjectsEditor

The primary API for modifying the subjects list. Use `m.EditSubjects` for full control. See
[Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

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
exists it is replaced; otherwise the new subject is appended.

#### EnsureServiceAccount

Convenience wrapper that ensures a `ServiceAccount` subject with the given name and namespace exists.

```go
e.EnsureServiceAccount("app-sa", "production")
```

#### RemoveSubject and RemoveServiceAccount

`RemoveSubject` removes a subject identified by kind, name, and namespace. `RemoveServiceAccount` is a convenience
wrapper for removing `ServiceAccount` subjects:

```go
e.RemoveSubject("User", "old-user", "")
e.RemoveServiceAccount("deprecated-sa", "default")
```

#### Raw Escape Hatch

`Raw()` returns a pointer to the underlying `[]rbacv1.Subject` for free-form editing:

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

Modifies labels and annotations via `m.EditObjectMetadata`. Available methods: `EnsureLabel`, `RemoveLabel`,
`EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/managed-by", "my-operator")
    e.EnsureAnnotation("operator.example.io/version", version)
    return nil
})
```

## Data Extraction

`WithDataExtractor` runs a callback after successful reconciliation with a value copy of the reconciled RoleBinding. Use
it to surface binding metadata to other resources:

```go
resource, err := rolebinding.NewBuilder(base).
    WithDataExtractor(func(rb rbacv1.RoleBinding) error {
        sharedState.RoleBindingName = rb.Name
        return nil
    }).
    Build()
```

## Full Example

```go
func BaseSubjectMutation(version string, saName, saNamespace string) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name:    "base-subject",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount(saName, saNamespace)
                return nil
            })
            return nil
        },
    }
}

func MonitoringSubjectMutation(version string, enabled bool) rolebinding.Mutation {
    return rolebinding.Mutation{
        Name:    "monitoring-subject",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *rolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount("monitoring-agent", "monitoring")
                return nil
            })
            return nil
        },
    }
}

resource, err := rolebinding.NewBuilder(base).
    WithMutation(BaseSubjectMutation(owner.Spec.Version, "app-sa", owner.Namespace)).
    WithMutation(MonitoringSubjectMutation(owner.Spec.Version, owner.Spec.EnableMonitoring)).
    Build()
```

When `EnableMonitoring` is true, the binding's subjects list contains both the base service account and the monitoring
agent. When false, only the base subject is present. Neither mutation needs to know about the other.

## Guidance

**Set `roleRef` on the base object, not via mutations.** Kubernetes makes `roleRef` immutable after creation. To change
a `roleRef`, delete and recreate the RoleBinding.

**Use `EnsureSubject` for idempotent subject management.** `EnsureSubject` upserts by Kind+Name+Namespace, making it
safe to call on every reconciliation without creating duplicates.

**Use `EnsureServiceAccount` as a shortcut for the most common subject type.** It sets `Kind`, `Name`, and `Namespace`
in one call and is equivalent to `EnsureSubject` with a `ServiceAccount` kind.

**Register mutations in dependency order.** If mutation B relies on a subject added by mutation A, register A first.
