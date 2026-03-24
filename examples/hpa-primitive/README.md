# HPA Primitive Example

This example demonstrates the usage of the `hpa` primitive within the operator component framework. It shows how to
manage a Kubernetes HorizontalPodAutoscaler as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing an HPA with a scale target ref, min/max replicas, and labels.
- **Feature Mutations**: Applying version-gated or conditional changes (CPU metrics, memory metrics, scaling behavior)
  using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools.
- **Operational Status**: Reporting HPA health based on `ScalingActive` and `AbleToScale` conditions.
- **Suspension (No-op)**: Demonstrating no-op suspend behavior — the HPA is left in place since an idle HPA has no
  cluster impact.
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: CPU metric, memory metric, scale behavior, and flavor functions.
- `resources/`: Contains the central `NewHPAResource` factory that assembles all features using the `hpa.Builder`.
- `main.go`: A standalone entry point that demonstrates a reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/hpa-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components through multiple spec changes.
4. Print the resulting status conditions and HPA state.
