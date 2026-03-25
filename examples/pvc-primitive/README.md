# PVC Primitive Example

This example demonstrates the usage of the `pvc` primitive within the operator component framework. It shows how to
manage a Kubernetes PersistentVolumeClaim as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a PVC with access modes, storage request, and metadata.
- **Feature Mutations**: Applying conditional storage expansion and metadata updates using the `Mutator`.
- **Suspension**: PVCs are immediately suspended with data preserved by default.
- **Data Extraction**: Harvesting PVC status after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: version labelling, storage annotation, and conditional large-storage expansion.
- `resources/`: Contains the central `NewPVCResource` factory that assembles all features using `pvc.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/pvc-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, demonstrating version upgrades, storage expansion, and suspension.
4. Print the resulting status conditions.
