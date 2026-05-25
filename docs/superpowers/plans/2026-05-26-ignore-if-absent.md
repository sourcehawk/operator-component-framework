# IgnoreIfAbsent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third NotFound mode for read-only resources, `IgnoreIfAbsent`, that silently skips the resource and continues reconciliation when the cluster reports the resource is missing.

**Architecture:** A boolean field on `ResourceOptions`, a builder method that sets it, build-time validation that requires `ReadOnly()` and forbids combining with `BlockOnAbsence()`, and one extra `case` in the existing NotFound branch of `reconcileResources` that emits `continue` instead of a guard-blocked result. The same change tightens `BlockOnAbsence()` to also require `ReadOnly()` at Build time so the two NotFound flags stay consistent.

**Tech Stack:** Go, testify (assert/require), controller-runtime fake client, sigs.k8s.io/controller-runtime/pkg/client.

**Spec:** `docs/superpowers/specs/2026-05-26-ignore-if-absent-design.md`

---

## File Structure

Files modified:

- `pkg/component/builder.go` — add `IgnoreIfAbsent` field to `ResourceOptions` struct.
- `pkg/component/resource_options_builder.go` — add `IgnoreIfAbsent()` method, add validation in `Build()`, update `BlockOnAbsence()` GoDoc.
- `pkg/component/resource_options_builder_test.go` — add tests for the new method and the new validation errors.
- `pkg/component/create.go` — extend the NotFound branch in `reconcileResources` to handle the new flag.
- `pkg/component/create_test.go` — add `TestReconcileResources_IgnoreIfAbsent` mirroring the existing `TestReconcileResources_BlockOnAbsence`.
- `docs/component.md` — add table rows for `IgnoreIfAbsent()` and update the `BlockOnAbsence()` row to reflect the tightened ReadOnly requirement.

No new files.

---

## Task 1: Wire up the `IgnoreIfAbsent` flag (field + builder method)

Add the struct field and a builder method that sets it. No validation yet — that comes in Task 2 — so this commit can stand on its own as the pure wiring change.

**Files:**

- Modify: `pkg/component/builder.go:35-41` (ResourceOptions struct, around the existing BlockOnAbsence field)
- Modify: `pkg/component/resource_options_builder.go:13-21` (builder struct field), `:81-96` (after BlockOnAbsence method), `:110-139` (Build method passes the flag through)
- Modify: `pkg/component/resource_options_builder_test.go:180-189` (insert new happy-path test cases in the table)

- [ ] **Step 1: Write the failing builder tests**

Open `pkg/component/resource_options_builder_test.go`. Locate the `"last WithFeatureGate wins"` entry near the end of the `tests` slice (around line 181). Insert these two cases immediately before it:

```go
		{
			name: "ignore if absent sets flag alongside read-only",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().ReadOnly().IgnoreIfAbsent().Build()
			},
			want: ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true},
		},
		{
			name: "ignore if absent preserved when deletion forced",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					ReadOnly().
					IgnoreIfAbsent().
					Build()
			},
			want: ResourceOptions{Delete: true, ReadOnly: false, IgnoreIfAbsent: true},
		},
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./pkg/component/ -run TestResourceOptionsBuilder_Build`
Expected: compile error referencing the unknown `IgnoreIfAbsent` field on `ResourceOptions` and the unknown `IgnoreIfAbsent` method on `*ResourceOptionsBuilder`.

- [ ] **Step 3: Add the `IgnoreIfAbsent` field to `ResourceOptions`**

In `pkg/component/builder.go`, locate the `ResourceOptions` struct. Append a new field after the existing `BlockOnAbsence` field (currently at the bottom of the struct):

```go
	// IgnoreIfAbsent applies to read-only resources. When true, a NotFound
	// response from the cluster when reading the resource is silently ignored:
	// the entry is skipped, no condition or observation is recorded, the data
	// extractor is not invoked, and reconciliation of subsequent resources
	// continues unchanged.
	//
	// Use this when the resource is genuinely optional (e.g., a reference to a
	// Secret owned by another operator that may or may not exist). It is
	// mutually exclusive with BlockOnAbsence and requires ReadOnly.
	IgnoreIfAbsent bool
```

