# ClusterRoleBinding Primitive

The `clusterrolebinding` primitive is the framework's built-in static abstraction for managing Kubernetes
`ClusterRoleBinding` resources. It integrates with the component lifecycle and provides a structured mutation API for
managing `.subjects` entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                                      |
| --------------------- | ----------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension. The resource is reconciled to desired state               |
| **Cluster-scoped**    | Cluster-scoped resource. Build() validates Name and requires metadata.namespace to be empty (errors if set) |
| **Mutation pipeline** | Typed editors for `.subjects` entries and object metadata, with a raw escape hatch for free-form access     |
| **Data extraction**   | Reads generated or updated values back from the reconciled ClusterRoleBinding after each sync cycle         |

## Building a ClusterRoleBinding Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrolebinding"

base := &rbacv1.ClusterRoleBinding{
    ObjectMeta: metav1.ObjectMeta{
        Name: "app-cluster-admin",
    },
    RoleRef: rbacv1.RoleRef{
        APIGroup: "rbac.authorization.k8s.io",
        Kind:     "ClusterRole",
        Name:     "cluster-admin",
    },
    Subjects: []rbacv1.Subject{
        {
            Kind:      "ServiceAccount",
            Name:      "app-sa",
            Namespace: "default",
        },
    },
}

resource, err := clusterrolebinding.NewBuilder(base).
    WithMutation(MySubjectMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ClusterRoleBinding` beyond its baseline. Each mutation is a named
function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MySubjectMutation(version string) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "my-subjects",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *clusterrolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount("my-sa", "default")
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
func ConditionalSubjectMutation(version string, addExtraSubject bool) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "conditional-subject",
        Feature: feature.NewVersionGate(version, nil).When(addExtraSubject),
        Mutate: func(m *clusterrolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount("extra-sa", "monitoring")
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

| Step | Category       | What it affects                                        |
| ---- | -------------- | ------------------------------------------------------ |
| 1    | Metadata edits | Labels and annotations on the `ClusterRoleBinding`     |
| 2    | Subject edits  | `.subjects` entries: Add, Remove, EnsureServiceAccount |

Within each category, edits are applied in their registration order. Later features observe the ClusterRoleBinding as
modified by all previous features.

## Relevant Editors

### BindingSubjectsEditor

The primary API for modifying `.subjects` entries. Use `m.EditSubjects` for full control:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureServiceAccount("my-sa", "default")
    e.RemoveSubject("User", "old-user", "")
    return nil
})
```

#### EnsureSubject

Upserts a subject in the subjects list. A subject is identified by the combination of Kind, Name, and Namespace. If a
matching subject already exists it is replaced; otherwise the new subject is appended:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureSubject(rbacv1.Subject{
        Kind:     "Group",
        Name:     "developers",
        APIGroup: "rbac.authorization.k8s.io",
    })
    return nil
})
```

#### EnsureServiceAccount

Convenience method that ensures a `ServiceAccount` subject with the given name and namespace exists:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureServiceAccount("app-sa", "production")
    return nil
})
```

#### RemoveSubject and RemoveServiceAccount

`RemoveSubject` removes a subject matching the given kind, name, and namespace. `RemoveServiceAccount` is a convenience
wrapper for removing `ServiceAccount` subjects:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.RemoveSubject("User", "old-user", "")
    e.RemoveServiceAccount("deprecated-sa", "default")
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns a pointer to the underlying `[]rbacv1.Subject` for free-form editing:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    raw := e.Raw()
    for i := range *raw {
        if (*raw)[i].Kind == "ServiceAccount" {
            (*raw)[i].Namespace = "updated-namespace"
        }
    }
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/managed-by", "my-operator")
    e.EnsureAnnotation("description", "cluster-wide admin binding")
    return nil
})
```

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for boolean
conditions.

**Cluster-scoped resources have no namespace.** Unlike namespaced primitives, ClusterRoleBinding does not require or
validate a namespace. The identity format is `rbac.authorization.k8s.io/v1/ClusterRoleBinding/<name>`.

**Register mutations in dependency order.** If mutation B relies on a subject added by mutation A, register A first.
