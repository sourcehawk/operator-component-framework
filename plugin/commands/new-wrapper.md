---
description: Scaffold an operator-component-framework primitive wrapper for a custom resource
argument-hint: [kind]
---

Scaffold a custom resource wrapper (a framework primitive for a CRD-backed type). Target kind (may be empty): $ARGUMENTS

First invoke the `ocf:custom-resource-wrappers` skill and follow it. Then:

1. Confirm this project uses the framework: `go.mod` must require `github.com/sourcehawk/operator-component-framework`.
   If it does not, stop and say this command is for operators built on that framework.
2. Check the target kind is not already covered by a built-in primitive (see the `ocf:using-primitives` skill's built-in
   list). If it is, use the built-in primitive instead and say so.
3. Ask whether a full wrapper is warranted. If the operator only needs to apply the resource without typed mutations or
   status interpretation, the unstructured primitive is the lighter answer; recommend it and stop unless the user
   confirms they need typed mutations or status handling.
4. Gather what you need, one question at a time, for anything you cannot infer:
   - The Go type and import path of the custom resource.
   - The resource category (this decides which status handlers the wrapper implements).
   - Namespaced or cluster-scoped.
5. Generate the package with the framework CLI rather than writing it by hand:
   `ocf scaffold wrapper --type <import-path>.<TypeName> --variant <category> --group <api-group>`, adding
   `--cluster-scoped` for a cluster-scoped kind. Install it first if it is missing
   (`go install github.com/sourcehawk/operator-component-framework/cmd/ocf@latest`), and run `go mod tidy` afterwards so
   the wrapped type resolves. If the CLI cannot be installed, or the package already exists and is being extended,
   implement the wrapper by hand following the eight steps in the skill, in order: category, mutation type alias,
   mutator, status handlers, builder, resource, feature mutations, component registration. Match the layout of any
   existing wrapper in this repository if one exists.
6. Replace the scaffolded defaults with kind-specific logic: the status handlers report healthy, completed, or
   operational unconditionally, and the suspension handlers are no-ops. Each carries a comment saying so.
7. Verify exact framework signatures with `go doc github.com/sourcehawk/operator-component-framework/pkg/generic` before
   finalizing; do not invent interfaces.
8. Write tests per the `ocf:testing-operators` skill, build, run the project's tests, and report what was created.
