# Unstructured Primitives

The `unstructured` primitives are an escape hatch for managing arbitrary Kubernetes objects that have no Go type
definition available at compile time — Crossplane resources, external CRDs, or any object known only at runtime.

Four variants are provided, one per [lifecycle category](../primitives.md#primitive-categories):

| Package                                   | Category    | Lifecycle Interfaces                                        |
| ----------------------------------------- | ----------- | ----------------------------------------------------------- |
| `pkg/primitives/unstructured/static`      | Static      | `DataExtractable`                                           |
| `pkg/primitives/unstructured/workload`    | Workload    | `Alive`, `Graceful`, `Suspendable`, `DataExtractable`       |
| `pkg/primitives/unstructured/integration` | Integration | `Operational`, `Graceful`, `Suspendable`, `DataExtractable` |
| `pkg/primitives/unstructured/task`        | Task        | `Completable`, `Suspendable`, `DataExtractable`             |

## No Semantic Defaults

Because the framework cannot know the semantics of an unstructured object, **no domain-specific status or suspension
behavior is inferred**. The unstructured builders only configure generic safe defaults: grace status defaults to
`Healthy`, suspension status to `Suspended`, and suspension mutations are no-ops. Only the converging or operational
status handler is required at build time; all other handlers are optional and fall back to these safe defaults when
omitted. Calling `Build()` without the required handler returns an error.

### Required Handlers per Variant

| Variant         | Required at `Build()` | Optional                                                 |
| --------------- | --------------------- | -------------------------------------------------------- |
| **Static**      | _(none)_              | —                                                        |
| **Workload**    | `ConvergingStatus`    | `GraceStatus` (defaults to Healthy), suspension handlers |
| **Integration** | `OperationalStatus`   | `GraceStatus` (defaults to Healthy), suspension handlers |
| **Task**        | `ConvergingStatus`    | Suspension handlers                                      |

Suspension handlers default to safe no-ops when omitted: `DeleteOnSuspend()` returns false, `Suspend()` is a no-op, and
`SuspensionStatus()` reports `Suspended` immediately. Override these via `WithCustomSuspendDeletionDecision`,
`WithCustomSuspendMutation`, and `WithCustomSuspendStatus` if the resource needs custom suspension behavior.

## Building an Unstructured Primitive

### Static (simplest)

```go
import (
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"
    uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

obj := &uns.Unstructured{}
obj.SetGroupVersionKind(schema.GroupVersionKind{
    Group: "example.crossplane.io", Version: "v1alpha1", Kind: "Database",
})
obj.SetName("my-database")
obj.SetNamespace(owner.Namespace)

resource, err := static.NewBuilder(obj).
    WithMutation(myMutation(owner.Spec.Version)).
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
        // Inspect o.Object to determine health
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
    WithCustomGraceStatus(func(o *uns.Unstructured) (concepts.GraceStatusWithReason, error) {
        return concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded}, nil
    }).
    WithCustomSuspendStatus(func(o *uns.Unstructured) (concepts.SuspensionStatusWithReason, error) {
        return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
    }).
    WithCustomSuspendMutation(func(m *unstruct.Mutator) error {
        return nil
    }).
    WithCustomSuspendDeletionDecision(func(o *uns.Unstructured) bool {
        return true // delete on suspend
    }).
    Build()
```

### Cluster-Scoped Resources

Call `MarkClusterScoped()` for resources without a namespace. The builder validates that the object's namespace is empty
and formats the identity string without a namespace segment.

```go
resource, err := static.NewBuilder(obj).
    MarkClusterScoped().
    Build()
```

## Mutations

Mutations follow the same pattern as typed primitives. The `Mutation` type is defined in the shared
`primitives/unstructured` package and used by all four variants:

```go
import (
    unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
)

func RegionMutation(version, region string) unstruct.Mutation {
    return unstruct.Mutation{
        Name:    "set-region",
        Feature: feature.NewResourceFeature(version, nil),
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

Within each feature, mutations execute in the following order:

1. **Metadata edits** — labels and annotations via `ObjectMetaEditor`
2. **Content edits** — nested fields via `UnstructuredContentEditor`

Features are applied in registration order. Later features observe the object as modified by all previous features.

## UnstructuredContentEditor

The `UnstructuredContentEditor` wraps the object's `map[string]interface{}` content and provides structured operations
for setting and removing values at nested paths.

### Methods

| Method                       | Signature                                                | Purpose                                     |
| ---------------------------- | -------------------------------------------------------- | ------------------------------------------- |
| `SetNestedField`             | `(value interface{}, fields ...string) error`            | Set any value at a nested path              |
| `RemoveNestedField`          | `(fields ...string)`                                     | Remove a field at a nested path             |
| `SetNestedString`            | `(value string, fields ...string) error`                 | Convenience for string fields               |
| `SetNestedBool`              | `(value bool, fields ...string) error`                   | Convenience for boolean fields              |
| `SetNestedInt64`             | `(value int64, fields ...string) error`                  | Convenience for integer fields              |
| `SetNestedFloat64`           | `(value float64, fields ...string) error`                | Convenience for float fields                |
| `SetNestedStringMap`         | `(value map[string]string, fields ...string) error`      | Set a string map (labels, selectors)        |
| `EnsureNestedStringMapEntry` | `(key, value string, fields ...string) error`            | Add/update one entry in a nested string map |
| `RemoveNestedStringMapEntry` | `(key string, fields ...string) error`                   | Remove one entry from a nested string map   |
| `SetNestedSlice`             | `(value []interface{}, fields ...string) error`          | Set an entire slice                         |
| `SetNestedMap`               | `(value map[string]interface{}, fields ...string) error` | Set an entire sub-object                    |
| `Raw`                        | `() map[string]interface{}`                              | Escape hatch for free-form access           |

### Raw Escape Hatch

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

## Identity

The identity string is derived from the object's GVK, namespace, and name:

- **Namespaced**: `{group}/{version}/{kind}/{namespace}/{name}`
- **Cluster-scoped**: `{group}/{version}/{kind}/{name}`

For namespaced resources with an empty namespace, `"default"` is used.

## Data Extraction

All four variants support data extraction. Extractors receive a value copy of the reconciled object:

```go
builder.WithDataExtractor(func(obj uns.Unstructured) error {
    ip, found, _ := uns.NestedString(obj.Object, "status", "atProvider", "ipAddress")
    if found {
        myComponent.DatabaseIP = ip
    }
    return nil
})
```

## Guidance

- **Choose the right variant.** Pick the variant matching the object's runtime behavior. If the object runs continuously
  and has observable health, use `workload`. If it depends on external assignments, use `integration`. If it runs to
  completion, use `task`. If it is configuration-like, use `static`.
- **Handlers encode your domain knowledge.** Since the framework has no type information for unstructured objects, the
  handlers you provide are the only source of lifecycle semantics. Inspect `obj.Object` fields to determine status.
- **Use typed primitives when possible.** Unstructured primitives trade compile-time safety for runtime flexibility.
  Prefer typed primitives for standard Kubernetes resources.
- **Test your handlers.** Without domain-specific defaults as a safety net, handler correctness is entirely on the
  operator author. Write table-driven tests covering all status transitions.