- [ ] **Step 4: Add the builder field and method**

In `pkg/component/resource_options_builder.go`, add a new field in the `ResourceOptionsBuilder` struct (immediately after `blockOnAbsence`):

```go
	ignoreIfAbsent                    bool
```

The struct should now look like:

```go
type ResourceOptionsBuilder struct {
	feature        feature.Gate
	requiredTruths []bool

	readOnly                          bool
	blockOnAbsence                    bool
	ignoreIfAbsent                    bool
	participationMode                 ParticipationMode
	suppressGraceInconsistencyWarning bool
}
```

Then add the method immediately after the existing `BlockOnAbsence` method (after the closing brace of that method, before the `Build` method):

```go
// IgnoreIfAbsent opts a read-only resource into "optional" semantics: if the
// cluster reports NotFound when reading the resource, the framework silently
// skips this entry and continues reconciling subsequent resources. No
// condition is reported, no observation is recorded, and the data extractor
// is not invoked.
//
// Only valid alongside ReadOnly(); Build() returns an error otherwise.
// Mutually exclusive with BlockOnAbsence(); Build() returns an error if both
// are set.
func (b *ResourceOptionsBuilder) IgnoreIfAbsent() *ResourceOptionsBuilder {
	b.ignoreIfAbsent = true
	return b
}
```

Finally, propagate the flag in `Build()`. Locate the `return ResourceOptions{...}` block at the end of `Build()` and add the new field:

```go
	return ResourceOptions{
		Delete:                            shouldDelete,
		ReadOnly:                          b.readOnly && !shouldDelete,
		BlockOnAbsence:                    b.blockOnAbsence,
		IgnoreIfAbsent:                    b.ignoreIfAbsent,
		ParticipationMode:                 b.participationMode,
		SuppressGraceInconsistencyWarning: b.suppressGraceInconsistencyWarning,
	}, nil
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/component/ -run TestResourceOptionsBuilder_Build -v`
Expected: PASS, including the two new cases `ignore if absent sets flag alongside read-only` and `ignore if absent preserved when deletion forced`.

- [ ] **Step 6: Commit**

