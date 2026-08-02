---
name: custom-resource-wrappers
description:
  Use when wrapping a custom resource (a CRD-backed type not covered by the built-in primitives) as an
  operator-component-framework primitive using pkg/generic - covers choosing a resource category, mutation type aliases,
  implementing the mutator, status handlers, the builder, the resource type, feature mutations, and component
  registration.
---

# Custom Resource Wrappers

## When to write a wrapper

The built-in primitives cover the common Kubernetes kinds (Deployments, StatefulSets, ConfigMaps, Services, and more).
Reach for a custom resource wrapper only when the kind an operator manages has no matching primitive: a custom CRD
defined by the project or a third-party operator, or a standard Kubernetes kind the built-in set does not yet wrap.

`pkg/generic` supplies the building blocks (reconciliation mechanics, the plan-and-apply mutation flow, suspension,
guards, declared data). A wrapper package combines these with kind-specific identity, status, and mutator logic, the
same way the built-in primitives do.

If the CRD has no typed Go struct, the unstructured static primitive (`pkg/primitives/unstructured/static`) manages it
without writing a wrapper at all. That is the lightweight alternative: it is sufficient when a typed, self-documenting
API is not needed for a kind the operator touches only occasionally. Write a full wrapper when a typed struct exists and
the kind is managed often enough to justify a dedicated package. Other unstructured variants exist per resource
category; see the using-primitives skill for the full set.

## Generate the package first

The framework ships a CLI that generates this whole pattern:

```bash
go install github.com/sourcehawk/operator-component-framework/cmd/ocf@latest
ocf scaffold wrapper --type <import-path>.<TypeName> --variant <static|workload|task|integration> --group <api-group>
```

It writes `mutator.go`, `builder.go`, `resource.go`, and `builder_test.go` into `./<package>`, wired to the framework
version the CLI was built from, with working default status handlers marked as scaffolded defaults to replace. Prefer it
over writing the files by hand: the boilerplate below is what it produces, so the remaining work is replacing those
defaults with kind-specific logic.

Run `go mod tidy` afterwards if the module does not already depend on the wrapped type's API package, since the CLI
never edits `go.mod`. The steps below stay the reference for what the generated code means, and for the cases the CLI
does not cover: an existing wrapper being extended, or a kind whose scaffold has already been customized.

A custom resource is three wrapped pieces: the builder configures and validates, producing a resource; the resource
delegates lifecycle methods to a generic base; the mutator records and applies changes to the Kubernetes object.

| Your type  | Wraps                                                         |
| ---------- | ------------------------------------------------------------- |
| `Builder`  | `generic.WorkloadBuilder[T, *Mutator]` (or one per category)  |
| `Resource` | `generic.WorkloadResource[T, *Mutator]` (or one per category) |
| `Mutator`  | Implements `generic.FeatureMutator`                           |

## The eight steps

### 1. Choose a resource category

The framework defines four categories, each mapping to a generic resource type with a different set of lifecycle
interfaces:

| Category        | Generic type                  | Lifecycle interfaces                                                     | Use when                                               |
| --------------- | ----------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------ |
| **Workload**    | `generic.WorkloadResource`    | `Alive`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable`       | Long-running processes with replica-based health       |
| **Static**      | `generic.StaticResource`      | `Guardable`, `DataExtractable`                                           | Configuration objects with no runtime health semantics |
| **Task**        | `generic.TaskResource`        | `Completable`, `Suspendable`, `Guardable`, `DataExtractable`             | Run-to-completion workloads                            |
| **Integration** | `generic.IntegrationResource` | `Operational`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable` | External-dependency objects (services, ingresses)      |

Every generic resource also satisfies `concepts.Previewable`, `concepts.MutationInspector`, `concepts.DataProducer`, and
`concepts.DataConsumer`, regardless of category. The category choice determines which status handlers are required or
meaningful (see Choosing a category below) and which methods the resource wrapper needs to implement.

### 2. Define the mutation type alias

Create a type alias for `feature.Mutation` parameterized on the mutator, mirroring the alias each built-in primitive
exports:

```go
type Mutation = feature.Mutation[*Mutator]
```

This gives callers a clean name when defining feature mutations for the wrapped kind.

### 3. Implement the mutator

The mutator records mutation intent and applies it in a single controlled pass. It must implement
`generic.FeatureMutator`:

```go
type FeatureMutator interface {
    Apply() error
    NextFeature()
}
```

`Apply()` executes all recorded mutations against the underlying object; `NextFeature()` advances to a new feature
scope, called by the framework between each registered mutation to maintain per-feature ordering boundaries. Mutator
methods record intent rather than modifying the object directly, the same plan-and-apply model the built-in primitives
use. The key decision here is keeping the exposed methods domain-specific (`SetMaxConnections`, `SetReplicas`) rather
than generic, so feature mutations stay self-documenting:

