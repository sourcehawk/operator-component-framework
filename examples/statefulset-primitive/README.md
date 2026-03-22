# StatefulSet Primitive Example

This example demonstrates the usage of the `statefulset` primitive within the operator component framework.
It shows how to manage a Kubernetes StatefulSet as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a StatefulSet with basic metadata, spec, and volume claim templates.
- **Feature Mutations**: Applying version-gated or conditional changes (sidecars, env vars, annotations) using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools (e.g., ArgoCD, manual edits).
- **Custom Status Handlers**: Overriding the default logic for determining readiness (`ConvergeStatus`) and health assessment during rollouts (`GraceStatus`).
- **Custom Suspension**: Extending the default suspension logic (scaling to 0) with additional mutations.
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: sidecar injection, env vars, and version-based image updates.
    - `flavors.go`: usage of `FieldApplicationFlavor` to preserve fields, plus custom status and suspension handlers.
- `resources/`: Contains the central `NewStatefulSetResource` factory that assembles all features using the `statefulset.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/statefulset-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components.
4. Print the resulting status conditions.
