# ServiceAccount Primitive

The `serviceaccount` primitive is the framework's built-in static abstraction for managing Kubernetes `ServiceAccount`
resources. It integrates with the component lifecycle and provides a structured mutation API for managing image pull
secrets, the automount token flag, and object metadata.

## Capabilities

| Capability            | Detail                                                                                                    |
| --------------------- | --------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state            |
| **Mutation pipeline** | Direct mutator methods for `.imagePullSecrets` and `.automountServiceAccountToken`, plus metadata editors |
| **Flavors**           | Preserves externally-managed fields — labels and annotations not owned by the operator                    |
| **Data extraction**   | Reads generated or updated values back from the reconciled ServiceAccount after each sync cycle           |

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
    WithFieldApplicationFlavor(serviceaccount.PreserveCurrentLabels).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current ServiceAccount with a deep copy of the desired object, then restores
server-managed metadata (ResourceVersion, UID, etc.), shared-controller fields (OwnerReferences, Finalizers), and the
live `.secrets` field from the original object. The `.secrets` field is populated by the token controller and is not
owned by the primitive — any `.secrets` value on the desired object is discarded. ServiceAccount has no Status
subresource, so no status preservation is needed.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := serviceaccount.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *corev1.ServiceAccount) error {
        // Only synchronise owned fields; leave other fields untouched.
        current.ImagePullSecrets = desired.ImagePullSecrets
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ServiceAccount` beyond its baseline. Each mutation is a named
function that receives a `*Mutator` and records edit intent through direct methods.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *serviceaccount.Mutator) error {
            m.EnsureImagePullSecret("my-registry")
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

```go
func PrivateRegistryMutation(version string, usePrivateRegistry bool) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "private-registry",
        Feature: feature.NewResourceFeature(version, nil).When(usePrivateRegistry),
        Mutate: func(m *serviceaccount.Mutator) error {
            m.EnsureImagePullSecret("private-registry-creds")
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyTokenMutation(version string) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name: "legacy-token",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *serviceaccount.Mutator) error {
            v := true
            m.SetAutomountServiceAccountToken(&v)
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are
recorded:

| Step | Category                | What it affects                                                    |
| ---- | ----------------------- | ------------------------------------------------------------------ |
| 1    | Metadata edits          | Labels and annotations on the `ServiceAccount`                     |
| 2    | Image pull secret edits | `.imagePullSecrets` — EnsureImagePullSecret, RemoveImagePullSecret |
| 3    | Automount edits         | `.automountServiceAccountToken` — SetAutomountServiceAccountToken  |

Within each category, edits are applied in their registration order. Later features observe the ServiceAccount as
modified by all previous features.

## Mutator Methods

### EnsureImagePullSecret

Adds a named image pull secret to `.imagePullSecrets` if not already present. Idempotent — calling it with an
already-present name is a no-op.

```go
m.EnsureImagePullSecret("my-registry-creds")
```

### RemoveImagePullSecret

Removes a named image pull secret from `.imagePullSecrets`. It is a no-op if the secret is not present.

```go
m.RemoveImagePullSecret("old-registry-creds")
```

### SetAutomountServiceAccountToken

Sets `.automountServiceAccountToken` to the provided value. Pass `nil` to unset the field.

```go
v := false
m.SetAutomountServiceAccountToken(&v)
```

### EditObjectMetadata

Modifies labels and annotations via `editors.ObjectMetaEditor`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("managed-by", "my-operator")
    return nil
})
```

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external
controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := serviceaccount.NewBuilder(base).
    WithFieldApplicationFlavor(serviceaccount.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on
overlap.

```go
resource, err := serviceaccount.NewBuilder(base).
    WithFieldApplicationFlavor(serviceaccount.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Full Example: Feature-Composed ServiceAccount

```go
func BaseImagePullSecretMutation(version string) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "base-pull-secret",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *serviceaccount.Mutator) error {
            m.EnsureImagePullSecret("default-registry")
            return nil
        },
    }
}

func DisableAutomountMutation(version string, disableAutomount bool) serviceaccount.Mutation {
    return serviceaccount.Mutation{
        Name:    "disable-automount",
        Feature: feature.NewResourceFeature(version, nil).When(disableAutomount),
        Mutate: func(m *serviceaccount.Mutator) error {
            v := false
            m.SetAutomountServiceAccountToken(&v)
            return nil
        },
    }
}

resource, err := serviceaccount.NewBuilder(base).
    WithFieldApplicationFlavor(serviceaccount.PreserveCurrentLabels).
    WithMutation(BaseImagePullSecretMutation(owner.Spec.Version)).
    WithMutation(DisableAutomountMutation(owner.Spec.Version, owner.Spec.DisableAutomount)).
    Build()
```

When `DisableAutomount` is true, `.automountServiceAccountToken` is set to `false`. When the condition is not met, the
field is left at its baseline value. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `EnsureImagePullSecret` for idempotent secret registration.** Multiple features can independently ensure their
required pull secrets without conflicting with each other.

**Register mutations in dependency order.** If mutation B relies on a secret added by mutation A, register A first.
