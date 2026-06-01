# Operator Component Framework

A Go framework for building Kubernetes operators that stay maintainable as they grow. It pulls reconciliation mechanics,
status reporting, and lifecycle behavior into reusable building blocks (**components** and **resource primitives**), so
your controllers stay thin and focused on construction and orchestration.

!!! note

    This framework is not a replacement for
    [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime). It is a library you use inside
    controller-runtime reconcilers (such as Kubebuilder-generated projects) to manage the layers between the reconciler
    and the Kubernetes resources it manages.

## Start here

<div class="grid cards" markdown>

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
