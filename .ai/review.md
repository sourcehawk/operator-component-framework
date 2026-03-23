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

## Existing pull request comments

When doing a review, check the comments and discussions in the pull request.

- Something already addressed in a previous comment should not be repeated in a new review unless it's a critical bug, or you are certain the outcome of the discussion did not properly address the issue.

- Before raising a new comment, scan existing threads to check if the same issue has already been flagged. If it has, do not open a duplicate thread.

- If a comment was explicitly marked as non-actionable — for example, the author replied with "won't fix", "by design", "out of scope", or a reviewer resolved it without a code change — treat it as a closed decision and do not re-raise it. Respect the outcome of the discussion even if you would have decided differently.

- If a thread is still open and unresolved, you may reference it rather than opening a new comment ("This ties into the open discussion in [thread]"), but do not pile on with a redundant point.

- If you believe a closed discussion reached the wrong conclusion and the issue is significant enough to re-raise, clearly acknowledge the prior decision and explain specifically why you think it needs revisiting. Do not silently re-raise it as if the prior discussion never happened.

- Do not comment on style, formatting, or naming choices that were already discussed and accepted as-is by a reviewer or the author.

## Reviewing completely in a single pass

Aim to be exhaustive in a single review. Do not hold back comments with the intent of raising them in a later round — if something is worth flagging, flag it now.

- When asked to review, go through the entire diff thoroughly before posting any comments. Do not do a shallow pass and leave obvious issues unraised because you were focused on one area.

- Do not artificially limit the number of comments in a review. If there are 15 things worth flagging, raise all 15. A thorough first review is less disruptive than multiple thin rounds.

- Do not save "minor" or "low priority" comments for later rounds. Include them in the first review, clearly labeled by severity if needed (e.g. nit, suggestion, blocking), so the author can address everything in one go.

- After the author addresses your comments and requests a new review, focus only on what changed and any issues introduced by those changes. Do not use re-reviews as an opportunity to raise issues you could have caught in the first pass.

- If you are uncertain whether something is an issue, first do a quick verification pass (e.g. read surrounding code, search for similar patterns, or check existing docs). Only leave a comment when you can point to a specific, evidenced concern, or clearly frame it as a question when you cannot confirm the intent.