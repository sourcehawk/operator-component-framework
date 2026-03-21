# ConfigMap Primitive

The `configmap` primitive is the framework's built-in static abstraction for managing Kubernetes `ConfigMap` resources. It integrates with the component lifecycle and provides a structured mutation API for managing `.data` entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                                   |
|-----------------------|----------------------------------------------------------------------------------------------------------|
| **Static lifecycle**  | No health tracking, grace periods, or suspension — the resource is reconciled to desired state           |
| **Mutation pipeline** | Typed editors for `.data` entries and object metadata, with a raw escape hatch for free-form access      |
| **MergeYAML**         | Deep-merges YAML patches into individual `.data` entries; composable across independent features         |
| **Flavors**           | Preserves externally-managed fields — labels, annotations, and `.data` entries not owned by the operator |
| **Data extraction**   | Reads generated or updated values back from the reconciled ConfigMap after each sync cycle               |

## Building a ConfigMap Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"

base := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-config",
        Namespace: owner.Namespace,
    },
    Data: map[string]string{
        "config.yaml": "log_level: info\n",
    },
}

resource, err := configmap.NewBuilder(base).
    WithFieldApplicationFlavor(configmap.PreserveExternalEntries).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current ConfigMap with a deep copy of the desired object. This ensures every reconciliation cycle produces a clean, predictable state and avoids any drift from the desired baseline.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := configmap.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *corev1.ConfigMap) error {
        // Only synchronise owned keys; leave other fields untouched.
        current.Data["owned-key"] = desired.Data["owned-key"]
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `ConfigMap` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

A mutation requires a `Feature` to control when it applies. A feature with no version constraints and no `When()` conditions is always enabled:

```go
func MyFeatureMutation(version string) configmap.Mutation {
    return configmap.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *configmap.Mutator) error {
            m.SetEntry("feature-flag", "enabled")
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

```go
func TLSConfigMutation(version string, tlsEnabled bool) configmap.Mutation {
    return configmap.Mutation{
        Name:    "tls-config",
        Feature: feature.NewResourceFeature(version, nil).When(tlsEnabled),
        Mutate: func(m *configmap.Mutator) error {
            m.SetEntry("tls_mode", "strict")
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyAuthMutation(version string) configmap.Mutation {
    return configmap.Mutation{
        Name: "legacy-auth",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *configmap.Mutator) error {
            m.SetEntry("auth_mode", "legacy-token")
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are recorded:

| Step | Category          | What it affects                               |
|------|-------------------|-----------------------------------------------|
| 1    | Metadata edits    | Labels and annotations on the `ConfigMap`     |
| 2    | Data edits        | `.data` entries — Set, Remove, MergeYAML, Raw |

Within each category, edits are applied in their registration order. Later features observe the ConfigMap as modified by all previous features.

## Editors

### ConfigMapDataEditor

The primary API for modifying `.data` and `.binaryData` entries. Use `m.EditData` for full control:

```go
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    e.Set("key", "value")
    e.Remove("stale-key")
    return e.MergeYAML("config.yaml", "debug: true\n")
})
```

#### Set and Remove

`Set` adds or overwrites a `.data` key. `Remove` deletes a `.data` key; it is a no-op if the key is absent.

```go
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    e.Set("mode", "production")
    e.Remove("dev-only-flag")
    return nil
})
```

#### SetBinary and RemoveBinary

`SetBinary` sets a raw byte slice in `.binaryData`. `RemoveBinary` deletes a `.binaryData` key; it is a no-op if the key is absent. No helpers are provided beyond set and remove — format and encode the value before passing it in.

```go
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    e.SetBinary("cert.pem", certBytes)
    e.RemoveBinary("old-cert.pem")
    return nil
})
```

#### MergeYAML

`MergeYAML` deep-merges a YAML patch string into the existing value at a key in `.data`. Merge semantics:

- If both the existing value and the patch are YAML mappings, their keys are merged recursively — keys present only in the base are preserved, keys present only in the patch are added, and keys present in both are resolved by applying `MergeYAML` recursively.
- For all other types (scalars, sequences, mixed), the patch value wins.
- If the key does not yet exist, the patch is written as-is.

This makes it suitable for composing contributions from independent features without each feature needing to know about the others:

```go
// Feature A contributes logging config.
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    return e.MergeYAML("app.yaml", "logging:\n  level: info\n")
})

// Feature B independently contributes tracing config into the same file.
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    return e.MergeYAML("app.yaml", "tracing:\n  enabled: true\n")
})
// Result: app.yaml contains both logging and tracing sections.
```

#### Raw Escape Hatches

`Raw()` returns the underlying `map[string]string` for `.data`. `RawBinary()` returns the underlying `map[string][]byte` for `.binaryData`. Both give direct access for free-form editing when none of the structured methods are sufficient:

```go
m.EditData(func(e *editors.ConfigMapDataEditor) error {
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
    e.EnsureAnnotation("checksum/config", configHash)
    return nil
})
```

## Convenience Methods

The `Mutator` exposes convenience wrappers for the most common `.data` operations:

| Method                      | Equivalent to                                   |
|-----------------------------|-------------------------------------------------|
| `SetEntry(key, value)`      | `EditData` → `e.Set(key, value)`                |
| `RemoveEntry(key)`          | `EditData` → `e.Remove(key)`                    |
| `MergeYAML(key, patch)`     | `EditData` → `e.MergeYAML(key, patch)`          |

Use these for simple, single-operation mutations. Use `EditData` when you need multiple operations or raw access in a single edit block.

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := configmap.NewBuilder(base).
    WithFieldApplicationFlavor(configmap.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on overlap.

```go
resource, err := configmap.NewBuilder(base).
    WithFieldApplicationFlavor(configmap.PreserveCurrentAnnotations).
    Build()
```

### PreserveExternalEntries

Preserves `.data` keys present on the live object but absent from the applied desired state. Applied values win on overlap.

Use this when other controllers or admission webhooks inject entries into the ConfigMap that your operator does not own:

```go
resource, err := configmap.NewBuilder(base).
    WithFieldApplicationFlavor(configmap.PreserveExternalEntries).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Data Hash

Two utilities are provided for computing a stable SHA-256 hash of a ConfigMap's `.data` and `.binaryData` fields. A common use is to annotate a Deployment's pod template with this hash so that a configuration change triggers a rolling restart.

### DataHash

`DataHash` hashes a ConfigMap value you already have — for example, one read from the cluster:

```go
hash, err := configmap.DataHash(cm)
```

The hash is derived from the canonical JSON encoding of `.data` and `.binaryData` with map keys sorted alphabetically, so it is deterministic regardless of insertion order. Metadata fields (labels, annotations, etc.) are excluded.

### Resource.DesiredHash

`DesiredHash` computes the hash of what the operator *will write* — that is, the base object with all registered mutations applied — without performing a cluster read and without a second reconcile cycle:

```go
cmResource, err := configmap.NewBuilder(base).
    WithMutation(BaseConfigMutation(owner.Spec.Version)).
    WithMutation(TracingMutation(owner.Spec.EnableTracing)).
    Build()

hash, err := cmResource.DesiredHash()
```

The hash covers only operator-controlled fields. Entries preserved by flavors from the live cluster (e.g. `PreserveExternalEntries`) are excluded — only changes to operator-owned content will change the hash.

### Annotating a Deployment pod template (single-pass pattern)

Build the configmap resource first, compute the hash, then pass it into the deployment resource factory. Both resources are registered with the same component, so the configmap is reconciled first and the deployment sees the correct hash on every cycle.

`DesiredHash` is defined on `*configmap.Resource`, not on the `component.Resource` interface, so keep the concrete type when you need to call it:

```go
cmResource, err := configmap.NewBuilder(base).
    WithMutation(features.BaseConfigMutation(owner.Spec.Version)).
    WithMutation(features.TracingMutation(owner.Spec.Version, owner.Spec.EnableTracing)).
    Build()
if err != nil {
    return err
}

hash, err := cmResource.DesiredHash()
if err != nil {
    return err
}

deployResource, err := resources.NewDeploymentResource(owner, hash)
if err != nil {
    return err
}

comp, err := component.NewComponentBuilder().
    WithResource(cmResource, component.ResourceOptions{}).  // reconciled first
    WithResource(deployResource, component.ResourceOptions{}).
    Build()
```

```go
// In NewDeploymentResource, use the hash in a mutation:
func ChecksumAnnotationMutation(version, configHash string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "config-checksum",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *deployment.Mutator) error {
            m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureAnnotation("checksum/config", configHash)
                return nil
            })
            return nil
        },
    }
}
```

When the configmap mutations change (version upgrade, feature toggle), `DesiredHash` returns a different value on the same reconcile cycle, the pod template annotation changes, and Kubernetes triggers a rolling restart.

## Full Example: Feature-Composed Configuration

```go
func BaseConfigMutation(version string) configmap.Mutation {
    return configmap.Mutation{
        Name:    "base-config",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *configmap.Mutator) error {
            return m.EditData(func(e *editors.ConfigMapDataEditor) error {
                return e.MergeYAML("app.yaml", `
server:
  port: 8080
  timeout: 30s
`)
            })
        },
    }
}

func MetricsFeatureMutation(version string, enabled bool) configmap.Mutation {
    return configmap.Mutation{
        Name:    "metrics-feature",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *configmap.Mutator) error {
            return m.EditData(func(e *editors.ConfigMapDataEditor) error {
                return e.MergeYAML("app.yaml", `
metrics:
  enabled: true
  port: 9090
`)
            })
        },
    }
}

resource, err := configmap.NewBuilder(base).
    WithFieldApplicationFlavor(configmap.PreserveExternalEntries).
    WithMutation(BaseConfigMutation(owner.Spec.Version)).
    WithMutation(MetricsFeatureMutation(owner.Spec.Version, owner.Spec.MetricsEnabled)).
    Build()
```

When `MetricsEnabled` is true, the final `app.yaml` entry will contain the merged result of both patches. When false, only the base config is written. Neither mutation needs to know about the other.

## Guidance

**Every mutation requires a `Feature`.** A `Mutation` with `Feature: nil` is never applied. Use `feature.NewResourceFeature(version, nil)` for an unconditional mutation, and chain `.When(bool)` for boolean gating.

**Use `MergeYAML` for composable config files.** When multiple features need to contribute to the same YAML entry, `MergeYAML` lets each feature contribute its section independently. Using `SetEntry` in multiple features for the same key means the last registration wins — only use that when replacement is the intended semantics.

**Use `PreserveExternalEntries` when sharing a ConfigMap.** If admission webhooks, external controllers, or manual operations add entries to a ConfigMap your operator manages, this flavor prevents your operator from silently deleting those entries each reconcile cycle.

**Register mutations in dependency order.** If mutation B relies on an entry set by mutation A, register A first.