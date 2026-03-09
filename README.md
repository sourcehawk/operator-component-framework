# Operator Component Framework

A Go framework for building highly maintainable Kubernetes operators using a behavioral component model and version-
gated feature mutations. This framework provides feature-level reconciliation, shared lifecycle handling, and 
version-aware resource customization for operator authors who need to manage complex resource lifecycles and evolving 
feature sets with consistency.

The framework is most useful once an operator has multiple logical features, non-trivial lifecycle handling, or 
growing version-specific resource logic.

## Why this package exists

Kubernetes operators often grow into bloated controllers with repetitive reconciliation logic, inconsistent status 
reporting, and complex version-specific conditionals. This framework provides a structured behavioral model to solve 
these problems by:
*   **Grouping resources into Components**: Manage logical features (like "Web Interface") as a single unit with 
  aggregated health and shared lifecycle behavior.
*   **Using Feature Mutations**: Define a clean baseline for resources and apply optional behavior or version-specific 
  compatibility as explicit, composable mutations.

## In this guide
- [Component Framework](#component-framework): Structured resource management, status aggregation, and lifecycle 
  orchestration.
- [Feature Mutations](#feature-mutations): Composable, version-gated resource modifications using an additive-first 
  planner model.

**Suggested reading path:** [Mental Model](#mental-model) → [Minimal Example](#minimal-example) → [Feature Mutations](#feature-mutations)

## Component Framework

The `component` package provides a structured way to manage logical features in a Kubernetes operator by grouping 
related resources into **Components**.

### Logic Fragmentation

In many Kubernetes operators, the controller ends up owning most of the system's behavior. Resource creation,
lifecycle management, status reporting, and feature-specific logic are all implemented inside the controller's
reconciliation loop.

As the operator grows, this often leads to:

- large reconciliation functions coordinating many resources
- lifecycle logic (rollouts, suspension, degradation) implemented repeatedly
- status reporting handled differently across features
- resource configuration mixed together with orchestration logic

Over time, controllers become harder to reason about and changes to lifecycle or status behavior must be replicated
across multiple parts of the codebase.

### Behavioral Units

This framework introduces a **Component** as a first-class abstraction for managing a logical feature of an operator.

Instead of implementing all orchestration inside the controller reconciliation loop, related resources are grouped
into a behavioral unit responsible for managing their lifecycle and reporting their combined health.

A Component becomes responsible for:

- Reconciling resources in a consistent, predictable way.
- Aggregating resource health into a single user-facing condition.
- Applying shared lifecycle semantics (progression, degradation, suspension).
- Centralizing error handling and condition updates.

Controllers then focus on **deciding which components should exist** based on the desired state of the custom resource,
while the framework provides the **shared mechanics for managing those resources**.

> A **Component** manages a set of resources as one logical feature and reports exactly one **Condition Type** on the owning CRD.

## Mental Model

A good way to think about the framework is as a hierarchy of responsibility:

```text
Controller
  └── Component
        └── Resource wrappers
              └── Kubernetes objects
```

Each layer has a different role:

1. **Controller**
   Decides which components should exist based on the owner spec and orchestrates reconciliation at a high level.

2. **Component**
   Represents one logical feature, reconciles its resources, and reports one user-facing condition such as 
   `WebInterfaceReady`.

3. **Resource wrapper**
   Encapsulates the desired state and lifecycle behavior of a Kubernetes object. This is where you define how a 
   Deployment, Service, or custom resource behaves inside the framework.

4. **Kubernetes object**
   The raw `client.Object` that is eventually persisted to the cluster.

This separation maintains thin controllers, consistent status handling, and reusable resource-specific behavior.

## Core Concepts

### Component

A `Component` is the top-level coordinator for a single logical feature.

It is responsible for:

* reconciling all of its registered resources
* aggregating their status into one condition
* applying grace-period behavior
* handling suspension and deletion behavior
* reporting failures consistently

A `Component` can be initialized using a builder:

```go
comp, err := component.NewComponentBuilder(owner.Spec.Suspended).
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(res, false, false).
    Build()
```

### Resource

A `Resource` wraps a Kubernetes object and defines how the framework should manage it.

The key responsibilities are:

* exposing the underlying object
* applying immutable fields during initial creation (`SetImmutable`)
* applying mutable fields during ongoing reconciliation (`SetMutable`)
* providing a stable identity for logging and error reporting

This abstraction separates **how an object should look** from **how the framework reconciles it**.

### Alive

`Alive` is an optional interface for resources with observable runtime health.

Implement it when a resource has meaningful readiness semantics beyond “the object exists.”

Implementation of the `ConvergingStatus` method:

```go
func (r *DeploymentResource) ConvergingStatus(op component.ConvergingOperation) (component.ConvergingStatusWithReason, error) {
    if r.deployment.Status.ReadyReplicas < *r.deployment.Spec.Replicas {
        return component.ConvergingStatusWithReason{
            Status:  component.ConvergingStatusCreating,
            Reason:  "Creating",
            Message: fmt.Sprintf("Waiting for replicas: %d/%d ready", r.deployment.Status.ReadyReplicas, *r.deployment.Spec.Replicas),
        }, nil
    }
    return component.ConvergingStatusWithReason{Status: component.ConvergingStatusReady}, nil
}
```

`Alive` enables two related status models:

* **Converging status**: how the resource is progressing toward readiness
* **Grace status**: how unhealthy the resource is after a grace period has expired

This allows the framework to distinguish between:

* “the resource is still rolling out”
* “the resource has had enough time and is now degraded or down”

### Suspendable

`Suspendable` is an optional interface for resources that support being paused, scaled down, or otherwise hibernated.

Defining a suspension strategy:

```go
func (r *DeploymentResource) Suspend() error {
    r.suspender = func() error {
        r.deployment.Spec.Replicas = ptr.To(int32(0))
        return nil
    }
    return nil
}
```

The framework treats suspension as a first-class lifecycle, ensuring it is not a controller-specific afterthought.

### DataExtractable

`DataExtractable` is an optional interface for resources that need to expose internal data after they have been 
synchronized with the cluster.

Registering a data extractor in a builder:

```go
func (b *DeploymentBuilder) WithDataExtractor(
    extractor func(appsv1.Deployment) error,
) *DeploymentBuilder {
    b.res.dataExtractors = append(b.res.dataExtractors, extractor)
    return b
}
```

By using `DataExtractable`, you avoid the need to retain concrete resource types elsewhere just to pull data back out. 
The framework handles the extraction automatically during reconciliation.

Data extraction is:

* **Observational**: It should be a read-only operation on the resource's underlying object.
* **Automatic**: It is triggered during `Reconcile()` after all creation and read-only resources are updated from the cluster.
* **Safe**: Extraction only happens during normal reconciliation and is skipped when the component is suspended.

### ReconcileContext

`ReconcileContext` carries the shared dependencies a component needs during reconciliation, such as:

* Kubernetes `Client`
* `Scheme`
* event recorder
* metrics recorder
* the owning CRD

It is intentionally small and explicit. The component receives everything it needs to reconcile without reaching into 
controller state directly.

## Status Model

One of the biggest advantages of the framework is that it reduces the state of many resources into one meaningful 
condition on the owner.

This is done through a prioritized state model.

### Converging states

Converging states represent progress toward readiness:

* `Creating`
* `Updating`
* `Scaling`
* `Ready`

These are used while a resource is still actively converging.

Within converging states, the framework uses priority so the most meaningful non-ready state wins.

### Grace states

If a component remains non-ready after its configured grace period, the framework switches from reporting **progress** 
to reporting **health**.

Grace states are:

* `Ready`
* `Degraded`
* `Down`

This is what prevents components from appearing permanently “Creating” or “Scaling” even when something is actually 
broken.

### Suspension states

When suspension is requested, the component moves through a separate lifecycle:

* `PendingSuspension`
* `Suspending`
* `Suspended`

This makes suspension visible and explicit in status instead of burying it inside custom controller logic.

### Condition priority

At the component level, statuses are aggregated using priority so that the most important explanation wins.

Conceptually, the order is:

1. **Error**
2. **Down**
3. **Degraded**
4. **Suspension states**
5. **Progression states**
6. **Ready**

This means:

* real failures dominate everything
* suspension dominates ordinary rollout progress
* progress dominates steady-state ready

The result is a top-level condition that communicates the real state of the feature.

## Design Philosophy

### Reconcile logical features, not raw objects

Users think in terms of features: *“Is the web UI ready?”* not *“Did the Service and Deployment reconcile separately?”*

The framework encourages modeling reconciliation around logical units that map to how humans reason about the system.

### Keep lifecycle behavior consistent

Without a shared framework, every controller tends to invent its own:

* readiness rules
* error handling style
* status transitions
* suspension behavior

That leads to drift and confusion. The component package centralizes these rules so features behave consistently.

### Separate object logic from orchestration

The controller should decide **what** to reconcile.
The component should decide **how the feature behaves**.
The resource wrapper should decide **how a specific object is configured**.

That separation improves readability, reuse, and testability.

### Treat status as stateful, not just reactive

Condition progression is intentionally stateful. The framework uses previous condition state and timestamps to avoid 
flapping and to distinguish between:

* a normal in-progress rollout
* a prolonged unhealthy state
* a deliberate suspension flow

This is one of the main reasons the resulting conditions tend to be more useful than naïve “latest observation only” 
reporting.

## Benefits

Using the framework provides several concrete advantages:

* **Consistent reconciliation behavior** across features.
* **Cleaner controllers** that focus on assembling components and calling `Reconcile`.
* **Richer status conditions** with improved reasons and lifecycle semantics.
* **Built-in suspension and grace handling**.
* **Reusable resource abstractions**.
* **Enhanced testability** at both resource and component levels.

In practice, this allows controllers to focus on business logic while the framework handles the repetitive status and 
lifecycle machinery.

## Flexibility and Extension Points

The framework is intentionally flexible.

You can adapt it by:

* wrapping any `client.Object` in your own `Resource`
* implementing `Alive` only where health is meaningful
* implementing `Suspendable` only where suspension is meaningful
* combining managed, read-only, and delete-only resources in one component
* injecting custom policy into generic wrappers through builders or handler functions

A particularly useful pattern is to build reusable resource wrappers with optional behavior injection. That lets you 
keep Kubernetes mechanics in one place while still allowing feature-specific policies.

## Example: Custom Deployment Resource

The example implementation shows a custom `Deployment` resource wrapper that implements:

* `Resource`
* `Alive`
* `Suspendable`
* `DataExtractable`

It also uses a `DeploymentBuilder` to allow optional injection of custom behavior for status handling and suspension.

This demonstrates one of the framework’s biggest strengths: the wrapper can stay generic while the feature-specific 
rules remain configurable.

### Resource Implementation

The following snippet illustrates how `SetMutable` uses a restricted mutator to apply version-gated feature mutations:

```go
func (r *DeploymentResource) SetMutable() error {
    // 1. Core baseline mutable state
    if err := r.applyCoreConfiguration(); err != nil {
        return err
    }

    // 2. Apply feature mutations via a restricted mutator interface
    mutator := NewDeploymentResourceMutator(r)

    for _, m := range r.mutations {
        if err := m.ApplyIntent(mutator); err != nil {
            return fmt.Errorf("failed to apply mutation %s: %w", m.Name, err)
        }
    }

    return mutator.Apply()
}
```
For the full implementation of this custom resource and its builder, see the [example resources directory](/examples/component-architecture-basics/resources/).

## Minimal Example

If you do not need custom behavior injection, usage can stay very small.

A minimal implementation of a component and its reconciliation:

```go
deployment := NewDeploymentBuilder("web", owner.Namespace).Build()

component, err := component.NewComponentBuilder(owner.Spec.Suspended).
	WithName("WebInterface").
	WithConditionType("WebInterfaceReady").
	WithResource(deployment, false, false).
	WithGracePeriod(5 * time.Minute).
	Build()
if err != nil {
	return err
}

err = component.Reconcile(ctx, recCtx)
if err != nil {
	return err
}
```

This keeps the “happy path” simple while still allowing more advanced customization later.

## Component assembly

A typical controller flow has three steps:

1. construct resource wrappers
2. assemble them into a component
3. call `Reconcile`

A full assembly example within a controller:

```go
// 1. Construct resources using builders
res := resources.NewDeploymentBuilder("web", owner.Namespace).
    WithMutation(features.TracingFeature(version, owner.Spec.TracingEnabled)).
    Build()

// 2. Assemble the component
comp, err := component.NewComponentBuilder(owner.Spec.Suspended).
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(res, false, false).
    Build()

// 3. Reconcile
recCtx := component.ReconcileContext{
    Client:   r.Client,
    Scheme:   r.Scheme,
    Owner:    owner,
    // ...
}
err = comp.Reconcile(ctx, recCtx)
```

This style keeps the controller focused on composition and leaves lifecycle orchestration to the framework.

## Practical Guidance

### When should I create a new `Resource` implementation?

Create a new wrapper when:

* the object needs non-trivial desired-state logic
* readiness or lifecycle semantics matter
* you want to reuse the wrapper across multiple components
* a built-in or existing wrapper would mix too much feature-specific policy into one place

### When should I implement `Alive`?

Implement `Alive` for resources where existence is not enough.

Good candidates:

* `Deployment`
* `StatefulSet`
* `Job`
* custom resources with meaningful status

Usually skip it for:

* `ConfigMap`
* `Secret`
* `ServiceAccount`
* RBAC resources

Those resources are usually either present or absent; they do not normally have their own convergence lifecycle.

### When should I implement `DataExtractable`?

Implement `DataExtractable` when you need to "read back" information from a resource after it has been reconciled.

Good candidates:

* `Secret` or `ConfigMap` with auto-generated values
* `Service` with a dynamically assigned `LoadBalancer` IP
* Any resource where the cluster-side state contains data needed by other parts of the operator

This pattern allows your controller to remain decoupled from the specific resource implementation while still being
able to access the data it needs.

### When should I implement `Suspendable`?

Implement `Suspendable` when the resource:

* represents active workload
* consumes meaningful cost or compute
* can be safely paused or scaled down
* needs explicit lifecycle behavior when suspension is requested

Examples:

* scaling a Deployment to zero
* mutating retention or deletion settings before shutdown
* choosing whether a resource should remain present while suspended

### When should I use read-only resources?

Use read-only resources when the component depends on something it does not own.

Examples:

* a frontend that depends on a database managed elsewhere
* a feature that should only become ready after another controller has reconciled a shared dependency

This allows a component to observe health without taking ownership of the object.

### When should I split one feature into multiple components?

Use multiple components when:

* they need separate status conditions
* they can fail independently
* they can be suspended independently
* they represent different user-facing features

Keep resources in one component when they share a common lifecycle and should be understood as one feature.

A good rule of thumb is:

> if users would expect separate readiness or suspension semantics, use separate components.

## Testing

The framework improves testing in two ways.

### Resource-level testing

You can test a wrapper in isolation:

* does `SetMutable()` produce the desired object spec?
* does `ConvergingStatus()` report the right rollout state?
* does `Suspend()` apply the expected mutation?

This can often be done without a running cluster.

### Component-level testing

You can test component orchestration separately from individual resource logic:

* does the component aggregate statuses correctly?
* does it transition through grace states correctly?
* does suspension take precedence?
* does it set error conditions consistently?

This lets you test lifecycle behavior once and then focus feature-specific tests on your resource wrappers.

## Summary

The component framework gives operator authors a consistent way to model:

* logical features instead of scattered objects
* resource lifecycle and readiness
* suspension and grace periods
* aggregated conditions on the owner

It is especially useful once an operator grows beyond a handful of simple objects and needs clearer status reporting, 
more reusable reconciliation patterns, and more predictable lifecycle behavior.

## Connecting Components and Feature Mutations

While **Components** define how a logical feature is reconciled and reported to the user, **Feature Mutations** define 
how individual resources inside those features remain readable and composable as optional behavior and version 
constraints accumulate.

Together, they allow you to build operators where the high-level orchestration is consistent and the low-level resource 
configuration is modular and version-aware.

## Feature Mutations

### The problem with historical layering

A recurring problem in operator development is that resource construction gradually becomes a mix of:
- application-version compatibility logic
- feature-specific behavior
- incremental mutations layered on top of older resource definitions
- one-off conditional branches for special cases

This usually starts out innocently. A resource begins with a “core” implementation, and support for a new application 
version is added with a few conditionals. Later, another feature is introduced; then another version changes behavior 
again.

Over time, the resource stops expressing a clear **desired state** and instead becomes a record of **how it evolved**.

Historical layering often creates several structural problems:

1.  **Desired state is obscured**: The core question—"What should this resource look like now?"—is buried under years of `if/else` logic.
2.  **Logic coupling**: Version compatibility, optional features, and baseline config are tightly mixed, making changes risky.
3.  **Fragmented patterns**: Each resource evolves its own style for version checks and slice manipulation (env vars, args).
4.  **Implicit interactions**: Multiple features modifying the same fields (like `Env` or `Args`) result in fragile, order-dependent code.

### The shift to baseline + mutations

To solve this, we invert the model:

> The core resource expresses the **baseline desired state for the current version**, and optional behavior is applied 
> through explicit, composable **feature mutations**.

This move shifts focus away from "patches on old logic" toward:
1.  **Core Resource**: Defines the current baseline desired state.
2.  **Feature Gates**: Decide if a mutation applies based on version or custom logic.
3.  **Feature Mutations**: Apply small, focused modifications via a controlled planner interface.

### Feature mutation design principles

When using feature mutations, follow these principles:

#### 1. The core resource must define the current baseline

The core should describe the default desired state for the latest implementation. It should not represent historical 
compatibility logic; it should represent the **baseline desired state**.

#### 2. Feature logic must not be embedded in the baseline

Feature-specific behavior should be expressed as explicit mutations registered on the resource rather than inlined 
into core construction logic.

#### 3. Feature mutations are additive-first
A mutation should only change the fields necessary for that feature. Use the planner/mutator interface to add 
environment variables or arguments rather than replacing entire slices. 

*Note: While additive behavior is preferred, compatibility may sometimes require narrowly scoped removal or override
behavior, which should still be performed through the mutator interface.*

#### 4. Feature mutations should be idempotent
Applying the same mutation more than once should not produce duplicates or corrupt the object.

#### 5. Composition must be intentional
Mutations should assume they may run alongside other features. Maintain narrow scope and minimal mutation surface.

### Feature gates

Feature mutations are controlled by version-aware feature gates.

A `ResourceFeature` manages the conditions under which a mutation is enabled. It uses a **logical AND** model: a 
feature is enabled only when **all** registered semver constraints match the current version and **all** additional 
truth conditions are true.

```go
type ResourceFeature struct {
	current            string
	versionConstraints []VersionConstraint
	requiredTruths     []bool
}

func NewResourceFeature(currentVersion string, versionConstraints []VersionConstraint) *ResourceFeature {
	return &ResourceFeature{
		current:            currentVersion,
		versionConstraints: versionConstraints,
	}
}

// When adds a boolean condition that must be true for the feature to be enabled.
// All values passed through When must be true for Enabled() to return true.
func (f *ResourceFeature) When(truth bool) *ResourceFeature {
	f.requiredTruths = append(f.requiredTruths, truth)
	return f
}

func (f *ResourceFeature) Enabled() (bool, error) {
	for _, truth := range f.requiredTruths {
		if !truth {
			return false, nil
		}
	}

	for _, constraint := range f.versionConstraints {
		enabled, err := constraint.Enabled(f.current)
		if err != nil {
			return false, err
		}
		if !enabled {
			return false, nil
		}
	}

	return true, nil
}
```

Defining a feature with specific version constraints:

```go
feature := NewResourceFeature(currentVersion, []feature.VersionConstraint{
	feats.FromSemver(">=8.0.0"),
	feats.FromSemver("<9.0.0"),
}).When(enableSomething)
```

This ensures that a feature only applies when all specified conditions (version and custom logic) are satisfied, 
keeping version-gating logic small and explicit.

### Feature mutation model

A feature mutation represents a small, self-contained modification intent for a resource.

```go
type Mutation[T any] struct {
    Name    string
    Feature *ResourceFeature
    Mutate  func(T) error
}
```

Feature mutations express **intent**. When `ApplyIntent` is called, the mutation records what it wants to change in 
a **mutation planner** (the mutator). The actual modification of the Kubernetes resource happens later in a final 
`Apply()` phase.

A resource evaluates and applies enabled mutations during `SetMutable()`:

```go
mutator := NewDeploymentResourceMutator(r)

for _, m := range r.mutations {
    // Apply the **intent** of each mutation to the planner
    if err := m.ApplyIntent(mutator); err != nil {
        return fmt.Errorf("failed to apply mutation %s: %w", m.Name, err)
    }
}

// Final phase where the planned mutations are applied to the Kubernetes object
mutator.Apply()
```

### Example: Deployment with additive feature mutations

The [example implementation](/examples/component-architecture-basics/) demonstrates a resource that:

* defines a clear baseline desired state
* supports version-gated feature mutations
* applies mutations through a **restricted mutator interface** that acts as a **mutation planner**
* avoids repeated slice scanning using internal maps

### Resource wrapper

The `DeploymentResource` holds the underlying Deployment and the list of mutations:

```go
type DeploymentResource struct {
    deployment *appsv1.Deployment
    mutations  []feature.Mutation[*DeploymentResourceMutator]
}
```

### Core baseline configuration

The core resource defines the baseline desired state before any features are applied.

```go
func (r *DeploymentResource) applyCoreConfiguration() error {
    replicas := int32(1)
    r.deployment.Spec.Replicas = &replicas

    if len(r.deployment.Spec.Template.Spec.Containers) == 0 {
        r.deployment.Spec.Template.Spec.Containers = append(r.deployment.Spec.Template.Spec.Containers, corev1.Container{
            Name: "app",
        })
    }
    
    container := &r.deployment.Spec.Template.Spec.Containers[0]
    container.Env = []corev1.EnvVar{
        {Name: "LOG_LEVEL", Value: "info"},
    }
    return nil
}
```

### Feature Mutations

Feature mutations express **intent**, which the mutator gathers as a plan. The final resource state is applied once
via a final `Apply()` call.

```go
func (r *DeploymentResource) SetMutable() error {
    if err := r.applyCoreConfiguration(); err != nil {
        return err
    }

    mutator := NewDeploymentResourceMutator(r)

    for _, m := range r.mutations {
        if err := m.ApplyIntent(mutator); err != nil {
            return fmt.Errorf("failed to apply mutation %s: %w", m.Name, err)
        }
    }

    return mutator.Apply()
}
```

### Why use a mutator interface

Feature mutations are intentionally **not applied directly to the resource wrapper or the underlying Kubernetes object**.

Instead, feature mutations operate on a **restricted mutator** (for example `DeploymentResourceMutator`) that acts as 
a **mutation planner**.

At first glance this may seem unnecessary — after all, a feature mutation could simply receive `*appsv1.Deployment`.

However, the mutator interface exists for several important reasons.

### 1. It prevents uncontrolled mutation of the resource

If feature mutations receive the full resource object, they can modify anything.

That makes it very easy to accidentally introduce:

* destructive changes
* non-additive mutations
* logic that bypasses shared conventions

For example, a feature could accidentally replace an entire environment variable list instead of adding a single entry.

```go
// dangerous mutation style
container.Env = []corev1.EnvVar{ ... }
```

Once patterns like this spread across features, it becomes difficult to reason about how multiple features interact.

By restricting mutations to a **restricted mutator**, the resource controls **how mutations happen**, not just 
**when they happen**.

### 2. It enforces additive mutation patterns

The mutator interface acts as a **controlled mutation surface** that ensures mutations follow the design principles:
- **Additive-first**: Use `EnsureContainerEnvVar` to add environment variables or arguments instead of replacing slices.
- **Idempotent**: Applying the same mutation twice has no ill effect.
- **Composable**: Multiple features can modify the same resource safely via a shared planner.
- **Predictable**: The resource wrapper controls exactly how and when fields are updated.

### 3. It keeps mutation semantics consistent across features

Without the mutator interface, every feature would implement its own logic for modifying Kubernetes objects.

This often leads to:

* repeated slice scanning
* inconsistent mutation styles
* duplicate env vars or args
* unpredictable ordering behavior

The mutator interface centralizes these patterns inside the resource implementation.

This provides two major benefits:

* feature authors do not need to reimplement low-level mutation logic
* mutation behavior stays consistent across the operator

### 4. It allows the resource to implement efficient mutation helpers

Another benefit of the mutator abstraction is that the resource can maintain **internal indexes** or caches to make 
repeated mutations efficient.

For example, when multiple features add environment variables, repeatedly scanning the container’s env list becomes inefficient.

A mutator implementation can build lookup maps once and reuse them across all feature mutations.

This keeps feature logic simple while allowing the resource wrapper to implement efficient mutation internally.

### 5. It preserves clear ownership boundaries

Feature mutations should describe **what to add or change**, not **how the Kubernetes object is structured internally**.

By introducing a mutator interface, the resource wrapper retains control over:

* how containers are located
* how slices are modified
* how duplicates are avoided
* how internal indexes are maintained

Feature mutations remain focused on **feature intent**, not Kubernetes implementation details.

### Recommended implementation style

When writing feature mutations, developers should treat the mutator interface as the **only supported way to modify the 
resource**. The goal is not simply to provide convenience helpers, but to enforce a **disciplined mutation model**.

Feature mutations should express a clear and narrow intent, modifying only the fields necessary for the feature while 
relying on the resource wrapper to handle mutation details.

A tracing feature implemented as a version-gated mutation:

```go
func TracingFeature(version string, enabled bool) feature.Mutation[*resources.DeploymentResourceMutator] {
    return feature.Mutation[*resources.DeploymentResourceMutator]{
        Name: "tracing",
        Feature: feature.NewResourceFeature(version, []feature.VersionConstraint{
            feats.FromSemver(">=8.1.0"),
        }).When(enabled),
        Mutate: func(m *resources.DeploymentResourceMutator) error {
            // Narrow intent: only ensure the environment variable is present
            m.EnsureContainerEnvVar("ENABLE_TRACING", "true")
            return nil
        },
    }
}
```

This keeps feature logic clear, safe, and composable without directly manipulating Kubernetes slices.

### Separation of responsibilities

The mutator interface intentionally separates two concerns.

#### Feature logic

Feature mutations define **what behavior should be enabled**.

Examples:

* enable tracing
* enable legacy compatibility
* inject a sidecar
* configure a probe

Feature mutations should remain small and declarative.

#### Resource mutation mechanics

The resource wrapper defines **how the Kubernetes object is modified safely**.

Examples:

* ensuring env vars are unique
* avoiding duplicate args
* maintaining lookup indexes
* managing slice mutations safely

These responsibilities belong to the resource wrapper, not individual features.

### Why this improves long-term maintainability

This helps ensure that as an operator grows:

* resource definitions remain readable
* feature logic remains modular
* mutation behavior remains consistent
* feature interactions remain predictable

### The restricted mutator

Feature mutations operate through a restricted mutator interface rather than modifying the resource directly. The 
mutator acts as a **mutation planner**, gathering operations and applying them in a final phase.

This allows the wrapper to provide **safe helper methods** for additive mutations.

For a complete example of a mutator implementation, see the [deployment_mutator.go](/examples/component-architecture-basics/resources/deployment_mutator.go) file.

### Registering feature mutations

A builder can register mutations like this:

```go
func (b *DeploymentBuilder) WithMutation(
    mutation feature.Mutation[*DeploymentResourceMutator],
) *DeploymentBuilder {
    b.res.mutations = append(b.res.mutations, mutation)
    return b
}
```

#### Example feature mutations

An example feature mutation for tracing:

```go
func TracingFeature(version string, enabled bool) feature.Mutation[*resources.DeploymentResourceMutator] {
    return feature.Mutation[*resources.DeploymentResourceMutator]{
        Name: "tracing",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{
                feats.FromSemver(">=8.1.0"),
            },
        ).When(enabled),
        Mutate: func(mutator *resources.DeploymentResourceMutator) error {
            mutator.EnsureContainerEnvVar("ENABLE_TRACING", "true")
            return nil
        },
    }
}
```

### Using the builder

```go
deployment := resources.NewDeploymentBuilder("my-app", namespace).
    WithMutation(features.TracingFeature(version, owner.Spec.TracingEnabled)).
    WithMutation(features.LegacyCompatibilityFeature(version)).
    Build()
```

### Why this model works

#### The baseline stays readable

The resource clearly expresses the intended modern desired state.

#### Feature behavior becomes explicit

Optional behavior is registered as isolated mutations rather than hidden conditionals.

#### Version gating is localized

Semver logic no longer spreads across resource construction.

#### Composition becomes the default

Feature logic becomes small, composable building blocks.

#### The code reflects the present, not the history

The resource implementation describes the **baseline desired state**, not the path that led there.

### Summary

Feature mutations solve a practical problem: resource construction becomes difficult to understand when version 
compatibility and optional behavior are layered directly into the core resource.

The model is simple:
1. Define the baseline desired state.
2. Register version-aware feature mutations.
3. Apply enabled mutations in a controlled sequence.
4. Keep mutations additive, idempotent, and composable.

This allows an operator to evolve safely as features and versions accumulate, without turning resource construction
into an unmaintainable set of conditionals.

### Conclusion

By combining **Components** for high-level orchestration with **Feature Mutations** for modular resource configuration,
this framework provides a complete architecture for modern Kubernetes operators. The result is a codebase that remains 
readable, testable, and maintainable as it evolves to support new features and application versions.