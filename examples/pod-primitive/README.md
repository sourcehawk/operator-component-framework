# Pod Primitive Example

This example demonstrates the usage of the `pod` primitive within the operator component framework. It shows how to
manage a Kubernetes Pod as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a Pod with basic metadata and spec.
- **Feature Mutations**: Applying version-gated or conditional metadata changes (labels) using the `Mutator`.
- **Suspension**: Deleting the pod when the component is suspended (pods cannot be paused).
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: label mutations applied to the Pod using the `Mutator`.
- `resources/`: Contains the central `NewPodResource` factory that assembles all features using the `pod.Builder`.
- `main.go`: A standalone entry point that demonstrates a single reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/pod-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components through multiple spec changes.
4. Print the resulting status conditions.
