# Deployment Primitives

The `deployment` primitive is a specialized workload abstraction designed for managing Kubernetes `Deployment` resources. It provides a structured way to handle complex mutations, lifecycle readiness, and graceful deactivation.

## 1. Overview

Deployments in the framework are managed through the `deployment.Resource`, which integrates with the component lifecycle:
- **Health Tracking**: Automatically monitors `ReadyReplicas` and reports status.
- **Graceful Rollouts**: Detects stalled or failing rollouts via grace periods.
- **Suspension**: Supports scaling to zero as a primary deactivation mechanism.
- **Mutation Pipeline**: Offers a rich API for modifying metadata, pod specs, and containers.

## 2. Deployment Mutations

The deployment primitive uses a **plan-and-apply pattern** for modifications. Instead of mutating the Kubernetes object directly and repeatedly, the framework records "edit intent" through a series of planned mutations.

### Why this pattern exists:
- **Prevents uncontrolled mutation**: Changes are staged and applied in a single, controlled pass.
- **Improves composability**: Multiple independent features can contribute edits without knowing about each other.
- **Predictable Ordering**: Features are applied in the order they are registered. Later features observe the resource state after earlier features have already applied their changes.
- **Efficiency**: Avoids expensive and error-prone manual slice manipulations.

### Internal Ordering within a Feature

While features apply in registration order, the internal operations within a single feature follow a fixed category-based sequence to ensure consistency across the deployment structure.

When a feature mutation is applied to a Deployment, it follows this internal sequence:

1.  **Deployment metadata edits**: Modifications to the Deployment's labels and annotations.
2.  **DeploymentSpec edits**: Changes to replicas, strategy, selectors, etc.
3.  **Pod template metadata edits**: Modifications to the metadata of the pods created by the deployment.
4.  **Pod spec edits**: Changes to volumes, service accounts, or affinity in the pod template.
5.  **Regular container presence operations**: Adding or removing containers from the standard containers list.
6.  **Regular container edits**: Modifications to containers (env vars, args, resources). These use a snapshot taken *after* presence operations to ensure stable selector matching.
7.  **Init container presence operations**: Adding or removing init containers.
8.  **Init container edits**: Modifications to init containers, also using a snapshot for stable selection.

This ordering ensures that if one part of a mutation depends on another (e.g., editing a container that was just ensured to exist), the results are predictable and consistent.

## 3. Deployment Editors

To perform mutations, the framework provides several typed editors:

- `DeploymentSpecEditor`: Controls deployment-level settings like `Replicas` or `Strategy`.
- `PodSpecEditor`: Manages pod-level configuration such as `ServiceAccountName` or `Volumes`.
- `ContainerEditor`: Specialized for container-level changes like `EnsureEnvVar`, `RemoveArg`, or `SetResourceLimit`.
- `ObjectMetaEditor`: Used for both Deployment and Pod template labels/annotations.

### Example: Adding a sidecar container

```go
func SidecarMutation() deployment.Mutation {
    return func(m *deployment.Mutator) error {
        // 1. Ensure the sidecar container exists
        m.EnsureContainer(corev1.Container{
            Name:  "logger-sidecar",
            Image: "busybox:latest",
        })
        
        // 2. Configure the sidecar
        m.EditContainers(selectors.ContainerNamed("logger-sidecar"), func(e *editors.ContainerEditor) error {
            e.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "info"})
            return nil
        })
        
        return nil
    }
}
```

## 4. Usage Guidance & Caveats

- **Feature Registration Order**: The order in which features are added to a `ComponentBuilder` determines the order in which their mutations are applied. If Feature B depends on a change made by Feature A, ensure Feature A is registered first.
- **Stable Selectors**: Container selectors within a single feature are evaluated against the list of containers *after* that same feature's presence operations (adds/removes) have been applied. This allows a single feature to both add a container and then configure it.
- **Raw Escape Hatch**: If the provided editors are insufficient, use the `.Raw()` method on any editor to access the underlying Kubernetes struct directly.
