# Service Primitive Example

This example demonstrates the usage of the `service` primitive within the operator component framework.
It shows how to manage a Kubernetes Service as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a Service with basic metadata, selector, and ports.
- **Feature Mutations**: Applying version-gated or conditional changes (additional ports, labels) using the `Mutator`.
- **Field Flavors**: Preserving annotations that might be managed by external tools (e.g., cloud load balancer controllers).
- **Operational Status**: Tracking whether the Service is operational (relevant for LoadBalancer types).
- **Suspension**: Deleting the Service when the component is suspended.
- **Data Extraction**: Harvesting information (ClusterIP, ports) from the reconciled resource.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from `examples/shared/app`.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: version labelling and conditional metrics port.
- `resources/`: Contains the central `NewServiceResource` factory that assembles all features using `service.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/service-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the Service ports after each cycle.
4. Print the resulting status conditions.
