# Mutations and Gating

This example demonstrates how to use **version-gated** and **boolean-gated** mutations within a single component that
manages two resources: a Deployment and a ConfigMap.

## What it shows

- **Baseline as latest version**: The base Deployment represents the v2 layout (container named "app" with HTTP and
  health ports). This follows the guideline that the baseline always reflects the latest desired state.
- **Version-gated backward compat mutation**: `BackwardCompatV1Container` activates for versions `< 2.0.0` and rolls the
  baseline back to the v1 layout (container named "server", HTTP port only). Uses a `semver.Constraint` as a
  `feature.VersionConstraint`. The `BackwardCompat` prefix makes the pattern immediately recognizable.
- **Boolean-gated mutation**: `TracingSidecarMutation` injects a Jaeger sidecar. It is gated via `.When(enabled)`, so
  the sidecar is added only when tracing is on and removed when it is off.
- **Mutation ordering for container name stability**: `DebugLoggingMutation` targets `ContainerNamed("app")` and is
  registered before `BackwardCompatV1Container`. This ensures it always sees the baseline name, even though the backward
  compat mutation renames the container for older versions. The env var edit carries through the rename because the
  backward compat mutation only overwrites `Name` and `Ports`, not `Env`.
- **Resource-level gating**: The ConfigMap is registered with `component.DeleteWhen(!owner.Spec.EnableMetrics)`. When
  metrics are disabled, the framework deletes the ConfigMap entirely rather than leaving an empty one behind.
- **Mutation-level gating within a resource**: `MetricsConfigMutation` on the ConfigMap is separately boolean-gated,
  showing that gating can happen at both the resource and mutation levels.
- **Suspension**: The component supports suspension via `Suspend(owner.Spec.Suspended)`.

## Reconciliation steps

1. v1.9.0 with tracing and metrics: backward compat container layout ("server", single port).
2. Enable debug logging on v1: LOG_LEVEL=debug targets "app" (baseline name) and carries through the backward compat
   rename.
3. Upgrade to v2.0.0: container switches to "app" with a health port added, debug logging still active.
4. Disable debug logging and tracing: sidecar removed, LOG_LEVEL removed from the Deployment.
5. Disable metrics: the entire ConfigMap is deleted via resource-level gating.
6. Suspend the application.

## Running

```bash
go run ./examples/mutations-and-gating/.
```
