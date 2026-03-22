# ServiceAccount Primitive Example

This example demonstrates the usage of the `serviceaccount` primitive within the operator component framework.
It shows how to manage a Kubernetes ServiceAccount as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a ServiceAccount with basic metadata.
- **Feature Mutations**: Composing image pull secrets and automount settings from independent, feature-gated mutations.
- **Metadata Mutations**: Setting version labels on the ServiceAccount via `EditObjectMetadata`.
- **Field Flavors**: Preserving labels managed by external controllers using `PreserveCurrentLabels`.
- **Data Extraction**: Harvesting ServiceAccount fields after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from `examples/shared/app`.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: version labelling, image pull secrets, private registry, and automount control.
- `resources/`: Contains the central `NewServiceAccountResource` factory that assembles all features using `serviceaccount.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/serviceaccount-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the ServiceAccount's image pull secrets and automount settings after each cycle.
4. Print the resulting status conditions.
