# Role Primitive Example

This example demonstrates the usage of the `role` primitive within the operator component framework.
It shows how to manage a Kubernetes Role as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a Role with core RBAC permissions.
- **Feature Mutations**: Composing policy rules from independent, feature-gated mutations using `AddRule`.
- **Metadata Mutations**: Setting version labels on the Role via `EditObjectMetadata`.
- **Data Extraction**: Inspecting the reconciled Role's rules after each sync cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from `examples/shared/app`.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: base rules, version labelling, and feature-gated secret and metrics access.
- `resources/`: Contains the central `NewRoleResource` factory that assembles all features using `role.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/role-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through four spec variations, printing the composed RBAC rules after each cycle.
4. Print the resulting status conditions.
