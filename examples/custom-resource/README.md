# Custom Resource

This example demonstrates how to manage a **custom resource** (CRD) that has no typed primitive in the framework, using
the **unstructured static builder**.

## What it shows

- **Unstructured builder**: `unstructured/static.NewBuilder` wraps an `*unstructured.Unstructured` object, giving it the
  same mutation and extraction capabilities as any typed primitive.
- **Content mutations**: `EditContent` with `UnstructuredContentEditor` sets nested spec fields (`issuerRef`,
  `dnsNames`) using structured helpers rather than raw map manipulation.
- **Metadata mutations**: `EditObjectMetadata` works the same way as on typed primitives.
- **Declared extraction**: `static.ExtractInto` reads fields from the reconciled unstructured object into a data cell.
- **Resource metrics**: `metrics.NewRecorder` records condition metrics and per-resource apply counters, and
  `WithMetricsIdentifier` keys the counters by a constant instead of the object's per-owner name.

## Use case

When your operator needs to manage a third-party CRD (cert-manager Certificate, Istio VirtualService, etc.) that has no
typed primitive wrapper in the framework, the unstructured builder is the escape hatch. You get the same reconciliation,
mutation, and extraction patterns without writing a full typed primitive.

## Reconciliation steps

1. Create the CertificateRequest with DNS names and issuer reference.
2. Steady-state reconciliation.
3. Print the apply counters gathered from the registry.

## Metrics

The CertificateRequest is named `<owner>-cert`, so keying a metric by its name would create one time series per
`ExampleApp`, and the framework never removes a series once created. `WithMetricsIdentifier("certificate")` keys it by a
constant instead, and the series count stays fixed however many owners exist.

The run ends by printing what the two reconciles did:

```text
ocf_resource_apply_total{...,operation="created",resource="certificate"} 1
ocf_resource_apply_total{...,operation="none",resource="certificate"} 1
```

One create, then nothing to do. A resource whose `operation="updated"` count keeps climbing in steady state is being
rewritten on every reconcile even though nothing changed. See [Metrics](../../docs/component.md#metrics).

## Running

```bash
go run ./examples/custom-resource/.
```