```bash
git add pkg/component/builder.go pkg/component/resource_options_builder.go pkg/component/resource_options_builder_test.go
git commit -m "$(cat <<'EOF'
add IgnoreIfAbsent flag to ResourceOptions

Adds the field, builder method, and Build() passthrough. Build-time
validation and runtime behavior land in follow-up commits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Tighten Build validation for both NotFound flags

Add the three new error cases at `Build()` time:

1. `IgnoreIfAbsent` set without `ReadOnly`.
2. `BlockOnAbsence` and `IgnoreIfAbsent` both set.
3. `BlockOnAbsence` set without `ReadOnly` (tightening — previously a silent no-op).

Validation uses the raw builder flags (`b.readOnly`), not the post-delete computed `ReadOnly` value. This is intentional: if a user wrote `ReadOnly().BlockOnAbsence().When(false)`, the user-supplied configuration is internally consistent and Build should not error — the resource is simply being deleted instead of read.

**Files:**

- Modify: `pkg/component/resource_options_builder.go:81-96` (BlockOnAbsence GoDoc update) and `:110-139` (Build method)
- Modify: `pkg/component/resource_options_builder_test.go` (the existing `tests` slice + add a follow-on test function for error cases)

- [ ] **Step 1: Write the failing validation tests**

In `pkg/component/resource_options_builder_test.go`, add a new top-level test function after the existing `TestResourceOptionsFor` block:

```go
func TestResourceOptionsBuilder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		build     func() (ResourceOptions, error)
		wantErrIs string
	}{
		{
			name: "IgnoreIfAbsent without ReadOnly errors",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().IgnoreIfAbsent().Build()
			},
			wantErrIs: "IgnoreIfAbsent requires ReadOnly",
		},
		{
			name: "BlockOnAbsence without ReadOnly errors",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().BlockOnAbsence().Build()
			},
			wantErrIs: "BlockOnAbsence requires ReadOnly",
		},
		{
			name: "BlockOnAbsence and IgnoreIfAbsent are mutually exclusive",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					ReadOnly().
					BlockOnAbsence().
					IgnoreIfAbsent().
					Build()
			},
			wantErrIs: "BlockOnAbsence and IgnoreIfAbsent are mutually exclusive",
		},
		{
			name: "BlockOnAbsence + deleted (no ReadOnly) does not error: user supplied ReadOnly, delete-precedence flipped it",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					ReadOnly().
					BlockOnAbsence().
					Build()
			},
			// This case must NOT error — covered by the happy-path table.
			// Included here as documentation of intent.
			wantErrIs: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()
			if tt.wantErrIs == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrIs)
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/component/ -run TestResourceOptionsBuilder_ValidationErrors -v`
Expected: FAIL — the three error cases return no error because validation has not been added yet.

- [ ] **Step 3: Add the validation to `Build()`**

In `pkg/component/resource_options_builder.go`, replace the entire `Build()` function with this version (adds three validation checks at the top, keeps the rest unchanged):

```go
func (b *ResourceOptionsBuilder) Build() (ResourceOptions, error) {
	if b.blockOnAbsence && b.ignoreIfAbsent {
		return ResourceOptions{}, fmt.Errorf(
			"BlockOnAbsence and IgnoreIfAbsent are mutually exclusive",
		)
	}
	if b.blockOnAbsence && !b.readOnly {
		return ResourceOptions{}, fmt.Errorf(
			"BlockOnAbsence requires ReadOnly",
		)
	}
	if b.ignoreIfAbsent && !b.readOnly {
		return ResourceOptions{}, fmt.Errorf(
			"IgnoreIfAbsent requires ReadOnly",
		)
	}

	shouldDelete := false

	if b.feature != nil {
		enabled, err := b.feature.Enabled()
		if err != nil {
			return ResourceOptions{}, err
		}
		if !enabled {
			shouldDelete = true
		}
	}

	if !shouldDelete {
		for _, t := range b.requiredTruths {
			if !t {
				shouldDelete = true
				break
			}
		}
	}

	return ResourceOptions{
		Delete:                            shouldDelete,
		ReadOnly:                          b.readOnly && !shouldDelete,
		BlockOnAbsence:                    b.blockOnAbsence,
		IgnoreIfAbsent:                    b.ignoreIfAbsent,
		ParticipationMode:                 b.participationMode,
		SuppressGraceInconsistencyWarning: b.suppressGraceInconsistencyWarning,
	}, nil
}
```

Add the `fmt` import at the top of the file if it is not already imported. The current imports are:

```go
import "github.com/sourcehawk/operator-component-framework/pkg/feature"
```

Change to:

```go
import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)
```

- [ ] **Step 4: Update the `BlockOnAbsence` GoDoc**

In the same file, replace the existing GoDoc on `BlockOnAbsence()` (lines ~81-92, the block ending with `// The flag has no effect on managed (non-read-only) resources.`). The new GoDoc:

```go
// BlockOnAbsence opts a read-only resource into guard-blocked semantics when
// the cluster reports NotFound: instead of returning an error (which triggers
// controller-runtime's exponential backoff), the component records a blocked
// status with a "waiting for <resource>" reason and short-circuits the
// remaining resources for the current reconcile.
//
// Use this when the consumer has a watch on the resource's type so that the
// reconcile is re-enqueued the moment the resource appears. The framework does
// not verify that a watch exists; without one the component will only retry on
// its periodic resync.
//
// Only valid alongside ReadOnly(); Build() returns an error otherwise.
// Mutually exclusive with IgnoreIfAbsent(); Build() returns an error if both
// are set.
```

Also update the GoDoc on the `BlockOnAbsence` field of the `ResourceOptions` struct in `pkg/component/builder.go`. Replace the existing comment block on the `BlockOnAbsence` field with:

