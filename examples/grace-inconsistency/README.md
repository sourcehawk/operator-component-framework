# Grace Inconsistency Suppression

This example demonstrates how to suppress the **grace inconsistency warning** using the
`component.SuppressGraceInconsistencyWarning()` resource option.

## What it shows

- **Custom grace handler**: The Deployment overrides `WithCustomGraceStatus` to always return `GraceStatusHealthy`,
  regardless of replica readiness. This is intentional for a soft-dependency resource like a monitoring sidecar.
- **Inconsistency**: The convergence handler reports non-healthy (0 ready replicas), while the grace handler reports
  healthy. By default the framework logs a warning about this mismatch.
- **Suppression**: passing `component.SuppressGraceInconsistencyWarning()` to `WithResource` tells the framework the
  inconsistency is deliberate, silencing the warning.
- **Grace period**: The component uses `WithGracePeriod(5 * time.Second)`. The grace handler is consulted when the
  component reconciles after the grace period ends.
- **Requeue**: No watch event arrives when the grace period ends. The controller reads the delay with
  `comp.GraceRemaining(owner)` and returns it as `RequeueAfter`, so the next reconcile runs on time.

## When to use this

Use `SuppressGraceInconsistencyWarning` when a custom grace handler deliberately reports a different health status than
the convergence handler. The typical case is a resource that is "nice to have" but should not block the component from
being considered healthy during its grace period.

## Reconciliation steps

1. Initial reconciliation with 0 ready replicas. The condition is `Creating`, and the controller asks for a requeue when
   the grace period ends.
2. Reconciliation after the grace period. Convergence says non-healthy, grace says healthy, so the condition stays
   `Creating` and no warning is logged. No grace period is pending, so the controller asks for no requeue.

## Running

```bash
go run ./examples/grace-inconsistency/.
```
