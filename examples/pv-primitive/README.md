# PersistentVolume Primitive Example

This example demonstrates the usage of the `pv` primitive within the operator component framework.
It shows how to manage a Kubernetes PersistentVolume as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a cluster-scoped PersistentVolume with storage configuration.
- **Feature Mutations**: Applying feature-gated changes to reclaim policy, mount options, and metadata.
- **Field Flavors**: Preserving annotations managed by external controllers using `PreserveCurrentAnnotations`.
- **Data Extraction**: Inspecting PV configuration after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from `examples/shared/app`.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: version labelling, boolean-gated retain policy, and mount options mutations.
- `resources/`: Contains the central `NewPVResource` factory that assembles all features using `pv.Builder`.
- `main.go`: A standalone entry point that demonstrates in-memory mutation across multiple spec variations.

## Running the Example

```bash
go run examples/pv-primitive/main.go
```

This will:
1. Create an `ExampleApp` owner object.
2. Apply mutations across four spec variations, printing the resulting PV YAML after each cycle.
3. Print operational status examples for each PV phase.
