# Custom resource implementation

This directory contains an example of a custom implementation using the component framework concepts.

## Running the Example

You can run this example directly using the Go toolchain:

```bash
go run examples/custom-resource-implementation/main.go
```

The example will simulate four different reconciliation scenarios:
1.  **Legacy Version (7.9.0)**: Demonstrates "Cleanup" and "Legacy Compatibility" features.
2.  **Modern Version (8.1.0)**: Demonstrates "Tracing" and "Legacy Compatibility" features.
3.  **Future Version (9.0.0)**: Demonstrates "Tracing" enabled and "Legacy Compatibility" disabled.
4.  **Suspension**: Demonstrates how the framework handles scaling down/suspending a component.

Example output:
```bash
$ go run examples/component-architecture-basics/main.go
=== Scenario 1: Reconciling Legacy Version 7.9.0 ===
2026-03-09T05:32:43Z    INFO    Deployment State        {"name": "example-legacy-web-ui"}
2026-03-09T05:32:43Z    INFO      Replicas      {"count": 1}
2026-03-09T05:32:43Z    INFO      Labels        {"labels": {"app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-legacy-web-ui"}}
2026-03-09T05:32:43Z    INFO      Pod Labels    {"labels": {"app":"example-legacy-web-ui","app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-legacy-web-ui"}}
2026-03-09T05:32:43Z    INFO      Container     {"name": "app"}
2026-03-09T05:32:43Z    INFO        Args        {"args": ["--legacy-mode"]}
2026-03-09T05:32:43Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"DEPRECATED_SETTING","value":"legacy-value"}]}
Owner: example-legacy (Version: 7.9.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 2: Reconciling Modern Version 8.1.0 ===
2026-03-09T05:32:43Z    INFO    Deployment State        {"name": "example-modern-web-ui"}
2026-03-09T05:32:43Z    INFO      Replicas      {"count": 1}
2026-03-09T05:32:43Z    INFO      Labels        {"labels": {"app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-modern-web-ui"}}
2026-03-09T05:32:43Z    INFO      Pod Labels    {"labels": {"app":"example-modern-web-ui","app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-modern-web-ui"}}
2026-03-09T05:32:43Z    INFO      Container     {"name": "app"}
2026-03-09T05:32:43Z    INFO        Args        {"args": ["--legacy-mode"]}
2026-03-09T05:32:43Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"NEW_MANDATORY_SETTING","value":"standard-value"}]}
Owner: example-modern (Version: 8.1.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 3: Reconciling Latest Version 9.0.0 ===
2026-03-09T05:32:43Z    INFO    Deployment State        {"name": "example-latest-web-ui"}
2026-03-09T05:32:43Z    INFO      Replicas      {"count": 1}
2026-03-09T05:32:43Z    INFO      Labels        {"labels": {"app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-latest-web-ui"}}
2026-03-09T05:32:43Z    INFO      Pod Labels    {"labels": {"app":"example-latest-web-ui","app.kubernetes.io/managed-by":"example-operator","app.kubernetes.io/name":"example-latest-web-ui"}}
2026-03-09T05:32:43Z    INFO      Container     {"name": "app"}
2026-03-09T05:32:43Z    INFO        Args        {"args": []}
2026-03-09T05:32:43Z    INFO        Env {"env": [{"name":"LOG_LEVEL","value":"info"},{"name":"NEW_MANDATORY_SETTING","value":"standard-value"}]}
Owner: example-latest (Version: 9.0.0, Suspended: false)
  Condition WebInterfaceReady: False (Reason: Creating, Message: Waiting for replicas: 0/1 ready)

=== Scenario 4: Suspending the Component ===
Owner: example-latest (Version: 9.0.0, Suspended: true)
  Condition WebInterfaceReady: True (Reason: Suspended, Message: All resources are suspended.)
```

## Directory Structure

- `resources/`: Custom resource wrappers for Kubernetes objects.
    - `deployment_resource.go`: Implements the framework's `Resource`, `Alive`, `Suspendable`, and `DataExtractable` interfaces for `*appsv1.Deployment`.
    - `deployment_mutator.go`: A "Planner-style" mutator for `*appsv1.Deployment` objects, allowing features to record intent without direct object manipulation.
    - `deployment_builder.go`: Configurable builder for the `DeploymentResource`.
    - `deployment_construction.go`: Factory for the core Deployment baseline configuration.
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
- **Baseline**: A core, stable configuration for the resource (applied in `Mutate(current client.Object)`).
- **Mutations**: Small, focused functions that modify the resource if a feature is enabled.
- **Planner-style Mutator**: A restricted set of methods (`DeploymentResourceMutator`) that mutations use to record their *intent*. This avoids direct manipulation of the Kubernetes object by features and allows the mutator to resolve changes (like slice updates) efficiently in a single final pass.
- **Registration**: Mutations are registered through a builder and applied during the resource's `Mutate(current client.Object)` call.
- **Application Process**: The `Mutate` implementation follows a precise sequence:
    1.  **Core Baseline**: Apply all desired fields from the core resource baseline to the `current` server object.
    2.  **Feature Mutations**: Apply all registered feature mutations to the `current` object, allowing them to override or extend the baseline.
    3.  **State Sync**: Update the internal `desired` state with the fully mutated `current` object. This ensures that subsequent calls to `ConvergingStatus` and `ExtractData` operate on the final, augmented state of the resource.

### Resource Interface
The `Resource` interface in `pkg/component` is the central contract for all objects managed by the framework:
- `Identity() string`: Returns a unique identifier for the resource (e.g., `apps/v1/Deployment/web-ui`).
- `Object() (client.Object, error)`: Returns a fresh copy of the baseline resource object before reconciliation. Returns the applied k8s object after reconciling.
- `Mutate(current client.Object) error`: Applies the core baseline, feature mutation intents, and any deferred mutations (like suspension) to the provided `current` server object. It also synchronizes the internal state to reflect these changes.
