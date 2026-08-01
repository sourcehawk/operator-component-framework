---
description: Look up operator-component-framework documentation for a topic
argument-hint: <topic>
---

Look up operator-component-framework documentation for: $ARGUMENTS

Resolution order:

1. Search the bundled references first. They live under `${CLAUDE_PLUGIN_ROOT}/skills/*/references/`:
   - `building-components/references/component.md`: component builder, lifecycle, status model, guards
   - `using-primitives/references/primitives.md`: primitive concepts, mutation system, editors, selectors
   - `using-primitives/references/primitives/<kind>.md`: per-kind primitive builders and mutators
   - `custom-resource-wrappers/references/custom-resource.md`: wrapping CRD-backed types with pkg/generic
   - `structuring-operators/references/guidelines.md`: operator structuring best practices
   - `structuring-operators/references/compatibility.md`: supported version policy
   - `testing-operators/references/testing.md`: mutation tests, golden snapshots, goldengen
2. If the topic is not covered there, fetch the published documentation at
   https://sourcehawk.github.io/operator-component-framework/
3. For exact signatures, verify against the version the consumer actually uses: read `go.mod` for the
   `github.com/sourcehawk/operator-component-framework` version, then run
   `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

Answer the question directly, citing which reference file or URL each claim came from. If the bundled references and
`go doc` disagree, trust `go doc` and say the plugin docs may lag the consumer's framework version.
