# AI Instructions — Operator Component Framework

## Project Context

This is a **professional open source Go framework** for building Kubernetes operators. Public APIs, documentation, and
examples are a published contract consumed by other teams. Every change must meet the standard of a well-maintained open
source library.

Module: `github.com/sourcehawk/operator-component-framework`

---

## Before Making Changes

**Read before writing.** Always gather context from the actual source code and documentation before proposing or making
changes. Do not reason from assumptions.

### Documentation to read

Understand the intended design first:

- `README.md` — architecture, mental model, quick start
- `docs/component.md` — component lifecycle, status model, reconciliation phases
- `docs/primitives.md` — primitive categories, field application, mutation system, editors, selectors
- `docs/primitives/*.md` — primitive implementations

### Source to read

Verify the real API before using or documenting it. Key packages:

- `pkg/component/` — builder, reconciliation, condition types, participation modes
- `pkg/component/concepts/` — lifecycle interfaces and their exact status type constants
- `pkg/primitives/` — kubernetes primitive resource wrappers with builders and mutators
- `pkg/mutation/editors/` — available methods per editor type
- `pkg/mutation/selectors/` — available container selectors
- `pkg/feature/feature.go` — `NewResourceFeature`, `Mutation[T]`

When changing a public API, also check `examples/` for real usage patterns and to identify what else needs updating.

---

## Architecture

```
Controller
  └─ Component              pkg/component
      └─ Resource Primitive  pkg/primitives/*
           └─ Kubernetes Object
```

Changes at lower layers (primitives, editors, feature gating) ripple upward. Always trace the full impact before
stopping.

---

## Rules for Code Changes

### Clarify before implementing

If a prompt is insufficiently detailed to make a coherent and well-designed change — ambiguous scope, unclear intent, or
missing context that would materially affect the approach — ask targeted followup questions before writing any code. A
precise implementation of the wrong thing is worse than a short delay to align on the right one.

### GoDoc

Every exported symbol has a GoDoc comment. Update it whenever you change the associated behaviour, signature, or
semantics. GoDoc is part of the public API surface.

### Documentation

Update documentation in the **same response** as the code change — never leave them out of sync.

| Code area changed                                 | Documentation to update |
| ------------------------------------------------- | ----------------------- |
| Component builder, reconciliation, status model   | `docs/component.md`     |
| Primitives, field application, editors, selectors | `docs/primitives.md`    |
| Primitive implementations                         | `docs/primitives/*.md`  |
| Any `pkg/` export visible in the quick start      | `README.md`             |
| Examples                                          | `examples/*/README.md`  |

When updating documentation in markdown files, make sure to run `make fmt-md` for consistent formatting.

### Examples

If you change a method signature, type name, or behaviour in `pkg/`, search `examples/` for usages and update them.

Ensure examples still build and run as expected after editing them using:

```bash
make build-examples
```

or to run them

```bash
make run-examples
```

or use a more targeted `go run` command for a specific example.

### Tests

The project uses Ginkgo/Gomega and testify (assert,require,mock).

Do not use things like `t.Fatal` in tests, they should use asserts and require instead.

Run with:

```bash
go test ./...
```

To verify linting and formatting along with tests after making code changes use:

```bash
make all
```

**Tests encode intent, not implementation.** A test must assert what the code is _supposed to do_, not simply mirror
what the code currently does. Never write or modify a test purely to make it pass — a failing test is evidence of a bug
or a broken assumption, not a test to be fixed.

**Prefer black-box tests.** Test behaviour through the public API by asserting inputs and outputs, without coupling
tests to internal implementation details. White-box tests are appropriate when internal state must be verified and
cannot be adequately observed through the public surface.

**Write new tests for new code paths.** When adding behaviour, write tests that cover:

- The primary success path
- All meaningful failure and error paths
- Boundary and edge cases specific to the logic being added

Updating an existing test is appropriate when the intended behaviour genuinely changed. If a test fails after a change,
first determine whether the code or the test is wrong:

- If the intention behind the code change is clear and the test expectation is now stale, update the test.
- If the intention is unclear — that is, it is ambiguous whether the failing test reflects a regression or an intended
  change — read the implementation, GoDoc, surrounding code, and git history to infer it. If the inferred intention
  reveals that a method signature is imprecise or misleading, update the signature to express the intention correctly.
  If the intention remains genuinely ambiguous after analysis, **stop and ask** before touching either the code or the
  test. Do not guess.

---

## Documentation Standards

- **Accurate** — verify every method name, field name, and import path against the source before writing examples.
- **Complete** — document edge cases and non-obvious defaults alongside the main behaviour.
- **Professional** — direct, precise prose. No filler phrases or vague hand-waving.
- **Consistent** — use the exact terminology from the source code: interface names, method names, status constant
  values.

---

## Code Review

When reviewing pull requests, apply the standards in
[`.github/copilot-review-guidelines.md`](copilot-review-guidelines.md).
