# Operator Architecture Example

This directory contains an example of the architecture used in an operator.
It demonstrates the two primary patterns: the **Component Framework** and **Feature Mutations**.

## Running the Example

You can run this example directly using the Go toolchain:

```bash
go run examples/component-architecture-basics/main.go
```

The example will simulate four different reconciliation scenarios:
1.  **Legacy Version (7.9.0)**: Demonstrates "Cleanup" and "Legacy Compatibility" features.
2.  **Modern Version (8.1.0)**: Demonstrates "Tracing" and "Legacy Compatibility" features.
3.  **Future Version (9.0.0)**: Demonstrates "Tracing" enabled and "Legacy Compatibility" disabled.
4.  **Suspension**: Demonstrates how the framework handles scaling down/suspending a component.

Example output:
```bash
$ go run docs/design/examples/component-architecture/main.go
=== Scenario 1: Reconciling Legacy Version 7.9.0 ===
2026-03-08T23:54:26Z    INFO    Deployment State        {"name": "example-legacy-web-ui"}
2026-03-08T23:54:26Z    INFO      Replicas      {"count": 1}
2026-03-08T23:54:26Z    INFO      Labels        {"labels": {"app.kubernetes.io/name":"example-legacy-web-ui","managed-by":"example-operator"}}
2026-03-08T23:54:26Z    INFO      Container     {"name": "app"}
2026-03-08T23:54:26Z    INFO        Args        {"args": ["--legacy-mode"]}
2026-03-08T23:54:26Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"DEPRECATED_SETTING","value":"legacy-value"}]}
Owner: example-legacy (Version: 7.9.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 2: Reconciling Modern Version 8.1.0 ===
2026-03-08T23:54:26Z    INFO    Deployment State        {"name": "example-modern-web-ui"}
2026-03-08T23:54:26Z    INFO      Replicas      {"count": 1}
2026-03-08T23:54:26Z    INFO      Labels        {"labels": {"app.kubernetes.io/name":"example-modern-web-ui","managed-by":"example-operator"}}
2026-03-08T23:54:26Z    INFO      Container     {"name": "app"}
2026-03-08T23:54:26Z    INFO        Args        {"args": ["--legacy-mode"]}
2026-03-08T23:54:26Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"NEW_MANDATORY_SETTING","value":"standard-value"}]}
Owner: example-modern (Version: 8.1.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 3: Reconciling Future Version 9.0.0 ===
2026-03-08T23:54:26Z    INFO    Deployment State        {"name": "example-future-web-ui"}
2026-03-08T23:54:26Z    INFO      Replicas      {"count": 1}
2026-03-08T23:54:26Z    INFO      Labels        {"labels": {"app.kubernetes.io/name":"example-future-web-ui","managed-by":"example-operator"}}
2026-03-08T23:54:26Z    INFO      Container     {"name": "app"}
2026-03-08T23:54:26Z    INFO        Args        {"args": []}
2026-03-08T23:54:26Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"NEW_MANDATORY_SETTING","value":"standard-value"}]}
Owner: example-future (Version: 9.0.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 4: Suspending the Component ===
Owner: example-future (Version: 9.0.0, Suspended: true)
  Condition WebInterfaceReady: True (Reason: Suspended, Message: All resources are suspended.)
```

## Directory Structure

- `resources/`: Custom resource wrappers for Kubernetes objects.
    - `deployment_resource.go`: A wrapper around a real `*appsv1.Deployment` implementing the framework's `Resource`, `Alive`, and `Suspendable` interfaces.
    - `deployment_mutator.go`: Example-specific restricted mutation interface for Deployments.
    - `deployment_builder.go`: Configurable builder for the `DeploymentResource`.
- `features/`: Version-aware feature gates and mutations using the framework's `feature.Mutation` type.
    - `constraints.go`: Example implementation of the `feature.VersionConstraint` interface using semver.
    - `tracing_feature.go`: Example feature: adds tracing configuration (version-gated).
    - `legacy_compat_feature.go`: Example feature: adds legacy compatibility flags (version-gated).
    - `compatibility_cleanup_feature.go`: Example feature: removes/replaces deprecated settings for older versions.
- `exampleapp/`: A mock application demonstrating the framework usage.
    - `owner.go`: A mock Custom Resource that implements the real `component.OperatorCRD` interface.
    - `controller_example.go`: A mock controller showing how to use the `component.Builder` and `ReconcileContext`.
- `main.go`: Entry point that demonstrates the assembly and usage of the components.

## Key Concepts

### Component Framework
The framework (`pkg/component`) groups related resources into a single logical unit. It centralizes:
- **Reconciliation Flow**: Standardized apply/delete/status logic.
- **Status Aggregation**: Computing a single "Ready" condition from multiple resources.
- **Lifecycle Management**: Handling suspension and "alive" checks consistently.

### Feature Mutations
Instead of using complex `if/else` logic inside resource configuration, we use **Feature Mutations**:
- **Baseline**: A core, stable configuration for the resource (applied in `SetImmutable` and `SetMutable`).
- **Mutations**: Small, focused functions that modify the resource if a feature is enabled.
- **Mutator Interface**: A restricted set of methods (`DeploymentResourceMutator`) that mutations use to interact with the resource, preventing them from making arbitrary or destructive changes.
- **Registration**: Mutations are registered through a builder and applied during the resource's `SetMutable` call.
