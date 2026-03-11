# Component System

The `component` package provides a structured way to manage logical features in a Kubernetes operator by grouping related resources into **Components**.

A Component acts as a behavioral unit responsible for reconciling multiple resources, managing their shared lifecycle, and reporting their aggregate health through a single condition on the owner CRD.

## Purpose

In complex operators, reconciliation logic often becomes fragmented across large controller loops. This leads to:
*   **Controller Logic Fragmentation**: Reconcilers coordinating dozens of unrelated resources in a single function.
*   **Inconsistent Lifecycle Handling**: Manual implementation of rollouts, suspension, and degradation for every feature.
*   **Scattered Status Reporting**: Inconsistent ways of determining if a feature is truly "Ready" or "Degraded".

Components solve these problems by providing:
*   **Structured Reconciliation**: A clear, repeatable pattern for resource synchronization.
*   **Lifecycle Orchestration**: Built-in support for progression, grace periods, and suspension.
*   **Consistent Status Aggregation**: Automated calculation of a single, meaningful status condition from multiple underlying resources.

## Component Responsibilities

A Component is responsible for:
*   **Resource Reconciliation**: Ensuring all registered resources (Deployments, Services, ConfigMaps, etc.) match their desired state.
*   **Health Aggregation**: Monitoring the status of each resource and determining the overall health of the logical feature.
*   **Lifecycle Semantics**: Applying high-level behaviors like "waiting for readiness" (grace periods) or "scaling down" (suspension).
*   **Status Exposure**: Maintaining exactly one `Condition` on the owner object's status that represents the component's state.

## Resource Registration

Resources are registered to a component using the `Builder`. The registration defines how the component interacts with each resource during reconciliation.

```go
builder := component.NewComponentBuilder(false).
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(deployment, false, false). // Managed (Create/Update)
    WithResource(configMap, false, true).  // Read-only
    WithResource(oldService, true, false)   // Delete-only
```

### Resource Flags

*   **Managed (Default)**: The component ensures the resource exists and matches the desired state. Its health contributes to the aggregate status.
*   **Read-only**: The component only reads the resource's state (e.g., to extract data or check health) but never modifies it in the cluster.
*   **Delete-only**: The component ensures the resource is removed from the cluster.

These flags dictate the reconciliation phase: managed resources are updated, read-only resources are only fetched, and delete-only resources are removed.

## Reconciliation Lifecycle

The `Reconcile` method follows a conceptual four-phase process:

1.  **Resource Synchronization**: All registered resources are processed. Managed resources are created or updated, delete-only resources are removed, and read-only resources are fetched.
2.  **Lifecycle Evaluation**: The component determines the current lifecycle mode (Normal or Suspended) and evaluates the progress of resources (e.g., checking if a Deployment is still rolling out).
3.  **Status Aggregation**: The individual states of all resources are collected and compared.
4.  **Condition Update**: A single aggregate `Condition` is calculated and applied to the owner CRD's status.

## Status Model

The framework categorizes component states into three functional groups:

### Converging States
These states occur during normal operation as the component moves toward a steady state.
*   **Creating**: Resources are being provisioned for the first time.
*   **Updating**: Existing resources are being modified.
*   **Scaling**: Resources (like Deployments) are changing their replica counts.
*   **Ready**: All resources are healthy and match the desired state.

### Grace States
These states are triggered when a component fails to reach "Ready" within its configured grace period.
*   **Ready**: All resources are healthy.
*   **Degraded**: The component is functional but some non-critical resources are unhealthy or it's taking longer than expected to converge.
*   **Down**: Critical resources are failing or the component is completely non-functional.

### Suspension States
These states manage the intentional deactivation of a component.
*   **PendingSuspension**: The suspension request is acknowledged, but work hasn't started.
*   **Suspending**: Resources are actively being scaled down or cleaned up.
*   **Suspended**: All resources have reached their suspended state (e.g., scaled to 0).

## Grace Period

A **Grace Period** defines how long a component is allowed to remain in "progressing" states (Creating, Updating, Scaling) before it is considered unhealthy.

*   During the grace period, the component reports its actual converging state (e.g., `Updating`).
*   After the grace period expires, if the component is still not `Ready`, the framework transitions the condition to **Degraded** or **Down** based on the resource health.

This prevents premature "False" readiness reports during normal operations like rolling updates.

## Suspension Lifecycle

Suspension allows an operator to intentionally "turn off" a component without deleting its configuration.

When a component is marked as suspended:
1.  It calls `Suspend()` on all `Suspendable` resources.
2.  Resources may scale down (e.g., Deployments to 0 replicas) or perform cleanup.
3.  The component tracks the `SuspensionStatus` of each resource.
4.  Once all resources report `Suspended`, the component condition transitions to `Suspended`.

## Condition Priority

When aggregating multiple resources, the framework uses a priority system to ensure the most critical information is reported. Failure states take precedence over progressing states, which take precedence over "Ready".

Conceptual priority (highest to lowest):
1.  **Error / Down / Degraded**: Something is wrong.
2.  **Suspension States**: The component is intentionally inactive.
3.  **Converging States**: The component is working toward readiness.
4.  **Ready**: Everything is healthy.

## ReconcileContext

The `ReconcileContext` is passed to the `Reconcile` method and provides all dependencies required for reconciliation:
*   **Kubernetes Client**: For interacting with the API server.
*   **Scheme**: For resource GVK lookups.
*   **Event Recorder**: For emitting Kubernetes Events.
*   **Metrics**: For recording component-level health metrics.
*   **Owner Object**: The CRD that owns the component.

Dependencies are passed explicitly to ensure the component remains testable and decoupled from global state or specific controller-runtime implementation details.

## Best Practices

*   **Keep Controllers Thin**: The controller should only be responsible for fetching the owner CRD and invoking component reconciliation.
*   **Model Logical Features**: Create one component per user-visible feature (e.g., "API", "UI", "Database").
*   **Group by Lifecycle**: Put resources that must live and die together into the same component.
*   **Split for Granularity**: If two features should report separate "Ready" conditions in the CRD status, they should be separate components.
