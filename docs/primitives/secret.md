# Secret Primitive

The `secret` primitive is the framework's built-in static abstraction for managing Kubernetes `Secret` resources. It
integrates with the component lifecycle and provides a structured mutation API for managing `.data` and `.stringData`
entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                                   |
| --------------------- | -------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state           |
| **Mutation pipeline** | Typed editors for `.data` and `.stringData` entries and object metadata, with a raw escape hatch         |
| **Flavors**           | Preserves externally-managed fields — labels, annotations, and `.data` entries not owned by the operator |
| **Data extraction**   | Reads generated or updated values back from the reconciled Secret after each sync cycle                  |

## Building a Secret Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"

base := &corev1.Secret{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-credentials",
        Namespace: owner.Namespace,
    },
    Data: map[string][]byte{
        "password": []byte("default-password"),
    },
}

resource, err := secret.NewBuilder(base).
    WithFieldApplicationFlavor(secret.PreserveExternalEntries).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current Secret with a deep copy of the desired object, then restores
server-managed metadata (ResourceVersion, UID, etc.) and shared-controller fields (OwnerReferences, Finalizers) from the
original live object. This ensures every reconciliation cycle produces a clean, predictable state without losing
server-managed data.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := secret.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *corev1.Secret) error {
        // Only synchronise owned keys; leave other fields untouched.
        if current.Data == nil {
            current.Data = make(map[string][]byte)
        }
        current.Data["owned-key"] = desired.Data["owned-key"]
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `Secret` beyond its baseline. Each mutation is a named function that
receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) secret.Mutation {
    return secret.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *secret.Mutator) error {
            m.SetData("feature-flag", []byte("enabled"))
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

```go
func TLSSecretMutation(version string, tlsEnabled bool) secret.Mutation {
    return secret.Mutation{
        Name:    "tls-secret",
        Feature: feature.NewResourceFeature(version, nil).When(tlsEnabled),
        Mutate: func(m *secret.Mutator) error {
            m.SetData("tls.crt", certBytes)
            m.SetData("tls.key", keyBytes)
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyTokenMutation(version string) secret.Mutation {
    return secret.Mutation{
        Name: "legacy-token",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *secret.Mutator) error {
            m.SetStringData("auth-mode", "legacy-token")
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are
recorded:

| Step | Category       | What it affects                                      |
| ---- | -------------- | ---------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `Secret`               |
| 2    | Data edits     | `.data` and `.stringData` entries — Set, Remove, Raw |

Within each category, edits are applied in their registration order. Later edits in the same mutation observe the Secret
as modified by all earlier edits.

## Editors

### SecretDataEditor

The primary API for modifying `.data` and `.stringData` entries. Use `m.EditData` for full control:

```go
m.EditData(func(e *editors.SecretDataEditor) error {
    e.Set("password", []byte("new-password"))
    e.Remove("stale-key")
    e.SetString("config-value", "plaintext")
    return nil
})
```

#### Set and Remove (.data)

`Set` adds or overwrites a `.data` key with a byte slice value. `Remove` deletes a `.data` key; it is a no-op if the key
is absent.

```go
m.EditData(func(e *editors.SecretDataEditor) error {
    e.Set("api-key", []byte("secret-value"))
    e.Remove("deprecated-key")
    return nil
})
```

#### SetString and RemoveString (.stringData)

`SetString` adds or overwrites a `.stringData` key with a plaintext value. The API server merges `.stringData` into
`.data` on write. `RemoveString` deletes a `.stringData` key; it is a no-op if the key is absent.

```go
m.EditData(func(e *editors.SecretDataEditor) error {
    e.SetString("username", "admin")
    e.RemoveString("old-username")
    return nil
})
```

#### Raw Escape Hatches

`Raw()` returns the underlying `map[string][]byte` for `.data`. `RawStringData()` returns the underlying
`map[string]string` for `.stringData`. Both give direct access for free-form editing when none of the structured methods
are sufficient:

```go
m.EditData(func(e *editors.SecretDataEditor) error {
    raw := e.Raw()
    for k, v := range externalDefaults {
        if _, exists := raw[k]; !exists {
            raw[k] = v
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
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("checksum/secret", secretHash)
    return nil
})
```

## Convenience Methods

The `Mutator` exposes convenience wrappers for the most common `.data` and `.stringData` operations:

| Method                      | Equivalent to                          |
| --------------------------- | -------------------------------------- |
| `SetData(key, value)`       | `EditData` → `e.Set(key, value)`       |
| `RemoveData(key)`           | `EditData` → `e.Remove(key)`           |
| `SetStringData(key, value)` | `EditData` → `e.SetString(key, value)` |
| `RemoveStringData(key)`     | `EditData` → `e.RemoveString(key)`     |

Use these for simple, single-operation mutations. Use `EditData` when you need multiple operations or raw access in a
single edit block.

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external
controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := secret.NewBuilder(base).
    WithFieldApplicationFlavor(secret.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on
overlap.

```go
resource, err := secret.NewBuilder(base).
    WithFieldApplicationFlavor(secret.PreserveCurrentAnnotations).
    Build()
```

### PreserveExternalEntries

Preserves `.data` keys present on the live object but absent from the applied desired state. Applied values win on
overlap.

Use this when other controllers or admission webhooks inject entries into the Secret that your operator does not own:

```go
resource, err := secret.NewBuilder(base).
    WithFieldApplicationFlavor(secret.PreserveExternalEntries).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Data Hash

Two utilities are provided for computing a stable SHA-256 hash of a Secret's effective data content (`.data` plus
`.stringData` merged using Kubernetes API-server semantics). A common use is to annotate a Deployment's pod template
with this hash so that a secret change triggers a rolling restart.

### DataHash

`DataHash` hashes a Secret value you already have — for example, one read from the cluster:

```go
hash, err := secret.DataHash(s)
```

The hash is derived from the canonical JSON encoding of the effective data map with keys sorted alphabetically, so it is
deterministic regardless of insertion order. Both `.data` and `.stringData` are included: `.stringData` entries are
merged into a copy of `.data` (with `.stringData` keys taking precedence) before hashing, matching Kubernetes API-server
write semantics. This ensures the hash is consistent whether called on a desired object (which may use `.stringData`) or
a cluster-read object (where `.stringData` has already been merged into `.data`).

### Resource.DesiredHash

`DesiredHash` computes the hash of what the operator _will write_ — that is, the base object with all registered
mutations applied — without performing a cluster read and without a second reconcile cycle:

```go
secretResource, err := secret.NewBuilder(base).
    WithMutation(BaseSecretMutation(owner.Spec.Version)).
    WithMutation(TLSMutation(owner.Spec.EnableTLS)).
    Build()

hash, err := secretResource.DesiredHash()
```

The hash covers only operator-controlled fields. Entries preserved by flavors from the live cluster (e.g.
`PreserveExternalEntries`) are excluded — only changes to operator-owned content will change the hash.

### Annotating a Deployment pod template (single-pass pattern)

Build the secret resource first, compute the hash, then pass it into the deployment resource factory. Both resources are
registered with the same component, so the secret is reconciled first and the deployment sees the correct hash on every
cycle.

`DesiredHash` is defined on `*secret.Resource`, not on the `component.Resource` interface, so keep the concrete type
when you need to call it:

```go
secretResource, err := secret.NewBuilder(base).
    WithMutation(features.BaseSecretMutation(owner.Spec.Version)).
    WithMutation(features.TLSMutation(owner.Spec.Version, owner.Spec.EnableTLS)).
    Build()
if err != nil {
    return err
}

hash, err := secretResource.DesiredHash()
if err != nil {
    return err
}

deployResource, err := resources.NewDeploymentResource(owner, hash)
if err != nil {
    return err
}

comp, err := component.NewComponentBuilder().
    WithResource(secretResource, component.ResourceOptions{}).  // reconciled first
    WithResource(deployResource, component.ResourceOptions{}).
    Build()
```

```go
// In NewDeploymentResource, use the hash in a mutation:
func ChecksumAnnotationMutation(version, secretHash string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "secret-checksum",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *deployment.Mutator) error {
            m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureAnnotation("checksum/secret", secretHash)
                return nil
            })
            return nil
        },
    }
}
```

When the secret mutations change (version upgrade, feature toggle), `DesiredHash` returns a different value on the same
reconcile cycle, the pod template annotation changes, and Kubernetes triggers a rolling restart.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `PreserveExternalEntries` when sharing a Secret.** If admission webhooks, external controllers, or manual
operations add entries to a Secret your operator manages, this flavor prevents your operator from silently deleting
those entries each reconcile cycle.

**Register mutations in dependency order.** If mutation B relies on an entry set by mutation A, register A first.

**Prefer `.stringData` for human-readable values.** The API server handles base64 encoding; using `SetStringData` avoids
manual encoding in mutation code.