```go
	// BlockOnAbsence applies to read-only resources. When true, a NotFound response
	// from the cluster is treated as a guard-blocked condition rather than an
	// error, preventing controller-runtime's exponential backoff and producing a
	// meaningful status reason. Only use this when the consumer has a watch on
	// the resource's type so that the reconcile is re-enqueued when the
	// resource appears. Mutually exclusive with IgnoreIfAbsent; the builder
	// rejects both at Build() time.
	BlockOnAbsence bool
```

- [ ] **Step 5: Run all builder tests to verify they pass**

Run: `go test ./pkg/component/ -run TestResourceOptionsBuilder -v`
Expected: PASS — both `TestResourceOptionsBuilder_Build` (all existing cases plus the two from Task 1) and `TestResourceOptionsBuilder_ValidationErrors` succeed.

Also verify the existing test case `"block on absence preserved when deletion forced"` still passes; under the new rules its raw `b.readOnly` is `true` (the user called `.ReadOnly()` before `.BlockOnAbsence()`), so Build does not error, even though the computed `ReadOnly` field is then flipped to `false` by delete-precedence.

- [ ] **Step 6: Commit**

```bash
git add pkg/component/builder.go pkg/component/resource_options_builder.go pkg/component/resource_options_builder_test.go
git commit -m "$(cat <<'EOF'
validate ReadOnly requirement and mutual exclusion for absence flags

ResourceOptionsBuilder.Build() now returns an error when:

- IgnoreIfAbsent is set without ReadOnly.
- BlockOnAbsence is set without ReadOnly (previously a silent no-op).
- BlockOnAbsence and IgnoreIfAbsent are both set.

The BlockOnAbsence ReadOnly requirement is a tightening of existing
behavior. Setting the flag on a managed resource was always a
misconfiguration; surfacing it at Build is an improvement.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Runtime branch in `reconcileResources`

Extend the existing NotFound branch so that an `IgnoreIfAbsent` resource simply `continue`s the loop instead of returning a guard-blocked result. The branch becomes a `switch` over the two flags.

**Files:**

- Modify: `pkg/component/create.go:202-222` (the NotFound branch inside `reconcileResources`)
- Modify: `pkg/component/create_test.go:560+` (append a new test function `TestReconcileResources_IgnoreIfAbsent`)

- [ ] **Step 1: Write the failing reconciliation tests**

In `pkg/component/create_test.go`, append this new test function after the closing brace of `TestReconcileResources_BlockOnAbsence`:

```go
func TestReconcileResources_IgnoreIfAbsent(t *testing.T) {
	var (
		scheme    = setupScheme()
		namespace = "test-namespace"
		owner     = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
		fakeClient       = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		reconcileContext = setupReconcileContext(scheme, owner, fakeClient)
		mapper           = createTestRESTMapper()
		ctx              = t.Context()
	)

	t.Run("a missing read-only resource with IgnoreIfAbsent is silently skipped", func(t *testing.T) {
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-optional-secret",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-optional-secret")

		entry := reconcileEntry{
			Resource: resource,
			Options:  ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true},
		}

		results, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)

		require.NoError(t, err)
		assert.Empty(t, results, "absent IgnoreIfAbsent resource must contribute no condition")
	})

	t.Run("subsequent resources still reconcile after an ignored absence", func(t *testing.T) {
		missingLeader := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-optional-leader",
				Namespace: namespace,
			},
		}
		leader := &MockResource{}
		leader.On("Object").Return(missingLeader, nil)
		leader.On("Identity").Return("v1/Secret/absent-optional-leader")

		// A follower that must be reached. Use a Secret that exists in the
		// cluster so its read succeeds; we assert Object is called on it.
		presentFollower := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "present-follower",
				Namespace: namespace,
			},
		}
		require.NoError(t, fakeClient.Create(ctx, presentFollower))

		follower := &MockResource{}
		follower.On("Object").Return(presentFollower, nil)
		follower.On("Identity").Return("v1/Secret/present-follower")

		entries := []reconcileEntry{
			{Resource: leader, Options: ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true}},
			{Resource: follower, Options: ResourceOptions{ReadOnly: true}},
		}

		results, err := reconcileResources(ctx, reconcileContext, entries, "comp", mapper)

		require.NoError(t, err)
		// The follower is a static MockResource: it produces no status, so
		// results may still be empty. The assertion that matters is that
		// the follower was actually reached.
		_ = results
		follower.AssertCalled(t, "Object")
	})

	t.Run("a missing read-only resource without any absence flag still errors", func(t *testing.T) {
		// Regression guard: the introduction of IgnoreIfAbsent must not change
		// the default behavior of plain ReadOnly on a missing resource.
		missing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "absent-strict-default",
				Namespace: namespace,
			},
		}
		resource := &MockResource{}
		resource.On("Object").Return(missing, nil)
		resource.On("Identity").Return("v1/Secret/absent-strict-default")

		entry := reconcileEntry{
			Resource: resource,
			Options:  ResourceOptions{ReadOnly: true},
		}

		_, err := reconcileResources(ctx, reconcileContext, []reconcileEntry{entry}, "comp", mapper)
		require.Error(t, err)
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/component/ -run TestReconcileResources_IgnoreIfAbsent -v`
Expected: FAIL — the first sub-test fails with an error from the read path (NotFound is not yet handled for the new flag).

- [ ] **Step 3: Extend the NotFound branch**

In `pkg/component/create.go`, locate the block at lines 210-222 inside `reconcileResources`:

```go
		if err != nil {
			if entry.Options.ReadOnly && entry.Options.BlockOnAbsence && apierrors.IsNotFound(err) {
				results = append(results, reconcileResult{
					Entry: entry,
					Status: convergingStatusWithReason{
						Status: convergingStatusGuardBlocked,
						Reason: fmt.Sprintf("waiting for %s", resource.Identity()),
					},
				})
				return results, nil
			}
			return nil, err
		}
