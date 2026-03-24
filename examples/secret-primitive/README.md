# Secret Primitive Example

This example demonstrates the usage of the `secret` primitive within the operator component framework. It shows how to
manage a Kubernetes Secret as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a Secret with basic metadata and type.
- **Feature Mutations**: Composing secret entries from independent, feature-gated mutations using `SetStringData`.
- **Metadata Mutations**: Setting version labels on the Secret via `EditObjectMetadata`.
- **Field Flavors**: Preserving `.data` entries managed by external controllers using `PreserveExternalEntries`.
- **Data Extraction**: Harvesting Secret entries after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: base credentials, version labelling, and feature-gated tracing and metrics tokens.
  - `flavors.go`: usage of `FieldApplicationFlavor` to preserve externally-managed entries.
- `resources/`: Contains the central `NewSecretResource` factory that assembles all features using `secret.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/secret-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the secret entries after each cycle.
4. Print the resulting status conditions.
