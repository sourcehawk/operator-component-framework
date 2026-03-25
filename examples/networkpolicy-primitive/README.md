# NetworkPolicy Primitive Example

This example demonstrates the usage of the `networkpolicy` primitive within the operator component framework. It shows
how to manage a Kubernetes NetworkPolicy as a component of a larger application, utilising features like:

- **Base Construction**: Initializing a NetworkPolicy with pod selector and policy types.
- **Feature Mutations**: Composing ingress and egress rules from independent, feature-gated mutations.
- **Boolean-Gated Rules**: Conditionally adding metrics ingress rules based on a spec flag.
- **Metadata Mutations**: Setting version labels on the NetworkPolicy via metadata editors.
- **Label Coexistence**: Demonstrating how label updates from this component can coexist with labels managed by other
  controllers.
- **Data Extraction**: Reading the applied policy configuration after each reconcile cycle.

## Directory Structure

- `app/`: Defines the controller that uses the component framework. The `ExampleApp` CRD is shared from
  `examples/shared/app`.
- `features/`: Contains modular feature definitions:
  - `mutations.go`: HTTP ingress, boolean-gated metrics ingress, DNS egress, and version labelling.
- `resources/`: Contains the central `NewNetworkPolicyResource` factory that assembles all features using
  `networkpolicy.Builder`.
- `main.go`: A standalone entry point that demonstrates multiple reconciliation cycles with a fake client.

## Running the Example

```bash
go run examples/networkpolicy-primitive/main.go
```

This will:

1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile through three spec variations, printing the applied policy details after each cycle.
4. Print the resulting status conditions.
