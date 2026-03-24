# ConfigMap Primitive Example

This example demonstrates the usage of the `configmap` primitive within the operator component framework. It shows how
to manage a Kubernetes ConfigMap as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a ConfigMap with basic metadata.
- **Feature Mutations**: Composing YAML configuration from independent, feature-gated mutations using `MergeYAML`.
- **Metadata Mutations**: Setting version labels on the ConfigMap via `EditObjectMetadata`.
- **Field Flavors**: Preserving `.data` entries managed by external controllers using `PreserveExternalEntries`.
- **Data Extraction**: Harvesting ConfigMap entries after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: base config, version labelling, and feature-gated tracing and metrics sections.
  - `flavors.go`: usage of `FieldApplicationFlavor` to preserve externally-managed entries.
- `resources/`: Contains the central `NewConfigMapResource` factory that assembles all features using
  `configmap.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/configmap-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the composed `app.yaml` after each cycle.
4. Print the resulting status conditions.
