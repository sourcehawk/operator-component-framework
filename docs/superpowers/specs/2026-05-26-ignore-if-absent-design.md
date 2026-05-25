# IgnoreIfAbsent for read-only resources

Status: design approved, pending implementation plan
Date: 2026-05-26

## Problem

A component frequently needs to reference a Kubernetes resource it does not
manage (a `Secret` or `ConfigMap` owned by another operator, a CRD provided by
the platform, etc.). Today the framework offers two read-only behaviors when
such a resource is absent from the cluster:

| Mode                                       | Behavior on `NotFound`                                                                            |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------- |
| `ReadOnly()`                               | Returns the error, triggering controller-runtime exponential backoff.                             |
| `ReadOnly().BlockOnAbsence()`              | Records a `guard-blocked` status (`waiting for <resource>`) and short-circuits the remaining resources. |

Neither matches a common third case: a resource that is genuinely **optional**.
If present, the consumer wants to hash its contents (via a data extractor) so
that changes propagate to downstream rolling updates; if absent, the component
should simply proceed as if the resource was not registered.

The current workarounds are awkward — for example wrapping the registration in
a `When(...)` condition that performs its own pre-flight `Get`, duplicating the
read the framework would do anyway.

## Goal

Add a third NotFound behavior, `IgnoreIfAbsent`, that:

1. Treats `IsNotFound` on the read-only fetch as a non-error.
2. Skips status collection, observation recording, and data extraction for the
   absent entry.
3. Does not interrupt or block subsequent resources in the component.
4. Reports nothing about the absent resource in component conditions.

## Non-goals

- Allowing `IgnoreIfAbsent` on managed (non-read-only) resources. Managed
  resources are created by the component, so "absent" is a transient state the
  framework resolves itself.
- Surfacing the missing optional resource in any condition entry. The chosen
  semantics are silent.
- Ergonomic improvements to primitive builders for reference-only registration
  (e.g., `secret.NewRefBuilder(name, namespace)`). Tracked separately.

## Design

### Builder API

A new method on `ResourceOptionsBuilder`:

```go
// IgnoreIfAbsent opts a read-only resource into "optional" semantics: if the
// cluster reports NotFound, the framework silently skips this entry and
// continues reconciling subsequent resources. No condition is reported, no
// observation is recorded, and the data extractor is not invoked.
//
// Only valid alongside ReadOnly(); Build() returns an error otherwise.
// Mutually exclusive with BlockOnAbsence(); Build() returns an error if both
// are set.
func (b *ResourceOptionsBuilder) IgnoreIfAbsent() *ResourceOptionsBuilder
```

### ResourceOptions struct

`ResourceOptions` gains a sibling field to `BlockOnAbsence`:

```go
// IgnoreIfAbsent applies to read-only resources. When true, a NotFound
// response when reading the resource is silently ignored: the entry is
// skipped, no condition or observation is recorded, and reconciliation of
// subsequent resources continues unchanged.
IgnoreIfAbsent bool
```

### Build-time validation

`ResourceOptionsBuilder.Build()` returns an error in three new cases:

1. `IgnoreIfAbsent` is set without `ReadOnly`.
2. `IgnoreIfAbsent` and `BlockOnAbsence` are both set.
3. `BlockOnAbsence` is set without `ReadOnly`.

Case 3 is a tightening of existing behavior: today `BlockOnAbsence` on a
managed resource is a silent no-op with only a GoDoc warning. A no-op flag set
on a managed resource is almost certainly a misconfiguration, so erroring at
Build is an improvement and keeps the two NotFound flags consistent.

When a feature gate or `When` condition forces `Delete=true`, `ReadOnly` is
flipped to `false` by Build. In that case the NotFound flags are functionally
inert (the resource is being deleted, not read). Build does **not** error in
this case; both flags pass through to the resulting `ResourceOptions` and are
simply not consulted at runtime.

### Runtime behavior

The existing read-only NotFound branch in `pkg/component/create.go` is
extended. The current shape (post-`BlockOnAbsence`):

```go
if err != nil {
    if entry.Options.ReadOnly && entry.Options.BlockOnAbsence && apierrors.IsNotFound(err) {
        // record guard-blocked, short-circuit
        return results, nil
    }
    return nil, err
}
```

becomes:

```go
if err != nil {
    if entry.Options.ReadOnly && apierrors.IsNotFound(err) {
        switch {
        case entry.Options.IgnoreIfAbsent:
            continue // skip this entry, keep reconciling
        case entry.Options.BlockOnAbsence:
            // existing guard-blocked short-circuit, unchanged
            results = append(results, reconcileResult{ ... })
            return results, nil
        }
    }
    return nil, err
}
```

`readResource` itself is untouched. It still returns the wrapped NotFound
error, and `reconcileResources` decides what to do with it. This concentrates
the policy in one place and keeps the read path simple.

### GoDoc and docs updates

In the same change:

- Update GoDoc on `BlockOnAbsence()`: replace the "no effect on managed
  resources" sentence with the new Build-error semantics.
- Add a row to the ResourceOptions table in `docs/component.md` for
  `ReadOnly + IgnoreIfAbsent`.
- Add a row to the builder method table in `docs/component.md` for
  `IgnoreIfAbsent()`.
- Update the existing `BlockOnAbsence()` row in the builder method table to
  reflect the tightened ReadOnly requirement.

No changes to `docs/primitives.md`, `docs/custom-resource.md`, or
`docs/guidelines.md` are required — they do not currently mention the NotFound
flags.

## Testing

### Reconciliation tests (`pkg/component/create_test.go`)

Mirror the existing `TestReconcileResources_BlockOnAbsence` shape:

- **Missing read-only resource with `IgnoreIfAbsent`** — no error, no result
  appended for that entry, subsequent resources still processed.
- **Missing read-only resource without either flag** — still errors. Regression
  guard for the default behavior.
- **Subsequent resources continue after an ignored absence** — verifies the
  `continue` semantics versus `BlockOnAbsence`'s short-circuit.
- **Present read-only resource with `IgnoreIfAbsent`** — behaves identically to
  plain `ReadOnly`: extractor runs, observation is recorded, status is
  collected.

### Builder validation tests (`pkg/component/resource_options_builder_test.go`)

- `IgnoreIfAbsent()` without `ReadOnly()` → Build error.
- `IgnoreIfAbsent()` together with `BlockOnAbsence()` → Build error.
- `BlockOnAbsence()` without `ReadOnly()` → Build error (tightening).
- `IgnoreIfAbsent()` + `ReadOnly()` happy path → `ResourceOptions{ReadOnly:
  true, IgnoreIfAbsent: true}`.
- `IgnoreIfAbsent()` + `ReadOnly()` + disabled feature → no error,
  `Delete=true`, `ReadOnly=false`, `IgnoreIfAbsent` flag preserved on the
  struct (inert at runtime).

### E2E

No new E2E coverage. This is a small, well-scoped change covered comprehensively
by unit tests against the real `reconcileResources` loop and a fake client.

## Backward compatibility

- `IgnoreIfAbsent` is purely additive — the zero value is `false`, which
  preserves today's behavior.
- The `BlockOnAbsence` tightening is a small behavior change: previously
  `BlockOnAbsence()` without `ReadOnly()` silently produced a no-op flag; now
  it returns a Build error. Any caller relying on the silent no-op was almost
  certainly misconfigured. Release notes should mention this explicitly.

## Open questions

None.
