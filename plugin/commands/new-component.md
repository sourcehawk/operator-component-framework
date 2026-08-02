---
description: Scaffold a new operator-component-framework component in this operator
argument-hint: [component name]
---

Scaffold a new component in this operator project. Component name (may be empty): $ARGUMENTS

First invoke the `ocf:building-components` skill and follow it. Then:

1. Confirm this project uses the framework: `go.mod` must require `github.com/sourcehawk/operator-component-framework`.
   If it does not, stop and say this command is for operators built on that framework.
2. Study one existing component in this repository (search for the framework's component builder usage) and match its
   file layout, naming, and registration wiring. Consistency with the existing operator beats any generic template.
3. Gather what you need before writing code. Ask the user, one question at a time, anything you cannot infer:
   - What does the component manage, and what is the logical condition it owns? (One component per logical condition.)
   - The condition type name as it should appear on the owner's status.
   - Which resource primitives it reconciles, in dependency order (registration order is execution order).
   - Participation mode, and whether it is gated behind a feature gate or has prerequisites on other components.
4. Scaffold the component:
   - The component constructor with the builder, condition type, and resources registered in dependency order.
   - Baseline desired-state functions that hold only version-independent fields; version-dependent or optional fields go
     in named mutations.
   - Registration in the controller alongside the existing components.
5. Write tests per the `ocf:testing-operators` skill: mutation unit tests for any mutations you added, and a golden
   snapshot for the component's rendered output.
6. Verify exact framework signatures with `go doc github.com/sourcehawk/operator-component-framework/pkg/component`
   before finalizing; do not invent builder methods.
7. Build and run the project's tests. Report exactly what was created and what the user still needs to fill in.