```go
type Mutator struct {
    current *examplev1.MessageQueue
    plans   []featurePlan
    active  *featurePlan
}

func NewMutator(current *examplev1.MessageQueue) *Mutator {
    m := &Mutator{current: current}
    m.NextFeature()
    return m
}

func (m *Mutator) NextFeature() {
    m.plans = append(m.plans, featurePlan{})
    m.active = &m.plans[len(m.plans)-1]
}

func (m *Mutator) SetReplicas(replicas int32) {
    m.active.replicaOps = append(m.active.replicaOps, func(spec *examplev1.MessageQueueSpec) {
        spec.Replicas = &replicas
    })
}

func (m *Mutator) Apply() error {
    for _, plan := range m.plans {
        for _, op := range plan.replicaOps {
            op(&m.current.Spec)
        }
    }
    return nil
}
```

### 4. Implement status handlers

Status handlers translate the CRD's runtime state into framework status types. Which handlers are needed depends on
category (see Choosing a category below). The generic builder's `Build()` fails if the convergence handler is missing:
for workload and task resources this is the handler registered with `WithCustomConvergeStatus`; for integration
resources it is `WithCustomOperationalStatus`. Every other handler defaults to a safe value at the generic layer: grace
status defaults to `Healthy` (workload and integration only), suspension status defaults to `Suspended`, the suspension
mutation defaults to a no-op, and the delete-on-suspend decision defaults to `false`. Register custom handlers only
where the CRD has domain-specific behavior.

The convergence handler and the grace handler evaluate the same object in the same reconcile loop, with no refetch
between them. When convergence returns `Healthy`, grace is never called; for every other state, grace must not
contradict convergence by also returning `Healthy`. The component logs a warning when it detects this inconsistency; if
intentional, pass `component.SuppressGraceInconsistencyWarning()` to `WithResource` to silence it.

### 5. Implement the builder

The builder wraps the generic builder (`generic.NewWorkloadBuilder`, `generic.NewStaticBuilder`,
`generic.NewTaskBuilder`, or `generic.NewIntegrationBuilder`), registers default handlers in its constructor, and
exposes a fluent configuration API. The identity function is required and must produce a stable, unique identity: the
framework's convention is `<groupversion>/<Kind>/<namespace>/<name>`. Every method should return `*Builder` for
chaining, and `Build()` validates before delegating to the generic build, which checks a non-nil object, a name, a
namespace (unless cluster-scoped), the identity function, the mutator factory, the required convergence handler, and
unique mutation names.

```go
type Builder struct {
    base *generic.WorkloadBuilder[*examplev1.MessageQueue, *Mutator]
}

func NewBuilder(mq *examplev1.MessageQueue) *Builder {
    identityFunc := func(mq *examplev1.MessageQueue) string {
        return fmt.Sprintf("messagequeues.example.io/v1/MessageQueue/%s/%s", mq.Namespace, mq.Name)
    }

    base := generic.NewWorkloadBuilder[*examplev1.MessageQueue, *Mutator](mq, identityFunc, NewMutator)
    base.
        WithCustomConvergeStatus(DefaultConvergingStatusHandler).
        WithCustomGraceStatus(DefaultGraceStatusHandler)

    return &Builder{base: base}
}

func (b *Builder) WithMutation(ms ...Mutation) *Builder {
    for _, m := range ms {
        b.base.WithMutation(feature.Mutation[*Mutator](m))
    }
    return b
}

func (b *Builder) Build() (*Resource, error) {
    genericRes, err := b.base.Build()
    if err != nil {
        return nil, err
    }
    return &Resource{base: genericRes}, nil
}
```

Expose declared data the same way every built-in primitive does: forward `WithDataGuard(cells ...concepts.DataCell)` and
`WithOptionalData(cells ...concepts.DataCell)` to the base as fluent methods, and add a package-level `ExtractInto`
function. It is package-level rather than a builder method because a Go method cannot introduce the value type
parameter; `generic.ExtractInto` takes a `*generic.BaseBuilder`, which every category builder embeds.

```go
func ExtractInto[V any](
    b *Builder, cell *concepts.Data[V], fn func(examplev1.MessageQueue) (V, error),
) {
    generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
```

`generic.WrapGuard` and `generic.WrapExtraction` convert value-receiver callbacks (`func(T)` and `func(T) (V, error)`)
into the pointer-receiver form the generic layer expects, so the wrapper's public API can take the kind by value.

### 6. Implement the resource

The resource is a thin wrapper that delegates every interface method to the generic base. This layer exists so the
package exports a concrete type rather than a generic one; list the interfaces it satisfies in its GoDoc. Do not omit
`Preview()`: it satisfies `concepts.Previewable`, and without it `component.Preview()` fails at runtime and golden
snapshot tests cannot render the resource. `RegisteredMutations()` and `FiringSet()` satisfy
`concepts.MutationInspector` and are used by version-matrix golden generation to introspect which mutations a resource
registers and which fire at a given version; delegate both to the base. Forward `ProducedData` and `ConsumedData`
whenever the resource can take part in a component's data flow, which is always if the builder exposes `ExtractInto`,
`WithDataGuard`, or `WithOptionalData`: they satisfy `concepts.DataProducer` and `concepts.DataConsumer`, and without
them build-time topology validation silently passes, `DataTopology()` omits the resource, and its cells are never
cleared at the start of a reconcile. Forward `RecordObservation` whenever the resource may be registered read-only and
declares an extraction, since the framework feeds the fetched cluster object back to the resource before extraction
runs.

