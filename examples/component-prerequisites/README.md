# Component Prerequisites

This example demonstrates how to use **component-level prerequisites** with `DependsOn` to order components within a
controller.

## What it shows

- **Two components, one controller**: The "infra" component manages a ConfigMap and reports `InfraReady`. The "app"
  component manages a Deployment and reports `AppReady`.
- **DependsOn prerequisite**: The app component uses `WithPrerequisite(component.DependsOn("InfraReady"))` to block
  until the infra component's condition is `True`.
- **Permanent pass**: Once a prerequisite is satisfied, it is never re-evaluated. Subsequent reconciles proceed without
  checking it again.
- **Sequential reconciliation**: The controller reconciles infra first, then app. The owner's in-memory status is
  updated between the two calls, so `DependsOn` can read the first component's condition.

## Reconciliation steps

1. Full reconciliation: infra sets `InfraReady=True`, then app's prerequisite passes and it proceeds.
2. Version upgrade: both components reconcile normally (prerequisite already passed).

## Running

```bash
go run ./examples/component-prerequisites/.
```
