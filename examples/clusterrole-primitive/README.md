# ClusterRole Primitive Example

This example demonstrates the usage of the `clusterrole` primitive within the operator component framework. It shows how
to manage a Kubernetes ClusterRole as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a cluster-scoped ClusterRole with basic metadata.
- **Feature Mutations**: Composing RBAC rules from independent, feature-gated mutations using `AddRule`.
- **Metadata Mutations**: Setting version labels on the ClusterRole via `EditObjectMetadata`.
- **Data Extraction**: Inspecting ClusterRole rules after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: core rules, version labelling, and feature-gated secret and deployment access.
- `resources/`: Contains the central `NewClusterRoleResource` factory that assembles all features using
  `clusterrole.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/clusterrole-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the composed rules after each cycle.
4. Print the resulting status conditions.
