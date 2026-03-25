# PodDisruptionBudget Primitive Example

This example demonstrates the usage of the `pdb` primitive within the operator component framework. It shows how to
manage a Kubernetes PodDisruptionBudget as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a PDB with a percentage-based `MinAvailable` and a label selector.
- **Feature Mutations**: Switching between `MinAvailable` and `MaxUnavailable` based on a feature toggle via `EditSpec`.
- **Metadata Mutations**: Setting version labels on the PDB via `EditObjectMetadata`.
- **Data Extraction**: Inspecting the reconciled PDB's disruption policy after each sync cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: version labelling and feature-gated strict availability.
- `resources/`: Contains the central `NewPDBResource` factory that assembles all features using `pdb.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/pdb-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the PDB disruption policy after each cycle.
4. Print the resulting status conditions.
