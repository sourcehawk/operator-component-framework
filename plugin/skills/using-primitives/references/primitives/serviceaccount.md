# ServiceAccount Primitive

The `serviceaccount` primitive wraps a Kubernetes `ServiceAccount` and manages image pull secrets, the automount token
flag, and object metadata within the component lifecycle.

## Capabilities

| Capability           | Interfaces / detail                                                                                 |
| -------------------- | --------------------------------------------------------------------------------------------------- |
| **Static lifecycle** | `component.Resource`. No health tracking, grace periods, or suspension                              |
| **Mutation**         | Direct mutator methods for `.imagePullSecrets` and `.automountServiceAccountToken`; metadata editor |
| **Guard**            | `concepts.Guardable`: blocks reconciliation when a precondition is not met (`Blocked`)              |
| **Data extraction**  | `concepts.DataExtractable`: reads values back after each sync cycle                                 |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full interface-to-status mapping.

## Building a ServiceAccount Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/serviceaccount"

base := &corev1.ServiceAccount{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-sa",
        Namespace: owner.Namespace,
    },
}

resource, err := serviceaccount.NewBuilder(base).
    WithMutation(BaseTokenMutation(owner.Spec.Version)).
    Build()
```

`Build()` returns an error if `Name` or `Namespace` is empty.

Identity format: `v1/ServiceAccount/<namespace>/<name>`.

## Mutations

Each mutation is a named `serviceaccount.Mutation` that receives a `*Mutator` and records edit intent through direct
methods. See [The Mutation System](../primitives.md#the-mutation-system) for the full model.

```go
func BaseTokenMutation(version string) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "base-token",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *serviceaccount.Mutator) error {
            m.EnsureImagePullSecret("default-registry")
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

| Step | Category                | What it affects                                                       |
| ---- | ----------------------- | --------------------------------------------------------------------- |
| 1    | Metadata edits          | Labels and annotations on the `ServiceAccount`                        |
| 2    | Image pull secret edits | `.imagePullSecrets`: `EnsureImagePullSecret`, `RemoveImagePullSecret` |
| 3    | Automount edits         | `.automountServiceAccountToken`: `SetAutomountServiceAccountToken`    |

Within each category, edits apply in registration order. Later features observe the object as modified by all earlier
ones.

## Relevant Editors

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`. Available methods: `EnsureLabel`, `RemoveLabel`,
`EnsureAnnotation`, `RemoveAnnotation`, `Raw`. See [Mutation Editors](../primitives.md#mutation-editors) for the general
editor model.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("managed-by", "my-operator")
    return nil
})
```

## Mutator Methods

The `*serviceaccount.Mutator` exposes direct methods that bypass a nested editor for the two ServiceAccount-specific
fields.

### EnsureImagePullSecret

Adds a named image pull secret to `.imagePullSecrets` if not already present. Idempotent: calling it with an
already-present name is a no-op.

```go
m.EnsureImagePullSecret("registry-creds")
```

### RemoveImagePullSecret

Removes a named image pull secret from `.imagePullSecrets`. No-op if the name is not present.

```go
m.RemoveImagePullSecret("old-registry-creds")
```

### SetAutomountServiceAccountToken

Sets `.automountServiceAccountToken`. Pass `nil` to unset the field.

```go
v := false
m.SetAutomountServiceAccountToken(&v)
```

The pointed-to value is snapshotted at registration time, so later caller-side changes do not affect `Apply()`.

## Data Extraction

`WithDataExtractor` runs a callback after successful reconciliation with a value copy of the reconciled ServiceAccount.
Use it to surface generated fields to other resources:

```go
resource, err := serviceaccount.NewBuilder(base).
    WithDataExtractor(func(sa corev1.ServiceAccount) error {
        sharedState.ServiceAccountName = sa.Name
        return nil
    }).
    Build()
```

## Full Example

```go
func PullSecretMutation(version string) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "pull-secret",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *serviceaccount.Mutator) error {
            m.EnsureImagePullSecret("default-registry")
            return nil
        },
    }
}

func DisableAutomountMutation(version string, disable bool) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "disable-automount",
        Feature: feature.NewVersionGate(version, nil).When(disable),
        Mutate: func(m *serviceaccount.Mutator) error {
            v := false
            m.SetAutomountServiceAccountToken(&v)
            return nil
        },
    }
}

resource, err := serviceaccount.NewBuilder(base).
    WithMutation(PullSecretMutation(owner.Spec.Version)).
    WithMutation(DisableAutomountMutation(owner.Spec.Version, owner.Spec.DisableAutomount)).
    Build()
```

When `DisableAutomount` is true, `.automountServiceAccountToken` is set to `false`. When the condition is not met, the
field stays at its baseline value.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that always run. Use
`feature.NewVersionGate(version, constraints)` when version gating is needed, and chain `.When(bool)` for boolean
conditions.

**Use `EnsureImagePullSecret` for idempotent secret registration.** Multiple features can independently ensure their
required pull secrets without conflicting.

**Register mutations in dependency order.** If one mutation depends on a field set by another, register the dependency
first.

**ServiceAccount is genuinely simple.** The `*Mutator` exposes direct methods rather than a nested editor because the
only mutable fields are `.imagePullSecrets` and `.automountServiceAccountToken`. For anything beyond those fields, use
`EditObjectMetadata`.
