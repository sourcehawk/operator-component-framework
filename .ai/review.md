# Code Review Guidelines — Operator Component Framework

When reviewing code, apply the standards of a professional open source Go library. The goal is to surface anything that would be a quality or clarity problem once published — not to enforce style preferences.

## API clarity

- Exported names must be unambiguous in isolation, without reading the implementation. If a name requires a comment to interpret, the name is wrong.
- Prefer types that encode constraints over `bool` parameters, stringly-typed arguments, or functions with multiple parameters of the same type — these make call sites confusing and misuse easy.
- Any exported function that can meaningfully fail must return an `error`. Silent failure is not acceptable in a library.
- Interfaces should be as small as necessary. Flag any interface that bundles unrelated behaviour or that imposes unnecessary implementation burden.

## GoDoc quality

- Every exported symbol must have a GoDoc comment beginning with the symbol name.
- Comments must describe behaviour and contracts, not restate the name. `// SetReplicas sets replicas.` adds nothing.
- Preconditions, postconditions, and non-obvious nil/zero behaviour must be documented where they exist.
- Silent no-op conditions or surprising defaults must be explicitly called out in the comment.

## Error quality

- Error messages must be specific and actionable. Flag generic messages such as `"operation failed"` or `"invalid input"`.
- Wrapped errors must add context that is not already present in the wrapped message.
- Errors must not be silently discarded, even in cleanup paths.

## Naming and clarity

- Flag vague names where a more specific alternative exists: `data`, `info`, `result`, `obj`, `flag`, `temp`.
- Flag unexported identifiers whose names require reading the implementation to understand.
- Magic literals must be named constants.

## Complexity

- Flag functions that are doing more than one thing. A function that requires significant scrolling to read is a signal to decompose.
- Flag abstractions with a single caller and no credible extension point — premature generality is a maintenance cost.
- Flag unnecessary indirection: wrappers that add no behaviour, interfaces with a single permanent implementation.

## Consistency

- New code must follow the patterns already established in the package — builder pattern, error handling style, naming conventions. Inconsistency is a quality issue even when the code is technically correct.

## Test quality

- New behaviour without tests must be flagged.
- Test descriptions must clearly state what is tested and under what condition. Flag `TestFoo` or `TestCase1`.
- Flag tests that only cover the happy path when meaningful failure cases or edge cases exist.

## Documentation drift

- If a code change affects behaviour described in `docs/`, `README.md`, or GoDoc, and that documentation was not updated, flag it.
- If a code change affects an example in `examples/`, and the example was not updated, flag it.