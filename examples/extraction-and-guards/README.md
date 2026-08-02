# Data Extraction and Guards

This example demonstrates the declared data API: a **cell** carries a value from one resource's **extraction** to a
later resource's **data guard** and **mutation**, within the same component.

## What it shows

- **Cells**: `concepts.NewData[string]("db-host")` creates a named, typed cell. Cells are created inside the component
  assembly function, once per reconcile, and passed to the resource factories that need them.
- **Declared extraction**: The ConfigMap resource calls `configmap.ExtractInto(builder, dbHost, fn)`. After the
  ConfigMap is reconciled, `fn` runs against the reconciled object and its return value is written into the cell.
- **Data guard**: The Secret resource calls `builder.WithDataGuard(dbHost)`. The component blocks the Secret (and
  anything registered after it) until `dbHost` has been set, with a generated reason explaining which cell it is waiting
  on.
- **Require in a mutation**: The Secret also registers a mutation that calls `dbHost.Require()` to read the value and
  copy it into the Secret's data, so the credentials and the endpoint they connect to travel together.
- **Registration order matters**: The ConfigMap is registered before the Secret. `Build()` validates that every guarded
  or read cell has a producer registered strictly earlier, and rejects the component otherwise.
- **Topology introspection**: `Component.DataTopology()` returns the declared data flow, one edge per cell, without
  running any extraction. `main.go` prints it before reconciling.

## Reconciliation steps

1. Normal reconciliation: the ConfigMap is created, its extraction writes `dbHost`, the Secret's guard unblocks, and the
   Secret is created with the `db-host` entry copied in.
2. Steady-state: both resources reconcile normally.

## Testing cluster-free previews

A mutation that calls `Require()` needs the cell to already be set, but a golden-file preview never runs reconciliation,
so no extraction ever executes. The resource- and component-level tests seed the cell directly
(`dbHost.Set("postgres.default.svc")`) before asserting the golden file, simulating the value a real reconcile would
have extracted. `BuildComponent` returns the cell for exactly this reason.

## Running

```bash
go run ./examples/extraction-and-guards/.
```