Which methods to include depends on category: a Static resource needs only `Identity`, `Object`, `Mutate`,
`GuardStatus`, `ExtractData`, `ProducedData`, `ConsumedData`, `RecordObservation`, `Preview`, `RegisteredMutations`, and
`FiringSet`. Workload, Task, and Integration resources add `ConvergingStatus`, `DeleteOnSuspend`, `Suspend`, and
`SuspensionStatus`; Workload and Integration additionally add `GraceStatus`. For Task and Integration resources,
`ConvergingStatus` returns `concepts.CompletionStatusWithReason` and `concepts.OperationalStatusWithReason`
respectively, matching the generic base method signature.

### 7. Define feature mutations

Feature mutations use the `Mutation` alias from step 2. Each declares a name, an optional feature gate, and a function
that calls mutator methods to record intent. Name every mutation: the name is what gating and error reporting refer to,
and the builder rejects duplicate names within a resource. Mutations apply in registration order; when a mutation's
`Feature` is nil or its gate reports enabled, its `Mutate` function runs, otherwise it is skipped. Version gating uses
`feature.NewVersionGate`, boolean conditions combine with `.When(...)`.

### 8. Register with a component

Use the custom resource with the component builder exactly like a built-in primitive: build it with the wrapper's
`NewBuilder`, register feature mutations with `WithMutation`, call `Build()`, then pass the result to
`component.NewComponentBuilder().WithResource(...)`. Resource options such as `ReadOnly()`, `Auxiliary()`, and
`BlockOnAbsence()` apply the same way they do to built-in primitives.

## Choosing a category

The category choice determines which status handlers are required, which are meaningful, and which methods the resource
wrapper implements (step 6 above):

- **Static** resources have the simplest implementation. They do not participate in convergence, grace, or suspension
  reporting; the builder uses `generic.NewStaticBuilder`. `pkg/primitives/configmap` is a complete reference.
- **Task** resources use `generic.NewTaskBuilder` and report convergence as `concepts.CompletionStatusWithReason`
  instead of `AliveStatusWithReason`. The converging handler, registered with `WithCustomConvergeStatus`, reports
  `Completed`, `TaskRunning`, `TaskPending`, or `TaskFailing`.
- **Integration** resources use `generic.NewIntegrationBuilder` and report convergence as
  `concepts.OperationalStatusWithReason`. The handler is registered with `WithCustomOperationalStatus`, not
  `WithCustomConvergeStatus`, and reports `Operational`, `OperationPending`, or `OperationFailing`. Integration
  resources also implement `Graceful`, defaulting to `Healthy`, so the resource wrapper includes `GraceStatus` alongside
  the other methods. `pkg/primitives/service` is a complete reference, including a grace handler that mirrors the
  operational logic.
- **Workload** resources implement the full set: `Alive`, `Graceful`, `Suspendable`, `Guardable`, and `DataExtractable`,
  with convergence reported as `AliveStatusWithReason`.

## Cluster-scoped wrappers

For cluster-scoped CRDs, call `MarkClusterScoped()` on the generic builder before building. Validation then rejects a
non-empty namespace instead of requiring one, and the identity function should omit the namespace segment.

## Anti-patterns

- **Skipping status handlers.** The component can never report readiness if the required convergence handler
  (`WithCustomConvergeStatus` or, for Integration, `WithCustomOperationalStatus`) is missing: `Build()` fails outright.
  Register it even for a minimal implementation.
- **Embedding owner-specific logic in the wrapper instead of feature mutations.** Version-dependent or feature-flag
  dependent behavior belongs in a named, gated `Mutation` (step 7), not hardcoded into the mutator or builder defaults.
  Hardcoding it defeats gating, golden testing, and per-mutation introspection.
- **Wrapping a kind that already has a built-in primitive.** Check the built-in primitive list before writing a wrapper;
  duplicating an existing primitive's behavior in a custom wrapper creates two divergent implementations of the same
  kind.

## Ground truth

The consumer's resolved module version is the source of truth, not these docs. Before asserting an exact signature,
method name, or option:

1. Read the framework version from the consumer's `go.mod` entry for
   `github.com/sourcehawk/operator-component-framework`.
2. Verify the symbol with `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

The reference files bundled with this skill match the framework version this plugin shipped with. When they disagree
with `go doc`, `go doc` wins.

## References

- `references/custom-resource.md`: the complete worked example (a `MessageQueue` workload CRD, plus a `DNSRecord`
  integration example), including the full mutator, builder, resource, and feature mutation listings, the
  status-constant reference table, and the cluster-scoped and category-specific sections referenced above.
