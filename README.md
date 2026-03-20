# Operator Component Framework

A Go framework for building highly maintainable Kubernetes operators using a behavioral component model and version-gated feature mutations.

## Introduction

The Operator Component Framework is a structured approach to operator development that simplifies the management of complex resource lifecycles and evolving feature sets. It provides:

*   **Behavioral Component Model**: Group related resources into logical features with aggregated health and shared lifecycle behavior.
*   **Structured Reconciliation**: Predictable resource management with built-in support for progression, degradation, and suspension.
*   **Version-Gated Feature Mutations**: Define a clean baseline for resources and apply optional behavior or version-specific compatibility as explicit, composable mutations.
*   **Reusable Kubernetes Resource Primitives**: Powerful abstractions for managing Kubernetes objects with built-in mutation safety.

## Why this framework exists

Kubernetes operators often grow into bloated controllers with:

- **Large reconciliation functions** coordinating many resources.
- **Inconsistent lifecycle logic** (rollouts, suspension, degradation) implemented repeatedly.
- **Scattered status reporting** that varies across different features.
- **Complex version compatibility** logic mixed with orchestration.

The framework addresses these problems through:

- **Components**: Manage logical features as a single unit.
- **Reusable Primitives**: Encapsulate the desired state and behavior of individual Kubernetes objects.
- **Mutation-based Customization**: Decouple version and feature logic from the baseline resource definition.

## Mental Model

The framework uses a hierarchy of responsibility to maintain thin controllers and consistent behavior:

```text
Controller
  └─ Component
      └─ Resource Primitive
           └─ Kubernetes Object
```

*   **Controller**: Decides which components should exist and orchestrates reconciliation at a high level.
*   **Component**: Represents one logical feature (e.g., "Web Interface"), reconciles its resources, and reports a single user-facing condition.
*   **Resource Primitive**: Encapsulates the desired state and lifecycle behavior of a single Kubernetes object.
*   **Kubernetes Object**: The raw `client.Object` (e.g., a `Deployment`) persisted to the cluster.

## Quick Start Example

A minimal example showing how to build and reconcile a component with a single resource primitive:

```go
// 1. Create a resource primitive (using a builder)
deployment := resources.NewCoreDeployment("web-server", owner.Namespace)
res, err := resources.NewDeploymentBuilder(deployment).Build()
if err != nil {
    return err
}

// 2. Build a component
comp, err := component.NewComponentBuilder().
    Suspend(owner.Spec.Suspended).
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(res, component.ResourceOptions{}).
    WithGracePeriod(5 * time.Minute).
    Build()
if err != nil {
    return err
}

// 3. Reconcile the component
recCtx := component.ReconcileContext{
    Client:   r.Client,
    Scheme:   r.Scheme,
    Recorder: r.Recorder,
    Owner:    owner,
}
err = comp.Reconcile(ctx, recCtx)
```

## Architecture Overview

The framework is divided into two main subsystems:

### Component Layer
Responsible for high-level feature orchestration, lifecycle management, and condition aggregation. It ensures that features behave consistently across the operator.

Detailed documentation: [Component Framework](docs/component.md)

### Primitive Layer
Responsible for Kubernetes resource abstractions, the mutation system, and safe field application. It handles the low-level details of how objects are constructed and modified.

Detailed documentation: [Resource Primitives](docs/primitives.md)

## Feature Mutations (high-level)

Feature mutations allow you to evolve resources safely as features and versions accumulate:

1.  **Baseline Desired State**: The core resource defines the current, modern desired state.
2.  **Optional Version-Gated Mutations**: Explicit modifications are applied only when specific version constraints or conditions are met.
3.  **Composable Customization**: Multiple mutations can be layered onto a single resource without conflicting or producing inconsistent results.

This approach keeps your code focused on the present state while explicitly managing its history and optional behaviors.

## Custom Resources

Users can implement their own resource wrappers by fulfilling the `Resource` interface. This is appropriate when:
- An object has unusual lifecycle behavior.
- You are managing custom CRDs with specialized status or readiness logic.
- You need specialized mutation semantics not covered by the built-in primitives.

For examples of how to implement custom resource wrappers, see the [examples directory](examples/).

## Examples

The [examples directory](examples/component-architecture-basics/) provides complete implementations demonstrating:
- **Custom resource implementation**: How to wrap standard or custom Kubernetes objects.
- **Component assembly**: How to group resources into functional units.
- **Feature mutation usage**: How to apply version-gated logic to resources.