# Secret Primitive

The `secret` primitive wraps a Kubernetes `Secret` and integrates with the component lifecycle as a Static resource,
providing a structured mutation API for managing `.data` and `.stringData` entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                               |
| --------------------- | ---------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension. The resource is reconciled to desired state        |
| **Mutation pipeline** | Typed editors for `.data` and `.stringData` entries and object metadata, with a `Raw()` escape hatch |
| **DataExtractable**   | Reads values back from the reconciled Secret after each sync cycle                                   |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports.

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
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example using the `SetData` convenience method:

```go
func MyFeatureMutation(version string) secret.Mutation {
    return secret.Mutation{
        Name:    "my-feature",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *secret.Mutator) error {
            m.SetData("feature-flag", []byte("enabled"))
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category       | What it affects                                     |
| ---- | -------------- | --------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `Secret`              |
| 2    | Data edits     | `.data` and `.stringData` entries: Set, Remove, Raw |

Within each category, edits run in registration order. Later features observe the Secret as modified by all earlier
ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

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
`map[string]string` for `.stringData`. Both give direct access for free-form editing:

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

## Data Hash

Two utilities compute a stable SHA-256 hash of a Secret's effective data content (`.data` plus `.stringData` merged
using Kubernetes API-server semantics). A common use is to annotate a Deployment's pod template with this hash so that a
secret change triggers a rolling restart.

### DataHash

`DataHash` hashes a Secret value you already have, for example one read from the cluster:

```go
hash, err := secret.DataHash(s)
```

The hash is derived from the canonical JSON encoding of the effective data map with keys sorted alphabetically.
`.stringData` entries are merged into a copy of `.data` (with `.stringData` keys taking precedence) before hashing,
matching Kubernetes API-server write semantics. This ensures the hash is consistent whether called on a desired object
or a cluster-read object.

### Resource.DesiredHash

`DesiredHash` computes the hash of what the operator _will write_ (the base object with all registered mutations
applied) without performing a cluster read and without a second reconcile cycle:

```go
secretResource, err := secret.NewBuilder(base).
    WithMutation(BaseSecretMutation(owner.Spec.Version)).
    WithMutation(TLSMutation(owner.Spec.EnableTLS)).
    Build()

hash, err := secretResource.DesiredHash()
```

The hash covers only operator-controlled fields.

### Annotating a Deployment pod template (single-pass pattern)

Build the Secret resource first, compute the hash, then pass it into the Deployment resource factory. Both resources are
registered with the same component, so the Secret is reconciled first and the Deployment sees the correct hash on every
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
    WithResource(secretResource). // reconciled first
    WithResource(deployResource).
    Build()
```

```go
// In NewDeploymentResource, use the hash in a mutation:
func ChecksumAnnotationMutation(version, secretHash string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "secret-checksum",
        Feature: feature.NewVersionGate(version, nil),
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

When the Secret mutations change (version upgrade, feature toggle), `DesiredHash` returns a different value on the same
reconcile cycle, the pod template annotation changes, and Kubernetes triggers a rolling restart.

## Data Extraction

`secret.ExtractInto` declares that this Secret produces the value of a data cell, which is how a generated credential
reaches the resources that need it. The function receives a value copy of the reconciled Secret after each sync cycle:

```go
password := concepts.NewData[string]("app-password")

builder := secret.NewBuilder(base)
secret.ExtractInto(builder, password, func(s corev1.Secret) (string, error) {
    return string(s.Data["password"]), nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(password)` or read it
opportunistically with `WithOptionalData(password)`. See [Declared Data](../component.md#declared-data).

A cell extracted from a Secret holds the decoded plaintext for the rest of the reconcile. Use it to build the object you
are applying, and keep it out of conditions, events, and log lines.

## Full Example

```go
func BaseSecretMutation(version string) secret.Mutation {
    return secret.Mutation{
        Name:    "base-secret",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *secret.Mutator) error {
            m.SetStringData("auth-mode", "token")
            return nil
        },
    }
}

var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyTokenMutation(version string) secret.Mutation {
    return secret.Mutation{
        Name: "legacy-token",
        Feature: feature.NewVersionGate(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *secret.Mutator) error {
            m.SetStringData("auth-mode", "legacy-token")
            return nil
        },
    }
}

func TLSSecretMutation(version string, tlsEnabled bool) secret.Mutation {
    return secret.Mutation{
        Name:    "tls-secret",
        Feature: feature.NewVersionGate(version, nil).When(tlsEnabled),
        Mutate: func(m *secret.Mutator) error {
            m.SetData("tls.crt", certBytes)
            m.SetData("tls.key", keyBytes)
            return nil
        },
    }
}

resource, err := secret.NewBuilder(base).
    WithMutation(BaseSecretMutation(owner.Spec.Version)).
    WithMutation(LegacyTokenMutation(owner.Spec.Version)).
    WithMutation(TLSSecretMutation(owner.Spec.Version, owner.Spec.EnableTLS)).
    Build()
```

On versions below 2.0.0 the `auth-mode` key is overwritten to `legacy-token` by the version-gated mutation. On 2.0.0 and
above only the base value is written. When TLS is enabled, the certificate bytes are added regardless of version.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for boolean conditions.

**Register mutations in dependency order.** If mutation B relies on an entry set by mutation A, register A first.

**Prefer `.stringData` for human-readable values.** The API server handles base64 encoding; using `SetStringData` avoids
manual encoding in mutation code.

**Use `DesiredHash` for rolling restarts triggered by secret rotation.** Build the Secret resource, call
`DesiredHash()`, and stamp the result as a pod-template annotation on the Deployment in the same reconcile pass.