```

Replace it with:

```go
		if err != nil {
			if entry.Options.ReadOnly && apierrors.IsNotFound(err) {
				switch {
				case entry.Options.IgnoreIfAbsent:
					continue
				case entry.Options.BlockOnAbsence:
					results = append(results, reconcileResult{
						Entry: entry,
						Status: convergingStatusWithReason{
							Status: convergingStatusGuardBlocked,
							Reason: fmt.Sprintf("waiting for %s", resource.Identity()),
						},
					})
					return results, nil
				}
			}
			return nil, err
		}
```

The `continue` skips both the result-append block below (no status entry) and the `extractResourceData` call (no extractor invocation), matching the silent semantics from the spec.

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./pkg/component/ -run TestReconcileResources_IgnoreIfAbsent -v`
Expected: PASS — all three sub-tests pass.

- [ ] **Step 5: Run all component tests to verify no regression**

Run: `go test ./pkg/component/... -v`
Expected: PASS — including all previously-existing `TestReconcileResources_BlockOnAbsence` sub-tests (the refactored `switch` must not change their behavior).

- [ ] **Step 6: Commit**

```bash
git add pkg/component/create.go pkg/component/create_test.go
git commit -m "$(cat <<'EOF'
silently skip absent read-only resources flagged IgnoreIfAbsent

Extends the NotFound branch in reconcileResources to a switch over the
two absence flags. IgnoreIfAbsent continues the loop without appending
a condition entry or invoking the data extractor; BlockOnAbsence keeps
its existing guard-blocked short-circuit behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Documentation updates

Update the two tables in `docs/component.md` so the new option and the tightened `BlockOnAbsence` semantics are visible to users reading the docs.

**Files:**

- Modify: `docs/component.md:68-75` (the `ResourceOptions` behavior table) and `:100-106` (the builder methods table)

- [ ] **Step 1: Add a new row to the `ResourceOptions` behavior table**

In `docs/component.md`, locate the table that begins around line 68 with the header `| Option | Behavior |`. The last row of that table is the `ReadOnly: true, BlockOnAbsence: true` row. Append one more row immediately after it:

```markdown
| `ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true}`          | **Optional read-only**: a NotFound from the cluster is silently ignored. The entry contributes nothing to the component's conditions, no observation is recorded, and the data extractor is not invoked. Subsequent resources reconcile unchanged. Use for resources that may legitimately be absent (e.g. a referenced Secret owned by another operator)                                                                                                                                                            |
```

- [ ] **Step 2: Update the builder method table**

In the same file, locate the table that begins around line 100 with the header `| Method | Effect |`. Replace the existing `BlockOnAbsence()` row (currently the last one) with:

```markdown
| `BlockOnAbsence()`                | Opts a read-only resource into guard-blocked semantics on NotFound. Requires `ReadOnly()` and is mutually exclusive with `IgnoreIfAbsent()`; `Build()` errors otherwise. Requires a watch on the resource's type to avoid stalling until the periodic resync. |
| `IgnoreIfAbsent()`                | Opts a read-only resource into "optional" semantics: a NotFound is silently ignored, the entry is skipped, no condition or observation is reported, and the data extractor is not invoked. Requires `ReadOnly()` and is mutually exclusive with `BlockOnAbsence()`; `Build()` errors otherwise. |
```

- [ ] **Step 3: Format the markdown**

Run: `make fmt-md`
Expected: no output; the file is reformatted in place to the project's canonical table alignment.

- [ ] **Step 4: Verify the rendered tables look right**

Run: `grep -n "IgnoreIfAbsent" docs/component.md`
Expected: three matches — the new row in the options table, the updated `BlockOnAbsence` row (which references it), and the new `IgnoreIfAbsent()` row in the methods table.

- [ ] **Step 5: Commit**

```bash
git add docs/component.md
git commit -m "$(cat <<'EOF'
document IgnoreIfAbsent and tightened BlockOnAbsence semantics

