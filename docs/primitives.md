# Resource Primitives

The `primitives` system provides a resource-centric abstraction layer for Kubernetes objects. It acts as the bridge 
between high-level **Components** and raw Kubernetes resources, handling the complexities of state synchronization, 
mutation, and lifecycle management.

## 1. What primitives are

Primitives are reusable, type-safe resource wrappers for Kubernetes objects. They encapsulate the logic required to 
reconcile a specific kind of resource (like a `Deployment` or `ConfigMap`) within the framework's behavioral model.

Each primitive encapsulates:

- **Desired state baseline**: A template or builder for the resource's "ideal" configuration.
- **Lifecycle integration**: Built-in support for readiness detection, grace periods, and suspension.
- **Mutation surfaces**: Controlled APIs for modifying resources based on active features or versions.
- **Field application behavior**: Precise rules for how fields are merged or preserved during reconciliation.

By using primitives, operator authors can avoid writing repetitive "create-or-update" boilerplate and instead focus on 
defining how their resources should behave.

---

## 2. Primitive categories

The framework distinguishes between three primary categories of primitives based on their operational characteristics.

### Static Primitives
Examples: `ConfigMap`, `Secret`, `ServiceAccount`, RBAC objects (`Role`, `RoleBinding`).

- **Characteristics**: These resources have a mostly static desired state. They are typically created once or updated 
  based on configuration changes but do not have complex runtime convergence or scaling behaviors.
- **Lifecycle**: Usually considered "Ready" as soon as they are successfully applied to the API server.

### Workload Primitives
Examples: `Deployment`, `StatefulSet`, `DaemonSet`.

- **Characteristics**: These resources represent long-running processes that require runtime convergence (e.g., 
  pods being scheduled and becoming ready).
- **Behavior**: They support advanced features like suspension (scaling to zero), grace handling for slow rollouts, and 
  complex feature-based mutations.

### Batch Primitives

TBD 

---

## 3. Field application model

Primitives use a structured pipeline to synchronize the desired state with the current state in the cluster. This 
process is managed by a **Field Applicator**.

### The Pipeline Order
When a primitive is reconciled, it follows a strict order of operations:

1.  **Baseline field application**: The `FieldApplicator` merges the "baseline" desired state onto the current object.
2.  **Flavor adjustments**: Post-baseline merge policies (Flavors) are applied to preserve specific fields.
3.  **Mutation edits**: Feature-specific or version-specific edits are applied (Workload primitives only).

This ensures that mutations always operate on a predictable, fully-formed baseline.

---

## 4. Field application flavors

**Flavors** are reusable merge policies that run after the baseline application but before mutations. Their primary 
purpose is to preserve fields that may be managed by other controllers or external systems (like sidecar injectors 
or autoscalers).

### Examples of Flavors:
- **Preserving Labels/Annotations**: Ensuring that metadata added by external tools is not wiped out during 
  reconciliation.
- **Preserving Pod Template Metadata**: Keeping sidecar-related annotations on a Deployment's pod template.

Flavors allow the framework to be "good citizens" in a cluster where multiple controllers might be touching the same 
resources.

---

## 5. Mutation system

Workload primitives employ a **plan-and-apply pattern** for modifications. Instead of mutating the Kubernetes object
directly and repeatedly, the framework records "edit intent" through a series of planned mutations.

### Why this pattern exists:
- **Prevents uncontrolled mutation**: Changes are staged and applied in a single, controlled pass.
- **Improves composability**: Multiple independent features can contribute edits without knowing about each other.
- **Efficiency**: Avoids expensive and error-prone manual slice manipulations (like searching for a container by name 
  multiple times).

---

## 6. Mutation editors

**Editors** provide a scoped, typed API for making changes to specific parts of a resource. They ensure that mutations 
are safe and follow Kubernetes best practices.

Common editors include:
- `ContainerEditor`: For modifying environment variables, arguments, and resource limits.
- `PodSpecEditor`: For managing volumes, affinity, or service account names.
- `DeploymentSpecEditor`: For controlling replicas, strategy, and selectors.
- `ObjectMetaEditor`: For manipulating labels and annotations.

Editors act as a protective layer, offering helper methods like `EnsureEnvVar` or `RemoveArg`.

---

## 7. Selectors

**Selectors** determine which parts of a resource an editor should target. This is particularly important for 
multi-container pods.

For example, a `ContainerSelector` can be used to:
- Target all containers (`AllContainers()`).
- Target a specific container by name (`ContainerNamed("sidecar")`).
- Target containers at specific indices (`ContainerAtIndex(0)`).

Selectors allow mutations to be precise and reusable across different resource configurations.

---

## 8. Raw mutation escape hatch

While editors provide safe wrappers, there are times when you need to perform advanced customizations that the 
framework doesn't explicitly support. For these cases, every editor provides a `Raw()` method.

- **Purpose**: Gives direct access to the underlying Kubernetes struct (e.g., `*corev1.Container`).
- **Safety**: The mutation remains scoped to the editor's target (e.g., you can't accidentally delete the entire PodSpec from a ContainerEditor).
- **Flexibility**: Ensures that the framework never blocks you from using new Kubernetes features or edge-case configurations.

---

## 9. Default lifecycle behavior

Workload primitives come with "sane defaults" for lifecycle management, integrated directly into the Component status model:

- **Convergence detection**: Automatically determines if a Deployment is "Ready", "Scaling", or "Updating" based on its status fields.
- **Grace handling**: Monitors how long a resource has been non-ready and reports "Degraded" or "Down" if it exceeds a grace period.
- **Suspension behavior**: Provides the logic for scaling resources down to zero and reporting the "Suspended" state.

These defaults can be overridden via the primitive's `Builder` if specialized behavior is required.

---

## 10. When to implement a custom resource

While the provided primitives cover the most common Kubernetes objects, you may need to implement a custom resource 
wrapper when:

- You are managing **custom CRDs** that require specific health checks.
- You have **unusual lifecycle semantics** (e.g., a resource that must be deleted and recreated instead of updated).
- You need **highly specialized mutation behavior** not covered by standard editors.

Custom resource wrappers can still leverage the framework's core interfaces (`component.Resource`, `component.Alive`, 
`component.Suspendable`). See the `examples/` directory for patterns on implementing custom resource wrappers.

---

## Examples

### Creating a primitive resource
```go
// Define a baseline Deployment
deployment := &appsv1.Deployment{ ... }

// Use the builder to create a primitive
resource, err := deployment.NewBuilder(deployment).
    WithFieldApplicationFlavor(deployment.PreserveCurrentLabels).
    Build()
```

### Adding mutation edits
```go
// Mutations are typically defined within Feature objects
mutation := deployment.Mutation{
    Name: "add-proxy-sidecar",
    ApplyIntent: func(m *deployment.Mutator) error {
        m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) {
            e.EnsureEnvVar(corev1.EnvVar{Name: "PROXY_ENABLED", Value: "true"})
        })
        return nil
    },
}
```

### Selecting containers for mutation
```go
// Targeting multiple specific containers
m.EditContainers(selectors.ContainersNamed("web", "api"), func(e *editors.ContainerEditor) {
    e.EnsureArg("--verbose")
})
```
