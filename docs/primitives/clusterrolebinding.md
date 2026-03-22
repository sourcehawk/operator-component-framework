# ClusterRoleBinding Primitive

The `clusterrolebinding` primitive is the framework's built-in static abstraction for managing Kubernetes `ClusterRoleBinding` resources. It integrates with the component lifecycle and provides a structured mutation API for managing `.subjects` entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                               |
|-----------------------|------------------------------------------------------------------------------------------------------|
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state       |
| **Cluster-scoped**    | No namespace required — only Name is validated during Build()                                        |
| **Immutable roleRef** | `DefaultFieldApplicator` preserves `roleRef` on updates since it is immutable after creation         |
| **Mutation pipeline** | Typed editors for `.subjects` entries and object metadata, with a raw escape hatch for free-form access |
| **Flavors**           | Preserves externally-managed fields — labels and annotations not owned by the operator               |
| **Data extraction**   | Reads generated or updated values back from the reconciled ClusterRoleBinding after each sync cycle  |

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
    WithFieldApplicationFlavor(clusterrolebinding.PreserveCurrentLabels).
    WithMutation(MySubjectMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current ClusterRoleBinding with a deep copy of the desired object, preserving `roleRef` on updates. The `roleRef` field is immutable after creation in the Kubernetes RBAC API — attempting to change it results in an API error.

When the current object already exists in the cluster (has a non-empty `ResourceVersion`), the applicator restores the original `roleRef` after copying. On initial creation, the desired `roleRef` is used as-is.

Use `WithCustomFieldApplicator` when you need different field application behaviour:

```go
resource, err := clusterrolebinding.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *rbacv1.ClusterRoleBinding) error {
        // Custom merge logic
        current.Subjects = desired.DeepCopy().Subjects
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ClusterRoleBinding` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature with no version constraints and no `When()` conditions is also always enabled:

```go
func MySubjectMutation(version string) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "my-subjects",
        Feature: feature.NewResourceFeature(version, nil),
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

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

```go
func ConditionalSubjectMutation(version string, addExtraSubject bool) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "conditional-subject",
        Feature: feature.NewResourceFeature(version, nil).When(addExtraSubject),
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

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are recorded:

| Step | Category        | What it affects                                         |
|------|-----------------|---------------------------------------------------------|
| 1    | Metadata edits  | Labels and annotations on the `ClusterRoleBinding`      |
| 2    | Subject edits   | `.subjects` entries — Add, Remove, EnsureServiceAccount |

Within each category, edits are applied in their registration order. Later features observe the ClusterRoleBinding as modified by all previous features.

## Editors

### BindingSubjectsEditor

The primary API for modifying `.subjects` entries. Use `m.EditSubjects` for full control:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureServiceAccount("my-sa", "default")
    e.Remove("User", "old-user", "")
    return nil
})
```

#### EnsureServiceAccount

Ensures a `ServiceAccount` subject with the given name and namespace exists. If an identical subject already exists, this is a no-op:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureServiceAccount("app-sa", "production")
    return nil
})
```

#### Add

Appends a subject to the list without checking for duplicates:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.Add(rbacv1.Subject{
        Kind:     "Group",
        Name:     "developers",
        APIGroup: "rbac.authorization.k8s.io",
    })
    return nil
})
```

#### Remove and RemoveServiceAccount

`Remove` removes all subjects matching the given kind, name, and namespace. `RemoveServiceAccount` is a convenience wrapper for removing `ServiceAccount` subjects:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.Remove("User", "old-user", "")
    e.RemoveServiceAccount("deprecated-sa", "default")
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns the underlying `[]rbacv1.Subject` for free-form editing:

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    raw := e.Raw()
    for i := range raw {
        if raw[i].Kind == "ServiceAccount" {
            raw[i].Namespace = "updated-namespace"
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

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := clusterrolebinding.NewBuilder(base).
    WithFieldApplicationFlavor(clusterrolebinding.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on overlap.

```go
resource, err := clusterrolebinding.NewBuilder(base).
    WithFieldApplicationFlavor(clusterrolebinding.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use `feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for boolean conditions.

**`roleRef` is immutable.** The default field applicator preserves the existing `roleRef` on updates. To change a `roleRef`, delete the ClusterRoleBinding and recreate it — the Kubernetes API does not support in-place updates to this field.

**Cluster-scoped resources have no namespace.** Unlike namespaced primitives, ClusterRoleBinding does not require or validate a namespace. The identity format is `rbac.authorization.k8s.io/v1/ClusterRoleBinding/<name>`.

**Register mutations in dependency order.** If mutation B relies on a subject added by mutation A, register A first.
