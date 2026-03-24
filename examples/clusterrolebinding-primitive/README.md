# ClusterRoleBinding Primitive Example

This example demonstrates the usage of the `clusterrolebinding` primitive within the operator component framework. It
shows how to manage a Kubernetes ClusterRoleBinding as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a ClusterRoleBinding with a roleRef and base subjects.
- **Feature Mutations**: Adding subjects conditionally via feature-gated mutations using `EditSubjects`.
- **Metadata Mutations**: Setting version labels on the ClusterRoleBinding via `EditObjectMetadata`.
- **Field Flavors**: Preserving labels managed by external controllers using `PreserveCurrentLabels`.
- **Data Extraction**: Inspecting ClusterRoleBinding state after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: version labelling and feature-gated monitoring subject addition.
- `resources/`: Contains the central `NewClusterRoleBindingResource` factory that assembles all features using
  `clusterrolebinding.Builder`.
- `main.go`: A standalone entry point that demonstrates building and mutating a ClusterRoleBinding through multiple spec
  variations.

## Running the Example

```bash
go run examples/clusterrolebinding-primitive/main.go
```

This will:

1. Create an in-memory `ExampleApp` owner object.
2. For each of three spec variations, build a fresh resource and apply mutations to a simulated current
   ClusterRoleBinding.
3. Print the reconciled ClusterRoleBinding state (labels, roleRef, subjects) after each mutation cycle.
