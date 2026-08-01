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

## Use case

When your operator needs to manage a third-party CRD (cert-manager Certificate, Istio VirtualService, etc.) that has no
typed primitive wrapper in the framework, the unstructured builder is the escape hatch. You get the same reconciliation,
mutation, and extraction patterns without writing a full typed primitive.

## Reconciliation steps

1. Create the CertificateRequest with DNS names and issuer reference.
2. Steady-state reconciliation.

## Running

```bash
go run ./examples/custom-resource/.
```
