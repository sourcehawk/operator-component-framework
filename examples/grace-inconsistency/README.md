# Grace Inconsistency Suppression

This example demonstrates how to suppress the **grace inconsistency warning** using `SuppressGraceInconsistencyWarning`
in `ResourceOptions`.

## What it shows

- **Custom grace handler**: The Deployment overrides `WithCustomGraceStatus` to always return `GraceStatusHealthy`,
  regardless of replica readiness. This is intentional for a soft-dependency resource like a monitoring sidecar.
- **Inconsistency**: The convergence handler reports non-healthy (0 ready replicas), while the grace handler reports
  healthy. By default the framework logs a warning about this mismatch.
- **Suppression**: `NewResourceOptionsBuilder().SuppressGraceInconsistencyWarning().Build()` tells the framework the
  inconsistency is deliberate, silencing the warning.
- **Grace period**: The component uses `WithGracePeriod(5 * time.Second)` to set the window during which the grace
  handler is consulted.

## When to use this

Use `SuppressGraceInconsistencyWarning` when a custom grace handler deliberately reports a different health status than
the convergence handler. The typical case is a resource that is "nice to have" but should not block the component from
being considered healthy during its grace period.

## Reconciliation steps

1. Initial reconciliation with 0 ready replicas. Convergence says non-healthy, grace says healthy, no warning logged.
2. Steady-state reconciliation.

## Running

```bash
go run ./examples/grace-inconsistency/.
```
