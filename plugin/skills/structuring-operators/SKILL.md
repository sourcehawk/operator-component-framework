---
name: structuring-operators
description:
  Use when designing, structuring, or reviewing an operator built on the operator-component-framework - desired state in
  the baseline, pure mutations, one component per logical condition, thin controllers, mutation ordering and layering,
  prerequisites vs guards vs feature gates, participation modes, grace periods, naming conventions, version floors, and
  supported version pinning.
---

# Structuring Operators

## Three load-bearing principles

**Desired state lives in the baseline object.** The object passed to a primitive builder should already read as the
real, latest-shape resource: name, labels, selector, replicas, ports, probes, and the primary container all belong
there. Mutations layer conditional or version-dependent concerns on top of a complete, valid baseline, not the other way
around.

**Mutations are pure functions of the spec.** A mutation computes its output from the owner spec and other build-time
inputs only. It never reads a resource's live cluster state to decide what to write, and within a single resource it
runs before that resource's own declared extractions, so a mutation can never read a data cell its own resource
produces.

**One component per logical condition.** If users would ask "is the backend ready?" and "is the frontend ready?" as
separate questions, those are separate components, each reporting its own condition. Combine resources into one
component only when they have no useful readiness independent of each other.

## The guideline index

Every guideline from `references/guidelines.md`, verbatim, with the rule in one sentence. Use this table as a review
checklist: a change that violates one of these rules is a candidate for rework, not just a style nit.

| Guideline                                                           | Rule                                                                                                                                                                                                                                    |
| ------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Represent Desired State in the Baseline Object                      | Put every field that is always present, regardless of version or feature flags, in the baseline; leave only conditional fields to mutations.                                                                                            |
| Mutations Are Pure Functions of the Spec                            | A mutation is a pure function of the owner spec and build-time inputs; it must never read live cluster state.                                                                                                                           |
| Leave Version-Dependent Fields Empty in the Baseline                | Give each field exactly one owner; when a value depends on the spec version, leave it empty in the baseline and let a single mutation set it.                                                                                           |
| One Component Per Logical Condition                                 | Split components when users would ask about their health separately or a failure in one should not mask another; combine when resources have no independent readiness.                                                                  |
| Keep Controllers Thin                                               | A controller fetches the owner, builds and reconciles components, and defers one `FlushStatus`; resource construction and mutation logic live in pure, testable component-building functions.                                           |
| Reconciler Error Handling and Requeueing                            | Return an error only for a genuine fault (a failed API call, a mutation that cannot apply, a version below the supported floor); let a merely converging resource report through its condition and requeue via normal watch and resync. |
| Resource Registration Order Is Execution Order                      | Resources reconcile in the exact order they were registered with `WithResource`; register dependencies before dependents.                                                                                                               |
| Mutation Ordering and Container-Name Dependencies                   | Use broad, name-independent selectors for version-independent mutations, and register name-specific mutations before any compat mutation that renames the container.                                                                    |
| Layer Mutations in a Fixed Order                                    | Order a resource's mutations into fixed layers: defaults, compat, overrides, then checksum, so the pipeline reads the same way for every workload.                                                                                      |
| Prefer Reverting Compat Mutations Over Forward Mutations            | Keep the baseline at the latest shape and add a version-gated revert mutation per structural change, rather than holding the baseline at an old shape and patching it forward.                                                          |
| Use Data Extraction and Guards for Intra-Component Dependencies     | When one resource depends on data from another resource in the same component, declare the flow with a data cell: `ExtractInto` on the producer, `WithDataGuard` or `WithOptionalData` on the consumer.                                 |
| Use Prerequisites for Cross-Component Dependencies                  | When a component cannot start until another component is ready, attach a prerequisite instead of orchestrating ordering in the controller.                                                                                              |
| Use Feature Gates for Optional Components and Conditional Resources | Gate optional pieces with a feature gate so the framework owns the full lifecycle, including deletion when the gate flips off.                                                                                                          |
| Provide a User-Override Escape Hatch as the Last Mutation           | Apply a documented user-override mutation as the last value-producing mutation so the user's input shadows the operator's own defaults.                                                                                                 |
| Fail Loudly Below the Supported Version Floor                       | Return an error from a compat mutation, rather than emit a silently wrong approximation, when a requested version is below the supported floor.                                                                                         |
| Name Mutations for Golden Introspection                             | Give every mutation a descriptive `Name`; golden manifests reference those names in their `requires` and `forbids` lists.                                                                                                               |
| Understand Participation Modes                                      | `Auxiliary` means reconciled but not required for the condition to go Ready, not skipped; a blocked guard always contributes to the condition regardless of participation mode.                                                         |
| Grace Periods Are Convergence Time                                  | Set the grace period to how long a resource legitimately takes to converge, not as a general safety margin.                                                                                                                             |
| Handle Cluster-Scoped Resources Explicitly                          | Clean up cluster-scoped resources explicitly with `Delete()` or `DeleteWhen()` plus a finalizer, since the framework cannot set an owner reference across a scope boundary.                                                             |
| Name Resources to Avoid Multi-Tenant Collisions                     | Derive every managed resource's name from the owner, and fold in the owner's namespace for cluster-scoped resources, which share one global namespace.                                                                                  |
| Name Conditions for the Audience Reading Them                       | Name condition types after the capability they represent (`BackendReady`), not the Kubernetes resource type backing them (`StatefulSetHealthy`).                                                                                        |
| Pin Rendered Output Across Supported Versions                       | Cover every supported version's rendered output with a golden, so a baseline change can be proven to touch only the version intended.                                                                                                   |

