# Ingress Primitive Example

This example demonstrates the usage of the `ingress` primitive within the operator component framework. It shows how to
manage a Kubernetes Ingress as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing an Ingress with rules, TLS, and ingress class.
- **Feature Mutations**: Applying conditional changes (TLS configuration, version annotations) using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools (e.g., cert-manager,
  ingress controllers).
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: version annotation and TLS configuration mutations.
- `resources/`: Contains the central `NewIngressResource` factory that assembles all features using the
  `ingress.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/ingress-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components through several spec changes (version upgrade, TLS toggle, suspension).
4. Print the resulting status conditions and Ingress resource state.
