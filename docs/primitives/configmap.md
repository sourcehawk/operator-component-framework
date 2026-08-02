# ConfigMap Primitive

The `configmap` primitive wraps a Kubernetes `ConfigMap` and integrates with the component lifecycle as a Static
resource, providing a structured mutation API for managing `.data` entries and object metadata.

## Capabilities

| Capability            | Detail                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------ |
| **Static lifecycle**  | No health tracking, grace periods, or suspension. The resource is reconciled to desired state    |
| **Mutation pipeline** | Typed editors for `.data` entries and object metadata, with a `Raw()` escape hatch               |
| **MergeYAML**         | Deep-merges YAML patches into individual `.data` entries; composable across independent features |
| **DataExtractable**   | Reads values back from the reconciled ConfigMap after each sync cycle                            |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports.

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
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example using the `SetEntry` convenience method:

```go
func MyFeatureMutation(version string) configmap.Mutation {
    return configmap.Mutation{
        Name:    "my-feature",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *configmap.Mutator) error {
            m.SetEntry("feature-flag", "enabled")
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category       | What it affects                              |
| ---- | -------------- | -------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `ConfigMap`    |
| 2    | Data edits     | `.data` entries: Set, Remove, MergeYAML, Raw |

Within each category, edits run in registration order. Later features observe the ConfigMap as modified by all earlier
ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

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

`SetBinary` sets a raw byte slice in `.binaryData`. `RemoveBinary` deletes a `.binaryData` key; it is a no-op if the key
is absent. Format and encode the value before passing it in.

```go
m.EditData(func(e *editors.ConfigMapDataEditor) error {
    e.SetBinary("cert.pem", certBytes)
    e.RemoveBinary("old-cert.pem")
    return nil
})
```

#### MergeYAML

`MergeYAML` deep-merges a YAML patch string into the existing value at a key in `.data`. Merge semantics:

- If both the existing value and the patch are YAML mappings, their keys are merged recursively. Keys present only in
  the base are preserved, keys present only in the patch are added, and keys present in both are resolved by applying
  `MergeYAML` recursively.
- For all other types (scalars, sequences, mixed), the patch value wins.
- If the key does not yet exist, the patch is written as-is.

This makes it suitable for composing contributions from independent features without each needing to know about the
others:

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

`Raw()` returns the underlying `map[string]string` for `.data`. `RawBinary()` returns the underlying `map[string][]byte`
for `.binaryData`. Both give direct access for free-form editing:

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

| Method                  | Equivalent to                          |
| ----------------------- | -------------------------------------- |
| `SetEntry(key, value)`  | `EditData` → `e.Set(key, value)`       |
| `RemoveEntry(key)`      | `EditData` → `e.Remove(key)`           |
| `MergeYAML(key, patch)` | `EditData` → `e.MergeYAML(key, patch)` |

Use these for simple, single-operation mutations. Use `EditData` when you need multiple operations or raw access in a
single edit block.

## Data Hash

Two utilities compute a stable SHA-256 hash of a ConfigMap's `.data` and `.binaryData` fields. A common use is to
annotate a Deployment's pod template with this hash so that a configuration change triggers a rolling restart.

### DataHash

`DataHash` hashes a ConfigMap value you already have, for example one read from the cluster:

```go
hash, err := configmap.DataHash(cm)
```

The hash is derived from the canonical JSON encoding of `.data` and `.binaryData` with map keys sorted alphabetically,
so it is deterministic regardless of insertion order. Metadata fields are excluded.

### Resource.DesiredHash

`DesiredHash` computes the hash of what the operator _will write_ (the base object with all registered mutations
applied) without performing a cluster read and without a second reconcile cycle:

```go
cmResource, err := configmap.NewBuilder(base).
    WithMutation(BaseConfigMutation(owner.Spec.Version)).
    WithMutation(TracingMutation(owner.Spec.EnableTracing)).
    Build()

hash, err := cmResource.DesiredHash()
```

The hash covers only operator-controlled fields.

### Annotating a Deployment pod template (single-pass pattern)

Build the ConfigMap resource first, compute the hash, then pass it into the Deployment resource factory. Both resources
are registered with the same component, so the ConfigMap is reconciled first and the Deployment sees the correct hash on
every cycle.

`DesiredHash` is defined on `*configmap.Resource`, not on the `component.Resource` interface, so keep the concrete type
when you need to call it:

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
    WithResource(cmResource). // reconciled first
    WithResource(deployResource).
    Build()
```

```go
// In NewDeploymentResource, use the hash in a mutation:
func ChecksumAnnotationMutation(version, configHash string) deployment.Mutation {
    return deployment.Mutation{
        Name:    "config-checksum",
        Feature: feature.NewVersionGate(version, nil),
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

When the ConfigMap mutations change (version upgrade, feature toggle), `DesiredHash` returns a different value on the
same reconcile cycle, the pod template annotation changes, and Kubernetes triggers a rolling restart.

## Data Extraction

`configmap.ExtractInto` declares that this ConfigMap produces the value of a data cell, which is how a rendered
configuration value reaches the resources that consume it. The function receives a value copy of the reconciled
ConfigMap after each sync cycle:

```go
bootstrapServers := concepts.NewData[string]("bootstrap-servers")

builder := configmap.NewBuilder(base)
configmap.ExtractInto(builder, bootstrapServers, func(cm corev1.ConfigMap) (string, error) {
    return cm.Data["bootstrap-servers"], nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(bootstrapServers)` or read it
opportunistically with `WithOptionalData(bootstrapServers)`. See [Declared Data](../component.md#declared-data).

## Full Example

```go
func BaseConfigMutation(version string) configmap.Mutation {
    return configmap.Mutation{
        Name:    "base-config",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *configmap.Mutator) error {
            m.EditData(func(e *editors.ConfigMapDataEditor) error {
                return e.MergeYAML("app.yaml", `
server:
  port: 8080
  timeout: 30s
`)
            })
            return nil
        },
    }
}

func MetricsFeatureMutation(version string, enabled bool) configmap.Mutation {
    return configmap.Mutation{
        Name:    "metrics-feature",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *configmap.Mutator) error {
            m.EditData(func(e *editors.ConfigMapDataEditor) error {
                return e.MergeYAML("app.yaml", `
metrics:
  enabled: true
  port: 9090
`)
            })
            return nil
        },
    }
}

resource, err := configmap.NewBuilder(base).
    WithMutation(BaseConfigMutation(owner.Spec.Version)).
    WithMutation(MetricsFeatureMutation(owner.Spec.Version, owner.Spec.MetricsEnabled)).
    Build()
```

When `MetricsEnabled` is true, the final `app.yaml` entry contains the merged result of both patches. When false, only
the base config is written. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for boolean conditions.

**Use `MergeYAML` for composable config files.** When multiple features contribute to the same YAML entry, `MergeYAML`
lets each contribute its section independently. Using `SetEntry` in multiple features for the same key means the last
registration wins; only use that when replacement is the intended semantics.

**Register mutations in dependency order.** If mutation B relies on an entry set by mutation A, register A first.

**Use `DesiredHash` for rolling restarts.** Build the ConfigMap resource, call `DesiredHash()`, and stamp the result as
a pod-template annotation on the Deployment in the same reconcile pass. No extra cluster reads are required.
