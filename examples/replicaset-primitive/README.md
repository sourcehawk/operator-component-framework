# ReplicaSet Primitive Example

This example demonstrates the usage of the `replicaset` primitive within the operator component framework.
It shows how to manage a Kubernetes ReplicaSet as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a ReplicaSet with basic metadata and spec.
- **Feature Mutations**: Applying version-gated or conditional changes (sidecars, env vars, annotations) using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools (e.g., ArgoCD, manual edits).
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: sidecar injection, env vars, and version-based image updates.
    - `flavors.go`: usage of `FieldApplicationFlavor` to preserve fields.
- `resources/`: Contains the central `NewReplicaSetResource` factory that assembles all features using the `replicaset.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/replicaset-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components.
4. Print the resulting status conditions.
