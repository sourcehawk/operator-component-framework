# Operator Component Framework

[![Go Reference](https://pkg.go.dev/badge/github.com/sourcehawk/operator-component-framework.svg)](https://pkg.go.dev/github.com/sourcehawk/operator-component-framework)
[![Go Report Card](https://goreportcard.com/badge/github.com/sourcehawk/operator-component-framework)](https://goreportcard.com/report/github.com/sourcehawk/operator-component-framework)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A Go framework for building highly maintainable Kubernetes operators using a behavioral component model and
version-gated feature mutations.

---

## Overview

Kubernetes operators tend to accumulate complexity over time: reconciliation functions grow large, lifecycle logic is
duplicated across resources, status reporting becomes inconsistent, and version-compatibility code gets tangled into
orchestration. The Operator Component Framework addresses these problems through a clear layered architecture.

The framework organizes operator logic into three composable layers:

- **Components** — logical feature units that reconcile multiple resources together and report a single user-facing
  condition.
- **Resource Primitives** — reusable, type-safe wrappers for individual Kubernetes objects with built-in lifecycle
  semantics.
- **Feature Mutations** — composable, version-gated modifications that keep baseline resource definitions clean while
  managing optional and historical behavior explicitly.

## Mental Model

```
Controller
  └─ Component
      └─ Resource Primitive
           └─ Kubernetes Object
```

| Layer                  | Responsibility                                                                          |
| ---------------------- | --------------------------------------------------------------------------------------- |
| **Controller**         | Determines which components should exist; orchestrates reconciliation at a high level   |
| **Component**          | Represents one logical feature; reconciles its resources and reports a single condition |
| **Resource Primitive** | Encapsulates desired state and lifecycle behavior for a single Kubernetes object        |
| **Kubernetes Object**  | The raw `client.Object` (e.g. `Deployment`) persisted to the cluster                    |

## Features

- **Structured reconciliation** with predictable, phased lifecycle management
- **Condition aggregation** across multiple resources into a single component condition
- **Grace period support** to avoid premature degraded status during normal operations like rolling updates
- **Suspension handling** with configurable behavior — scale to zero, delete, or custom logic
- **Version-gated mutations** to apply backward-compatibility patches only when needed
- **Composable mutation layers** that stack without interfering with each other
- **Built-in lifecycle interfaces** (`Alive`, `Graceful`, `Suspendable`, `Completable`, `Operational`,
  `DataExtractable`) covering the full range of Kubernetes workload types
- **Typed mutation editors** for kubernetes resource primitives
- **Metrics and event recording** integrations out of the box

## Installation

```bash
go get github.com/sourcehawk/operator-component-framework
```

Requires Go 1.21+ and a project using [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

## Quick Start

The following example builds a component that manages a single `Deployment`, with an optional tracing feature applied as
a mutation.

```go
import (
    "time"
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/sourcehawk/operator-component-framework/pkg/component"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
)

func buildWebInterfaceComponent(owner *MyOperatorCR) (*component.Component, error) {
    // 1. Define the baseline resource
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "web-server",
            Namespace: owner.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app": "web-server"},
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{"app": "web-server"},
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {Name: "app", Image: "my-app:latest"},
                    },
                },
            },
        },
    }

    // 2. Build a resource primitive, applying optional feature mutations
    res, err := deployment.NewBuilder(dep).
        WithMutation(TracingFeature(owner.Spec.Version, owner.Spec.TracingEnabled)).
        Build()
    if err != nil {
        return nil, err
    }

    // 3. Assemble the component
    return component.NewComponentBuilder().
        WithName("web-interface").
        WithConditionType("WebInterfaceReady").
        WithResource(res, component.ResourceOptions{}).
        WithGracePeriod(5 * time.Minute).
        Suspend(owner.Spec.Suspended).
        Build()
}

// 4. Reconcile from your controller
func (r *MyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    owner := &MyOperatorCR{}
    if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    comp, err := buildWebInterfaceComponent(owner)
    if err != nil {
        return reconcile.Result{}, err
    }

    recCtx := component.ReconcileContext{
        Client:   r.Client,
        Scheme:   r.Scheme,
        Recorder: r.Recorder,
        Owner:    owner,
    }

    return reconcile.Result{}, comp.Reconcile(ctx, recCtx)
}
```

## Feature Mutations

Mutations decouple version-specific or feature-gated logic from the baseline resource definition. A mutation declares a
condition under which it applies and a function that modifies the resource.

```go
import (
    corev1 "k8s.io/api/core/v1"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
)

func TracingFeature(version string, enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "enable-tracing",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *deployment.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
                e.EnsureEnvVar(corev1.EnvVar{Name: "TRACING_ENABLED", Value: "true"})
                return nil
            })
            return nil
        },
    }
}
```

Mutations are applied in registration order. Each mutation is independent — multiple mutations can target the same
resource without interfering with each other, and the framework guarantees a consistent application sequence.

## Resource Lifecycle Interfaces

Resource primitives implement behavioral interfaces that the component layer uses for status aggregation:

| Interface         | Behavior                                          | Example resources                       |
| ----------------- | ------------------------------------------------- | --------------------------------------- |
| `Alive`           | Observable health with rolling-update awareness   | Deployments, StatefulSets, DaemonSets   |
| `Graceful`        | Time-bounded convergence with degradation         | Workloads with slow rollouts            |
| `Suspendable`     | Controlled deactivation (scale to zero or delete) | Workloads, task primitives              |
| `Completable`     | Run-to-completion tracking                        | Jobs                                    |
| `Operational`     | External dependency readiness                     | Services, Ingresses, Gateways, CronJobs |
| `DataExtractable` | Post-reconciliation data harvest                  | Any resource exposing status fields     |

## Implementing a Custom Resource

You can wrap any Kubernetes object — including custom CRDs — by implementing the `Resource` interface:

```go
type Resource interface {
    // Object returns the desired-state Kubernetes object.
    Object() (client.Object, error)

    // Mutate receives the current cluster state and applies the desired state to it.
    Mutate(current client.Object) error

    // Identity returns a stable string that uniquely identifies this resource.
    Identity() string
}
```

Optionally implement any of the lifecycle interfaces (`Alive`, `Suspendable`, etc.) to participate in condition
aggregation.

See the [examples directory](examples/) for complete implementations.

## Documentation

| Document                                              | Description                                                          |
| ----------------------------------------------------- | -------------------------------------------------------------------- |
| [Component Framework](docs/component.md)              | Reconciliation lifecycle, condition model, grace periods, suspension |
| [Resource Primitives](docs/primitives.md)             | Primitive categories, field application pipeline, mutation system    |
| [Deployment Primitive](docs/primitives/deployment.md) | Deployment-specific mutation ordering and editors                    |

## Examples

The [examples directory](examples/) contains runnable, end-to-end implementations:

| Example                                                                      | Description                                                                                |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| [`deployment-primitive`](examples/deployment-primitive/)                     | Core Deployment primitive: mutations, suspension, data extraction                          |
| [`custom-resource-implementation`](examples/custom-resource-implementation/) | Full custom resource wrapper implementing lifecycle interfaces and version-gated mutations |

Run any example with:

```bash
go run examples/<example-name>/main.go
```

## Project Structure

```
pkg/
├── component/          # Component framework: builder, reconciliation, conditions
│   └── concepts/       # Lifecycle interface definitions (Alive, Suspendable, …)
├── primitives/
│   └── deployment/     # Deployment primitive: builder, mutator, editors
├── feature/            # Feature and version-constraint types
├── mutation/
│   ├── editors/        # Typed mutation APIs (DeploymentSpec, PodSpec, Container, …)
│   └── selectors/      # Container selectors (ByName, ByIndex, All)
└── recording/          # Event recording helpers

examples/
├── deployment-primitive/
└── custom-resource-implementation/

docs/
├── component.md
├── primitives.md
└── primitives/deployment.md
```

## Contributing

Contributions are welcome. Please open an issue to discuss significant changes before submitting a pull request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes
4. Open a pull request against `main`

All new code should include tests. The project uses [Ginkgo](https://github.com/onsi/ginkgo) and
[Gomega](https://github.com/onsi/gomega) for testing.

```bash
go test ./...
```

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
