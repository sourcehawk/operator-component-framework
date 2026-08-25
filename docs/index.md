# Operator Component Framework

A Go framework for building Kubernetes operators that stay maintainable as they grow.

## Start here

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

- :material-chart-line: **[Observability](observability.md)**

    Grafana dashboards and Prometheus alerts for your operator.

</div>

## Why this exists

A Kubernetes operator does far more than create resources. For every resource it manages, a controller has to construct
the desired object, apply it without overwriting fields it does not own, decide whether the resource is healthy, fold
that health into a status condition on the owner, and adapt behavior to feature flags and the application versions it
supports. Written by hand, this logic collects in the reconciler until it is large, repetitive, and hard to test, and
the part you actually care about, what your operator does, is buried under mechanics that every operator repeats.

This framework gives you two reusable layers, **components** and **resource primitives**, that sit between your
reconciler and the Kubernetes objects it manages. You declare the desired state of each resource and the behavior that
varies by flag or version. The framework handles server-side apply, per-resource health, aggregation into a single owner
condition without update conflicts, lifecycle (grace periods, suspension, prerequisites, guards), and feature gating.
Controllers stay thin, version-specific behavior lives in small named mutations you can test in isolation, and you keep
full control where it matters.

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