## Choosing the right dependency mechanism

Three mechanisms cover three different dependency shapes. Picking the wrong one either breaks silently or forces
orchestration logic back into the controller.

**Declared data, for a dependency between two resources inside one component.** Create a typed cell with
`concepts.NewData[T]`, declare the producer's extraction with the primitive package's `ExtractInto` function, and
declare the consumer's read with `WithDataGuard` (block until present, read with `Require`) or `WithOptionalData` (never
gates; a mutation reads with `Get` and enriches only when the value is there). `Build()` rejects the component unless a
producer is registered strictly earlier than every reader, so a broken flow is a build error rather than a resource
waiting forever.

```go
roleARN := concepts.NewData[string]("cloud-role-arn")

roleBuilder := static.NewBuilder(cloudRole(app))
static.ExtractInto(roleBuilder, roleARN, func(obj uns.Unstructured) (string, error) {
    arn, _, err := uns.NestedString(obj.Object, "status", "arn")
    return arn, err
})
roleRes, _ := roleBuilder.Build()

bucketBuilder := static.NewBuilder(cloudBucket(app))
bucketBuilder.WithDataGuard(roleARN)
bucketRes, _ := bucketBuilder.Build()
```

`WithDataGuard` generates both the guard and its reason (`waiting for data "cloud-role-arn"`), so the message users read
cannot drift from the real dependency; keep `WithGuard` for preconditions that are not "a value exists". A data guard
re-evaluates every reconcile, so it naturally re-blocks the dependent if its input disappears. Prefer stable values (a
status field written once, a generated credential reference) over values that can transiently clear during normal
operation (a replica count, a field cleared mid rolling-update), or the guard will re-block a resource that is already
running. This applies doubly to optional enrichment, which has no guard to hold the resource back: a source value that
comes and goes makes the enriched field flap.

**Prerequisites, for a dependency between two components.** Attach `WithPrerequisite` on the dependent component rather
than sequencing the components in the controller.

```go
frontendComp, err := component.NewComponentBuilder().
    WithName("frontend").
    WithConditionType("FrontendReady").
    WithPrerequisite(component.DependsOn("BackendReady")).
    WithResource(frontendService).
    WithResource(frontendDeployment).
    Build()
```

A prerequisite is a startup barrier only: once a component passes it for the first time, the barrier is permanently
satisfied and never re-checked, even if the depended-on component later becomes unhealthy. Use this for "can this
component be created?", not for ongoing health coupling; if the backend later goes down, the frontend keeps reconciling
and the two conditions reflect their own health independently.

