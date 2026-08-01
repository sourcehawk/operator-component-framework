# Unstructured Primitives

The unstructured primitives are an escape hatch for managing arbitrary Kubernetes objects that have no Go type
definition at compile time: external CRDs, Crossplane resources, or any object known only at runtime.

## When to Use Unstructured

Choose between the three approaches in this order:

1. **Typed primitive** (`pkg/primitives/<kind>`): use this whenever a built-in primitive covers your kind. It has the
   most safety, the richest editor API, and the best domain defaults.
2. **Unstructured primitive** (this page): use this when the object's kind has no corresponding Go type or when you want
   to manage an external CRD without generating Go client code. You supply all lifecycle semantics through required
   handlers.
3. **Custom resource wrapper** (`pkg/generic`): use this when you own the Go type (your own CRD) or want a fully typed
   mutation surface with a custom builder API. See the [Custom Resource Implementation Guide](../custom-resource.md).

See also [Unstructured Primitives](../primitives.md#unstructured-primitives) in the Primitives Overview for a summary
table and [Implementing a Custom Resource](../primitives.md#implementing-a-custom-resource) for the full walkthrough.

## Variants

One variant exists per [lifecycle category](../primitives.md#primitive-categories), each implementing the corresponding
interfaces. Status values below are the runtime strings that appear in conditions (see
[Lifecycle Interfaces](../primitives.md#lifecycle-interfaces)).

| Package                                   | Category    | Lifecycle interfaces                                                     | Required at `Build()`         |
| ----------------------------------------- | ----------- | ------------------------------------------------------------------------ | ----------------------------- |
| `pkg/primitives/unstructured/static`      | Static      | `Guardable`, `DataExtractable`                                           | _(none)_                      |
| `pkg/primitives/unstructured/workload`    | Workload    | `Alive`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable`       | `WithCustomConvergeStatus`    |
| `pkg/primitives/unstructured/integration` | Integration | `Operational`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable` | `WithCustomOperationalStatus` |
| `pkg/primitives/unstructured/task`        | Task        | `Completable`, `Suspendable`, `Guardable`, `DataExtractable`             | `WithCustomConvergeStatus`    |

## No Semantic Defaults

Because the framework has no type information for unstructured objects, it infers no domain-specific status or
suspension behavior. Safe fallbacks are configured instead:

- Grace status defaults to `Healthy` when no handler is provided.
- Suspension status defaults to `Suspended` immediately (no-op suspend mutation, `DeleteOnSuspend` returns `false`).

Only the converge or operational status handler is required. All other handlers are optional. Calling `Build()` without
the required handler returns an error.

## Building Unstructured Primitives

### Static (simplest)

```go
import (
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"
    uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

obj := &uns.Unstructured{}
obj.SetGroupVersionKind(schema.GroupVersionKind{
    Group: "example.io", Version: "v1alpha1", Kind: "Widget",
})
obj.SetName("my-widget")
obj.SetNamespace(owner.Namespace)

resource, err := static.NewBuilder(obj).
    WithMutation(RegionMutation(owner.Spec.Version, owner.Spec.Region)).
    Build()
```

### Workload

```go
import (
    "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
    unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/workload"
    uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

resource, err := workload.NewBuilder(obj).
    WithCustomConvergeStatus(func(op concepts.ConvergingOperation, o *uns.Unstructured) (concepts.AliveStatusWithReason, error) {
        ready, _, _ := uns.NestedBool(o.Object, "status", "ready")
        if ready {
            return concepts.AliveStatusWithReason{
                Status: concepts.AliveConvergingStatusHealthy,
                Reason: "resource is ready",
            }, nil
        }
        return concepts.AliveStatusWithReason{
            Status: concepts.AliveConvergingStatusCreating,
            Reason: "waiting for readiness",
        }, nil
    }).
    Build()
```

### Integration

```go
import (
    "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/integration"
    uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

resource, err := integration.NewBuilder(obj).
    WithCustomOperationalStatus(func(op concepts.ConvergingOperation, o *uns.Unstructured) (concepts.OperationalStatusWithReason, error) {
        phase, _, _ := uns.NestedString(o.Object, "status", "phase")
        switch phase {
        case "Ready":
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
        case "Pending":
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusPending}, nil
        default:
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusFailing, Reason: phase}, nil
        }
    }).
    Build()
```

### Cluster-Scoped Resources

Call `MarkClusterScoped()` for resources without a namespace. The builder rejects a non-empty namespace and formats the
identity string without a namespace segment. See [Cluster-Scoped Primitives](../primitives.md#cluster-scoped-primitives)
for details.

```go
resource, err := static.NewBuilder(obj).
    MarkClusterScoped().
    Build()
```

## Mutations

All four variants share `unstruct.Mutation` and `*unstruct.Mutator` from the parent `pkg/primitives/unstructured`
package. Mutations follow the same pattern as typed primitives. For a full explanation of the mutation system,
boolean-gated mutations, and version-gated mutations see [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

```go
import (
    unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
)

func RegionMutation(version, region string) unstruct.Mutation {
    return unstruct.Mutation{
        Name:    "set-region",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *unstruct.Mutator) error {
            m.EditContent(func(e *editors.UnstructuredContentEditor) error {
                return e.SetNestedString(region, "spec", "forProvider", "region")
            })
            m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
                e.EnsureLabel("region", region)
                return nil
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits execute in a fixed category order regardless of the order they are recorded:

| Step | Category       | What it affects                               |
| ---- | -------------- | --------------------------------------------- |
| 1    | Metadata edits | Labels and annotations via `ObjectMetaEditor` |
| 2    | Content edits  | Nested fields via `UnstructuredContentEditor` |

Features apply in registration order. Later features observe the object as modified by all earlier ones.

## Relevant Editors

For the full method list of any editor see the
[Go API reference](https://pkg.go.dev/github.com/sourcehawk/operator-component-framework/pkg/mutation/editors). The
generic concept is explained in [Mutation Editors](../primitives.md#mutation-editors).

### UnstructuredContentEditor

The `UnstructuredContentEditor` wraps the object's `map[string]interface{}` content and provides structured operations
for setting and removing values at nested paths. Access it via `m.EditContent`.

| Method                       | Signature                                                | Purpose                                        |
| ---------------------------- | -------------------------------------------------------- | ---------------------------------------------- |
| `SetNestedField`             | `(value interface{}, fields ...string) error`            | Set any value at a nested path                 |
| `RemoveNestedField`          | `(fields ...string)`                                     | Remove a field at a nested path                |
| `SetNestedString`            | `(value string, fields ...string) error`                 | Convenience for string fields                  |
| `SetNestedBool`              | `(value bool, fields ...string) error`                   | Convenience for boolean fields                 |
| `SetNestedInt64`             | `(value int64, fields ...string) error`                  | Convenience for integer fields                 |
| `SetNestedFloat64`           | `(value float64, fields ...string) error`                | Convenience for float fields                   |
| `SetNestedStringMap`         | `(value map[string]string, fields ...string) error`      | Set a string map (labels, selectors)           |
| `EnsureNestedStringMapEntry` | `(key, value string, fields ...string) error`            | Add or update one entry in a nested string map |
| `RemoveNestedStringMapEntry` | `(key string, fields ...string) error`                   | Remove one entry from a nested string map      |
| `SetNestedSlice`             | `(value []interface{}, fields ...string) error`          | Set an entire slice                            |
| `SetNestedMap`               | `(value map[string]interface{}, fields ...string) error` | Set an entire sub-object                       |
| `Raw`                        | `() map[string]interface{}`                              | Escape hatch for free-form access              |

When the structured methods are insufficient, `Raw()` returns the underlying content map for direct manipulation:

```go
m.EditContent(func(e *editors.UnstructuredContentEditor) error {
    raw := e.Raw()
    spec, ok := raw["spec"].(map[string]interface{})
    if !ok {
        spec = map[string]interface{}{}
        raw["spec"] = spec
    }
    spec["customField"] = someComplexValue
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

!!! note "Metadata bridging"

    `*uns.Unstructured` does not embed `metav1.ObjectMeta`. During `Apply()`, the mutator populates a temporary
    `ObjectMeta` from the object's labels and annotations, runs the editor, and writes the results back via
    `SetLabels`/`SetAnnotations`. The behavior is identical to typed primitives from the caller's perspective.

## Identity

The identity string is derived from the object's GVK, namespace, and name at build time:

- Namespaced: `{group}/{version}/{kind}/{namespace}/{name}`
- Cluster-scoped: `{group}/{version}/{kind}/{name}`

Namespaced resources must have a non-empty namespace; `Build()` rejects empty namespaces unless `MarkClusterScoped()`
was called.

## Data Extraction

All four variants support data extraction. The extractor receives a value copy of the reconciled object after each sync
cycle:

```go
builder.WithDataExtractor(func(obj uns.Unstructured) error {
    ip, found, _ := uns.NestedString(obj.Object, "status", "atProvider", "ipAddress")
    if found {
        myComponent.ResourceIP = ip
    }
    return nil
})
```

## Suspension Handlers

The non-static variants support custom suspension behavior. All three handlers default to safe no-ops when omitted.

| Builder method                      | Default behavior                    |
| ----------------------------------- | ----------------------------------- |
| `WithCustomSuspendDeletionDecision` | Returns `false` (keep the resource) |
| `WithCustomSuspendMutation`         | No-op (no spec changes on suspend)  |
| `WithCustomSuspendStatus`           | Reports `Suspended` immediately     |

Override them when the resource has native suspend semantics or must be deleted on suspend:

```go
workload.NewBuilder(obj).
    WithCustomSuspendDeletionDecision(func(o *uns.Unstructured) bool {
        return true // delete on suspend
    }).
    WithCustomSuspendMutation(func(m *unstruct.Mutator) error {
        return nil // no-op; deletion handles everything
    }).
    WithCustomSuspendStatus(func(o *uns.Unstructured) (concepts.SuspensionStatusWithReason, error) {
        return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
    }).
    Build()
```

## Full Example

```go
// Manage an external CRD that provisions a database connection.
obj := &uns.Unstructured{}
obj.SetGroupVersionKind(schema.GroupVersionKind{
    Group: "db.example.io", Version: "v1", Kind: "Connection",
})
obj.SetName("app-db")
obj.SetNamespace(owner.Namespace)

resource, err := integration.NewBuilder(obj).
    WithMutation(unstruct.Mutation{
        Name:    "connection-config",
        Feature: feature.NewVersionGate(owner.Spec.Version, nil),
        Mutate: func(m *unstruct.Mutator) error {
            m.EditContent(func(e *editors.UnstructuredContentEditor) error {
                if err := e.SetNestedString(owner.Spec.Region, "spec", "region"); err != nil {
                    return err
                }
                return e.SetNestedInt64(int64(owner.Spec.PoolSize), "spec", "poolSize")
            })
            return nil
        },
    }).
    WithCustomOperationalStatus(func(_ concepts.ConvergingOperation, o *uns.Unstructured) (concepts.OperationalStatusWithReason, error) {
        phase, _, _ := uns.NestedString(o.Object, "status", "phase")
        switch phase {
        case "Ready":
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
        case "Provisioning":
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusPending, Reason: "provisioning"}, nil
        default:
            return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusFailing, Reason: phase}, nil
        }
    }).
    WithDataExtractor(func(o uns.Unstructured) error {
        endpoint, _, _ := uns.NestedString(o.Object, "status", "endpoint")
        myComponent.DBEndpoint = endpoint
        return nil
    }).
    Build()
```

## Guidance

**Choose the right variant.** Pick the variant that matches the object's runtime behavior. Use `workload` for
long-running objects with observable health, `integration` for objects whose readiness depends on an external
controller, `task` for objects that run to completion, and `static` for configuration-like objects.

**Handlers encode all lifecycle semantics.** The framework has no type information for unstructured objects. The
handlers you provide are the sole source of lifecycle semantics. Inspect `obj.Object` fields directly to determine
status.

**Prefer typed primitives when possible.** Unstructured primitives trade compile-time safety for runtime flexibility. If
a built-in typed primitive covers the kind, use it.

**Test handlers thoroughly.** Without domain-specific defaults as a safety net, handler correctness is entirely on the
operator author. Write table-driven tests covering all status transitions before deploying.

**Use typed primitives or custom resource wrappers for your own CRDs.** Unstructured primitives are intended for
third-party or generated resources where a Go type is unavailable. For your own CRDs, generate the Go type and use a
typed wrapper; see the [Custom Resource Implementation Guide](../custom-resource.md).
