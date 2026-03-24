# RoleBinding Primitive Example

This example demonstrates the usage of the `rolebinding` primitive within the operator component framework. It shows how
to manage a Kubernetes RoleBinding as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a RoleBinding with an immutable `roleRef` and basic metadata.
- **Feature Mutations**: Composing subjects from independent, feature-gated mutations using `EditSubjects`.
- **Metadata Mutations**: Setting version labels on the RoleBinding via `EditObjectMetadata`.
- **Field Flavors**: Preserving labels managed by external controllers using `PreserveCurrentLabels`.
- **Data Extraction**: Inspecting subjects and roleRef after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: base subject binding, version labelling, and feature-gated monitoring subject.
- `resources/`: Contains the central `NewRoleBindingResource` factory that assembles all features using
  `rolebinding.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/rolebinding-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through three spec variations, printing the subjects after each cycle.
4. Print the resulting status conditions.