**Feature gates, for an optional component or an optional resource within a component.** A component-level
`WithFeatureGate` disables the whole component, deleting its resources and reporting `True/Disabled`.

```go
cacheComp, err := component.NewComponentBuilder().
    WithName("cache").
    WithConditionType("CacheReady").
    WithFeatureGate(feature.NewVersionGate(app.Spec.Version, nil).When(app.Spec.Cache.Enabled)).
    WithResource(cacheService).
    WithResource(cacheDeployment).
    Build()
```

A resource-level `component.GatedBy` does the same for one resource the component owns, deleting it once the gate turns
off. For an optional resource the component does not own, such as a read-only Secret reference behind an optional spec
field, use `IncludeWhen` instead, which omits the resource without ever deleting it.

The three mechanisms compose: a component can have a feature gate, a prerequisite, and internal guards simultaneously,
each answering a different question (is this component enabled, can it start, is this resource's own dependency
satisfied right now).

## Compatibility

The framework requires Go 1.26 or later, and its own supported dependency combinations are documented in
`references/compatibility.md`:

| Framework | controller-runtime | k8s.io/\* | Kubernetes | Go   | Status  |
| --------- | ------------------ | --------- | ---------- | ---- | ------- |
| main      | v0.24.x            | v0.36.x   | 1.36       | 1.26 | Primary |
| main      | v0.23.x            | v0.35.x   | 1.35       | 1.26 | Tested  |
| main      | v0.22.x            | v0.34.x   | 1.34       | 1.26 | Tested  |

**Primary** is tested on every commit and is what `go.mod` declares. **Tested** combinations are verified weekly and are
fully supported: bugs reported against a Tested combination are treated as bugs in the framework, not as unsupported
configurations. Versions v0.21.x and below are not supported at all, because dependency module path migrations in that
range make those combinations irresolvable. When you need to stay on an older Tested combination, pin your own
controller-runtime and `k8s.io/*` versions with `replace` directives in your `go.mod`; Go's Minimum Version Selection
otherwise pulls your dependencies up to the framework's declared minimums.

The same "fail below the floor, do not degrade quietly" policy applies to an operator's own owner-CRD version field. Per
the Fail Loudly Below the Supported Version Floor guideline, when a compat mutation cannot faithfully represent a
requested version, it should return an error rather than render an approximation:

```go
func compatV1Container(app *v1alpha1.WebApp) deployment.Mutation {
    return deployment.Mutation{
        Name:    "CompatV1Container",
        Feature: feature.NewVersionGate(app.Spec.Version, []feature.VersionConstraint{lessThan("2.0.0")}),
        Mutate: func(m *deployment.Mutator) error {
            if belowFloor(app.Spec.Version, "1.0.0") {
                return fmt.Errorf("version %s is below the supported floor 1.0.0", app.Spec.Version)
            }
            // ... roll back to the legacy shape
            return nil
        },
    }
}
```

That error propagates out of `Component.Reconcile`, and because `FlushStatus` is deferred, the failure still lands on
the owner's condition even as the error causes controller-runtime to back off and retry. Do not attempt a best-effort
render for an unsupported version: a loud, visible failure is the correct behavior, not a bug to be worked around.

## Ground truth

The consumer's resolved module version is the source of truth, not these docs. Before asserting an exact signature,
method name, or option:

1. Read the framework version from the consumer's `go.mod` entry for
   `github.com/sourcehawk/operator-component-framework`.
2. Verify the symbol with `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

The reference files bundled with this skill match the framework version this plugin shipped with. When they disagree
with `go doc`, `go doc` wins.

## References

- `references/guidelines.md`: full guideline text, rationale, and code examples for every row in the guideline index
  above.
- `references/compatibility.md`: the framework's supported version matrix, the Go requirement, the version floor policy,
  and how to pin your own controller-runtime and Kubernetes versions.
