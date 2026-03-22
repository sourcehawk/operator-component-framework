# ClusterRoleBinding Primitive Example

This example demonstrates the usage of the `clusterrolebinding` primitive within the operator component framework. It shows how to manage a Kubernetes ClusterRoleBinding as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a ClusterRoleBinding with a roleRef and base subjects.
- **Feature Mutations**: Adding subjects conditionally via feature-gated mutations using `EditSubjects`.
- **Metadata Mutations**: Setting version labels on the ClusterRoleBinding via `EditObjectMetadata`.
- **Field Flavors**: Preserving labels managed by external controllers using `PreserveCurrentLabels`.
- **Data Extraction**: Inspecting ClusterRoleBinding state after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from `examples/shared/app`.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: version labelling and feature-gated monitoring subject addition.
- `resources/`: Contains the central `NewClusterRoleBindingResource` factory that assembles all features using `clusterrolebinding.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/clusterrolebinding-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through three spec variations, printing the reconciled ClusterRoleBinding state after each cycle.
4. Print the resulting status conditions.