Adds rows for ReadOnly+IgnoreIfAbsent and the IgnoreIfAbsent() builder
method to docs/component.md. Updates the BlockOnAbsence() method row to
state that it now requires ReadOnly and is mutually exclusive with
IgnoreIfAbsent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Full verification

Run the project's full check suite to catch anything missed (linting, formatting, unrelated test breakage).

- [ ] **Step 1: Run `make all`**

Run: `make all`
Expected: PASS — all tests pass, linting clean, formatting clean.

- [ ] **Step 2: Confirm no E2E changes are needed**

Per the spec, no E2E coverage is required for this change. Confirm by inspecting which directories under `e2e/` reference `ReadOnly`, `BlockOnAbsence`, or `IgnoreIfAbsent`:

Run: `grep -rn -E "BlockOnAbsence|IgnoreIfAbsent" e2e/ || echo "no e2e references"`
Expected: `no e2e references` — confirming the change is fully covered by the unit tests added in Tasks 2 and 3.

- [ ] **Step 3: Push and open a PR**

Only after explicit user instruction. The user reviews the branch first.

---

## Self-Review (already performed inline)

**Spec coverage:**

- Builder API (`IgnoreIfAbsent()` method, GoDoc) → Tasks 1, 2.
- `ResourceOptions.IgnoreIfAbsent` field → Task 1.
- Build-time validation (three error cases) → Task 2.
- Runtime behavior (silent skip, continue, no extractor) → Task 3.
- `BlockOnAbsence()` GoDoc update → Task 2.
- Builder method GoDoc on `IgnoreIfAbsent()` → Task 1.
- `docs/component.md` table updates → Task 4.
- Reconciliation tests mirroring `BlockOnAbsence` → Task 3.
- Builder validation tests → Task 2.
- Inert-when-deleted flag preservation → covered by the `"ignore if absent preserved when deletion forced"` table case in Task 1 and the `"BlockOnAbsence + deleted"` documentary case in Task 2.

**Placeholder scan:** No TBDs, no "appropriate error handling", no "similar to Task N" shortcuts. Every code change is shown in full.

**Type consistency:** Field name `IgnoreIfAbsent` used consistently across struct, builder field (`ignoreIfAbsent`), method, GoDoc, tests, docs, and commit messages. Method name `IgnoreIfAbsent()` used throughout. `BlockOnAbsence` reference everywhere unchanged.
