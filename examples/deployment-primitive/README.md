# Deployment Primitive Example

This example demonstrates the usage of the `deployment` primitive within the operator component framework. It shows how
to manage a Kubernetes Deployment as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a Deployment with basic metadata and spec.
- **Feature Mutations**: Applying version-gated or conditional changes (sidecars, env vars, annotations) using the
  `Mutator`.
- **Custom Status Handlers**: Overriding the default logic for determining readiness (`ConvergeStatus`) and health
  assessment during rollouts (`GraceStatus`).
- **Custom Suspension**: Extending the default suspension logic (scaling to 0) with additional mutations.
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: sidecar injection, env vars, and version-based image updates.
  - `status.go`: implementation of custom handlers for convergence, grace, and suspension.
- `resources/`: Contains the central `NewDeploymentResource` factory that assembles all features using the
  `deployment.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/deployment-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components.
4. Print the resulting status conditions.
