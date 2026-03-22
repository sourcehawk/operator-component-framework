# CronJob Primitive Example

This example demonstrates the usage of the `cronjob` primitive within the operator component framework.
It shows how to manage a Kubernetes CronJob as a component of a larger application, utilizing features like:

- **Base Construction**: Initializing a CronJob with a schedule, job template, and containers.
- **Feature Mutations**: Applying conditional changes (tracing env vars, metrics annotations, version-based image updates) using the `Mutator`.
- **Field Flavors**: Preserving labels and annotations that might be managed by external tools.
- **Suspension**: Suspending the CronJob by setting `spec.suspend = true`.
- **Data Extraction**: Harvesting information from the reconciled resource.

## Directory Structure

- `app/`: Defines the mock `ExampleApp` CRD and the controller that uses the component framework.
- `features/`: Contains modular feature definitions:
    - `mutations.go`: tracing env vars, metrics annotations, and version-based image updates.
- `resources/`: Contains the central `NewCronJobResource` factory that assembles all features using the `cronjob.Builder`.
- `main.go`: A standalone entry point that demonstrates a reconciliation loop using a fake client.

## Running the Example

You can run this example directly using `go run`:

```bash
go run examples/cronjob-primitive/main.go
```

This will:
1. Initialize a fake Kubernetes client.
2. Create an `ExampleApp` owner object.
3. Reconcile the `ExampleApp` components through multiple spec changes.
4. Print the resulting status conditions.
