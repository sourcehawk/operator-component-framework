# Operator Component Framework

A Go framework for building Kubernetes operators that stay maintainable as they grow.

## Why this exists

Operators tend to accumulate the same problems. Status conditions are assembled by hand, and aggregating them into a
single owner condition without provoking update conflicts is fiddly to get right. Reconcilers grow into fat,
hard-to-test functions that mix construction, ordering, health checks, and status writes. Version-gating logic ends up
scattered through the reconcile path as conditionals that are easy to break and hard to review.

This framework moves that work into two reusable layers, **components** and **resource primitives**, that sit between
your reconciler and the Kubernetes objects it manages. Reconciliation mechanics, health aggregation, and feature gating
live in the framework, so controllers stay thin and the version-specific behavior lives in named, testable mutations.

## Key features

**Reconciliation and status**

- Resource primitives report health in a way that fits their category, and the component aggregates them into one owner
  condition with a single status write.
- Grace periods give resources time to converge before a component reports degraded or down.

**Feature and version management**

- Mutations apply patches only when a flag is set or a version constraint matches, keeping the baseline object clean.
- Feature gates enable or disable an entire component, or an individual resource within one, based on flags or version
  ranges.

**Orchestration**

- Guards block a resource and everything after it until a precondition is met.
- Data extraction harvests values from one resource for guards and mutations on later ones.
- Prerequisites express startup ordering between components.

## A taste

A component composes resource primitives into one reconcilable unit with a single owner condition. The reconciler builds
it and hands it to the framework.

```go
comp, err := component.NewComponentBuilder().
    WithName("example-app").
    WithConditionType("AppReady").
    WithResource(deployResource).
    WithResource(cmResource, component.DeleteWhen(!owner.Spec.EnableMetrics)).
    Suspend(owner.Spec.Suspended).
    Build()
if err != nil {
    return err
}

return comp.Reconcile(ctx, recCtx)
```

[Getting Started](getting-started.md) walks through building `deployResource` and `cmResource` and wiring the reconcile
loop end to end.

## Where to go next

New to the framework? Start with **Getting Started**. Already building and looking for patterns? Read **Guidelines**.

<div class="grid cards" markdown>

<!-- prettier-ignore -->
- :material-rocket-launch-outline: **[Getting Started](getting-started.md)**

    Build your first component step by step.

- :material-cube-outline: **[Component](component.md)**

    Lifecycle, status model, and reconciliation phases.

- :material-shape-outline: **[Primitives](primitives.md)**

    Typed wrappers over Kubernetes resources with builders, mutators, and feature gating.

- :material-source-branch: **[Custom Resources](custom-resource.md)**

    Build custom resource wrappers with `pkg/generic`.

- :material-book-open-variant: **[Guidelines](guidelines.md)**

    Patterns for structuring operators well.

- :material-test-tube: **[Testing](testing.md)**

    Golden snapshots and version-matrix golden generation.

</div>
