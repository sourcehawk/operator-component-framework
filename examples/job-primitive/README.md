# Job Primitive Example

This example demonstrates the usage of the `job` primitive within the operator component framework.
It shows how to manage a Kubernetes Job as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a Job with basic metadata, spec, and restart policy.
- **Feature Mutations**: Applying version-gated or conditional changes (env vars, image version, retry policies) using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools (e.g., ArgoCD, manual edits).
- **Custom Status Handlers**: Overriding the default logic for determining completion status (`ConvergeStatus`).
- **Suspension**: Demonstrating how Jobs are suspended (deleted by default) when the component is suspended.
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: tracing env vars, retry policies, and version-based image updates.
    - `flavors.go`: usage of `FieldApplicationFlavor` to preserve fields.
    - `status.go`: implementation of a custom handler for completion status.
- `resources/`: Contains the central `NewJobResource` factory that assembles all features using the `job.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/job-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components through multiple spec changes.
4. Print the resulting status conditions after each reconciliation step.
