# ClusterRoleBinding Primitive

The `clusterrolebinding` primitive wraps a Kubernetes `ClusterRoleBinding` and manages the subjects list and object
metadata within the component lifecycle.

!!! warning "Ownership limitation for namespaced owners"

    When a namespaced owner manages a cluster-scoped resource such as a `ClusterRoleBinding`, the framework cannot set a
    controller owner reference (the scopes are incompatible). The owner reference is skipped and the skip is logged.
    The `ClusterRoleBinding` is **not** garbage-collected when the owner is deleted. Manage its lifecycle explicitly (for example with a finalizer on the owner) or use a cluster-scoped owner if automatic cleanup is required. See
    [Cluster-Scoped Resources](../component.md#cluster-scoped-resources) for the full behavior.

## Capabilities

| Capability            | Interfaces / detail                                                                       |
| --------------------- | ----------------------------------------------------------------------------------------- |
| **Static lifecycle**  | `component.Resource`. No health tracking, grace periods, or suspension                    |
| **Mutation**          | `BindingSubjectsEditor` for `.subjects`; `ObjectMetaEditor` for labels and annotations    |
| **Immutable roleRef** | `roleRef` must be set on the base object and cannot be changed after creation             |
| **Cluster-scoped**    | `MarkClusterScoped()` called during construction; `Build()` rejects a non-empty namespace |
| **Guard**             | `concepts.Guardable`: blocks reconciliation when a precondition is not met (`Blocked`)    |
| **Data extraction**   | `concepts.DataExtractable`: reads values back after each sync cycle                       |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full interface-to-status mapping. For
cluster-scoped builder behavior, see [Cluster-Scoped Primitives](../primitives.md#cluster-scoped-primitives).

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
    WithMutation(ExtraSubjectMutation(owner.Spec.Version, owner.Spec.EnableExtra)).
    Build()
```

`Build()` returns an error if `Name` is empty or if `Namespace` is non-empty. The constructor calls
`MarkClusterScoped()` internally, so you do not need to call it manually.

`roleRef` must be set on the base object passed to `NewBuilder`. It is immutable after creation in Kubernetes and is not
modifiable via the mutation API. To change a `roleRef`, delete and recreate the ClusterRoleBinding.

Identity format: `rbac.authorization.k8s.io/v1/ClusterRoleBinding/<name>`.

## Mutations

Each mutation is a named `clusterrolebinding.Mutation` that receives a `*Mutator` and records edit intent through typed
editors. See [The Mutation System](../primitives.md#the-mutation-system) for the full model.

```go
func ExtraSubjectMutation(version string, enabled bool) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "extra-subject",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
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

For boolean conditions, chain `.When()` on the gate. See
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations). For version constraints, see
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

## Internal Mutation Ordering

Within a single mutation, edits are applied in this fixed category order regardless of the call order:

| Step | Category       | What it affects                                    |
| ---- | -------------- | -------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `ClusterRoleBinding` |
| 2    | Subject edits  | `.subjects` entries via `BindingSubjectsEditor`    |

Within each category, edits apply in registration order. Later features observe the object as modified by all earlier
ones.

## Relevant Editors

### BindingSubjectsEditor

The primary API for modifying `.subjects`. Use `m.EditSubjects` for full control. See
[Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

```go
m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
    e.EnsureServiceAccount("my-sa", "default")
    e.RemoveSubject("User", "old-user", "")
    return nil
})
```

#### EnsureSubject

`EnsureSubject` upserts a subject by the combination of `Kind`, `Name`, and `Namespace`. If a matching subject already
exists it is replaced; otherwise the new subject is appended.

```go
e.EnsureSubject(rbacv1.Subject{
    Kind:     "Group",
    Name:     "developers",
    APIGroup: "rbac.authorization.k8s.io",
})
```

#### EnsureServiceAccount

Convenience wrapper that ensures a `ServiceAccount` subject with the given name and namespace exists:

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
    for i := range *raw {
        if (*raw)[i].Kind == "ServiceAccount" {
            (*raw)[i].Namespace = "updated-namespace"
        }
    }
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`. Available methods: `EnsureLabel`, `RemoveLabel`,
`EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/managed-by", "my-operator")
    e.EnsureAnnotation("description", "cluster-wide binding")
    return nil
})
```

## Data Extraction

`WithDataExtractor` runs a callback after successful reconciliation with a value copy of the reconciled
ClusterRoleBinding. Use it to surface binding metadata to other resources:

```go
resource, err := clusterrolebinding.NewBuilder(base).
    WithDataExtractor(func(crb rbacv1.ClusterRoleBinding) error {
        sharedState.ClusterRoleBindingName = crb.Name
        return nil
    }).
    Build()
```

## Full Example

```go
func BaseSubjectMutation(version, saName, saNamespace string) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "base-subject",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *clusterrolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount(saName, saNamespace)
                return nil
            })
            return nil
        },
    }
}

func ExtraSubjectMutation(version string, enabled bool) clusterrolebinding.Mutation {
    return clusterrolebinding.Mutation{
        Name:    "extra-subject",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *clusterrolebinding.Mutator) error {
            m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
                e.EnsureServiceAccount("extra-sa", "monitoring")
                return nil
            })
            return nil
        },
    }
}

resource, err := clusterrolebinding.NewBuilder(base).
    WithMutation(BaseSubjectMutation(owner.Spec.Version, "app-sa", owner.Namespace)).
    WithMutation(ExtraSubjectMutation(owner.Spec.Version, owner.Spec.EnableMonitoring)).
    Build()
```

When `EnableMonitoring` is true, the binding's subjects list contains both the base service account and the monitoring
service account. When false, only the base subject is present.

## Guidance

**Set `roleRef` on the base object, not via mutations.** Kubernetes makes `roleRef` immutable after creation. To change
a `roleRef`, delete and recreate the ClusterRoleBinding.

**Use `EnsureServiceAccount` as a shortcut for the most common subject type.** It sets `Kind`, `Name`, and `Namespace`
in one call and is equivalent to `EnsureSubject` with a `ServiceAccount` kind.

**Cluster-scoped resources are not garbage-collected by namespaced owners.** A namespaced custom resource cannot own a
cluster-scoped `ClusterRoleBinding`. Handle deletion explicitly, for example by adding a finalizer on the owner that
deletes the `ClusterRoleBinding` before the owner is removed.

**Cluster-scoped bindings have no namespace.** The identity format is
`rbac.authorization.k8s.io/v1/ClusterRoleBinding/<name>`. Leave `ObjectMeta.Namespace` empty; `Build()` rejects a
non-empty namespace.
