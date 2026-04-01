# Data Extraction and Guards

This example demonstrates how to use **data extraction** from one resource to feed a **guard** on a subsequent resource
within the same component.

## What it shows

- **Data extraction**: The ConfigMap resource registers a `WithDataExtractor` that captures the `db-host` value into a
  shared pointer after reconciliation.
- **Guard**: The Secret resource registers a `WithGuard` that checks whether the extracted `db-host` is non-empty. If it
  is empty, the guard returns `Blocked` and the Secret (and any resources registered after it) are skipped.
- **Registration order matters**: The ConfigMap is registered before the Secret. Guards can only read data extracted by
  preceding resources.

## Reconciliation steps

1. Normal reconciliation: the ConfigMap is created, `db-host` is extracted, the guard unblocks, and the Secret is
   created.
2. Steady-state: both resources reconcile normally.

## Running

```bash
go run ./examples/extraction-and-guards/.
```
