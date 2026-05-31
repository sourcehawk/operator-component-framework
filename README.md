# Operator Component Framework

[![Go Reference](https://pkg.go.dev/badge/github.com/sourcehawk/operator-component-framework.svg)](https://pkg.go.dev/github.com/sourcehawk/operator-component-framework)
[![Go Report Card](https://goreportcard.com/badge/github.com/sourcehawk/operator-component-framework)](https://goreportcard.com/report/github.com/sourcehawk/operator-component-framework)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A Go framework for building Kubernetes operators that stay maintainable as they grow. It pulls reconciliation mechanics,
status reporting, and lifecycle behavior into reusable building blocks (**components** and **resource primitives**), so
your controllers stay thin and focused on construction and orchestration, without sacrificing customizability where it
matters.

<p align="center">
  <img width="400" height="400" alt="OCF Logo" src="https://github.com/user-attachments/assets/ac8162e7-5963-4284-a590-5a9215090103" />
</p>

> [!NOTE]
>
> This framework is not a replacement for [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
> It is a library you use inside controller-runtime reconcilers, such as in Kubebuilder-generated projects, to manage
> the layers between the reconciler and the Kubernetes resources it manages.

---

## Architecture

An operator built with this framework has two layers between the controller and raw Kubernetes objects:

```mermaid
graph TB
    subgraph controller [" "]
        R["⚪ Your Reconciler"]
    end

    subgraph components [" "]
        C1["🔵 Web Interface component"]
        C2["🔵 Monitoring component"]
    end

    subgraph primitives [" "]
        P1["🟢 ConfigMap"]
        P2["🟢 Deployment"]
        P3["🟢 Service"]
        P4["🟢 ServiceAccount"]
        P5["🟢 DaemonSet"]
    end

    subgraph cluster [" "]
        K["⚪ Kubernetes API"]
    end

    R --> C1 & C2
    C1 --> P1 & P2 & P3
    C2 --> P4 & P5
    P1 & P2 & P3 & P4 & P5 --> K
```

> ⚪ What you already have &emsp; 🔵 OCF component layer &emsp; 🟢 OCF primitive layer

## Features

**Reconciliation and health**

- **Predictable status management** with consistent condition reporting aggregated from all managed resources
- **Grace periods** allow time for resources to converge before reporting degraded or down status
- **Suspend and resume** entire components with configurable behavior (scale to zero, delete, or custom logic)
- **Lifecycle-aware primitives** for deployments, jobs, services, and more, each reporting health in a way that fits its
  category

**Feature management**

- **Version-gated mutations** apply patches only when a version constraint matches, keeping the baseline clean
- **Stackable mutations** that compose independently on the same resource without conflicts
- **Typed editors and selectors** for modifying containers, pod specs, metadata, and other resource fields safely
- **Feature gates** to enable or disable entire components or individual resources based on flags or version ranges

**Orchestration**

- **Resource guards** block a resource (and everything after it) until a precondition is met
- **Data extraction** harvests values from one resource and makes them available to subsequent guards and mutations
- **Prerequisites** express startup ordering between components (e.g., "wait for the database before starting the API")
- **Metrics and event recording** integrations out of the box

## Installation

```bash
go get github.com/sourcehawk/operator-component-framework
```

Requires Go 1.25.6+ and [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) v0.22 or later. See
[Compatibility](docs/compatibility.md) for the full support matrix.

## Quick Start

The following example builds a component that manages a ConfigMap, Deployment, and Service together. Each resource is
built by its own function, mutations are defined separately, and a component function composes everything into a single
reconcilable unit.

### Resource builders

Each function returns a `component.Resource` wrapping a single Kubernetes object. The framework provides typed
[primitive builders](docs/primitives.md) for common resource types.

```go
import (
    "time"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/sourcehawk/operator-component-framework/pkg/component"
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/service"
)

func NewWebConfig(owner *MyOperatorCR) (component.Resource, error) {
    return configmap.NewBuilder(&corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{Name: "web-config", Namespace: owner.Namespace},
        Data:       map[string]string{"log-level": owner.Spec.LogLevel},
    }).Build()
}

func NewWebDeployment(owner *MyOperatorCR) (component.Resource, error) {
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{Name: "web-server", Namespace: owner.Namespace},
        Spec: appsv1.DeploymentSpec{
            Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web-server"}},
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web-server"}},
                Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}}},
            },
        },
    }
    return deployment.NewBuilder(dep).
        WithMutation(TracingFeature(owner.Spec.TracingEnabled)).
        WithMutation(LegacyPortConfig(owner.Spec.Version)).
        Build()
}

func NewWebService(owner *MyOperatorCR) (component.Resource, error) {
    return service.NewBuilder(&corev1.Service{
        ObjectMeta: metav1.ObjectMeta{Name: "web-server", Namespace: owner.Namespace},
        Spec: corev1.ServiceSpec{
            Selector: map[string]string{"app": "web-server"},
            Ports:    []corev1.ServicePort{{Port: 8080}},
        },
    }).Build()
}
```

### Feature mutations

[Mutations](docs/primitives.md#mutation-system) decouple version-specific or feature-gated logic from the baseline
resource definition. Each mutation declares a condition under which it applies and a function that modifies the resource
through typed [editors](docs/primitives.md#mutation-editors) and
[container selectors](docs/primitives.md#container-selectors).

A boolean-gated mutation applies only when a flag is set:

```go
func TracingFeature(enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "enable-tracing",
        Feature: feature.NewVersionGate("", nil).When(enabled),
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

A version-gated mutation applies only when the current version satisfies a constraint. This is useful for backward
compatibility: the baseline reflects the latest shape, and mutations patch it back for older versions.

```go
func LegacyPortConfig(version string) deployment.Mutation {
    return deployment.Mutation{
        Name: "legacy-port-config",
        Feature: feature.NewVersionGate(version, []feature.VersionConstraint{
            LessThan("2.0.0"), // user-provided VersionConstraint implementation
        }),
        Mutate: func(m *deployment.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
                e.Raw().Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}
                return nil
            })
            return nil
        },
    }
}
```

Mutations are applied in registration order. Multiple mutations can target the same resource without interfering with
each other, and the framework guarantees a consistent application sequence.

### Component

The [component](docs/component.md) composes resources into a single reconcilable unit with one condition on the owner
object. Resources are reconciled in registration order, so the ConfigMap exists before the Deployment is applied.

```go
func NewWebInterfaceComponent(owner *MyOperatorCR) (*component.Component, error) {
    configMap, err := NewWebConfig(owner)
    if err != nil {
        return nil, err
    }
    deployment, err := NewWebDeployment(owner)
    if err != nil {
        return nil, err
    }
    service, err := NewWebService(owner)
    if err != nil {
        return nil, err
    }

    return component.NewComponentBuilder().
        WithName("web-interface").
        WithConditionType("WebInterfaceReady").
        WithResource(configMap).
        WithResource(deployment).
        WithResource(service).
        WithGracePeriod(5 * time.Minute).
        Suspend(owner.Spec.Suspended).
        Build()
}
```

### Reconciliation

The controller builds the component and hands it to the framework.

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (_ reconcile.Result, err error) {
    owner := &MyOperatorCR{}
    if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    recCtx := component.ReconcileContext{
        Client:   r.Client,
        Scheme:   r.Scheme,
        Recorder: r.Recorder,
        Metrics:  r.Metrics,
        Owner:    owner,
    }
    defer func() {
        if flushErr := component.FlushStatus(ctx, recCtx); flushErr != nil && err == nil {
            err = flushErr
        }
    }()

    comp, err := NewWebInterfaceComponent(owner)
    if err != nil {
        return reconcile.Result{}, err
    }

    return reconcile.Result{}, comp.Reconcile(ctx, recCtx)
}
```

Components stage their conditions on `owner` in memory; a single deferred `component.FlushStatus` at the end of the
reconcile loop persists every condition with one `Status().Update` call. This keeps controllers with multiple components
free of self-induced 409 conflicts.

## Beyond the Basics

The Quick Start shows the common path. The sections below highlight capabilities that matter once your operator grows
beyond a single resource.

### Guards and Data Extraction

Resources are reconciled in order. A [data extractor](docs/primitives.md#lifecycle-interfaces) on an earlier resource
can feed a [guard](docs/component.md#guards) on a later one, letting you express dependencies between resources within a
single component.

```go
func NewDatabaseConfig(owner *MyOperatorCR, dbHost *string) (component.Resource, error) {
    return configmap.NewBuilder(baseCM).
        WithDataExtractor(func(cm corev1.ConfigMap) error {
            *dbHost = cm.Data["database-host"]
            return nil
        }).
        Build()
}

func NewAppDeployment(owner *MyOperatorCR, dbHost *string) (component.Resource, error) {
    return deployment.NewBuilder(baseDep).
        WithGuard(func(_ appsv1.Deployment) (concepts.GuardStatusWithReason, error) {
            if dbHost == nil || *dbHost == "" {
                return concepts.GuardStatusWithReason{
                    Status: concepts.GuardStatusBlocked,
                    Reason: "waiting for database host from ConfigMap",
                }, nil
            }
            return concepts.GuardStatusWithReason{Status: concepts.GuardStatusUnblocked}, nil
        }).
        Build()
}
```

When a guard blocks, the component reports a `Blocked` condition and skips all subsequent resources in the pipeline.

### Component Prerequisites and Feature Gates

[Prerequisites](docs/component.md#prerequisites) express startup ordering between components.
[Feature gates](docs/component.md#component-feature-gates) conditionally enable or disable an entire component: when
disabled, all its resources are deleted and the condition reports `Disabled`.

```go
func NewAPIGatewayComponent(owner *MyOperatorCR) (*component.Component, error) {
    // ... build gateway deployment, gateway service ...

    return component.NewComponentBuilder().
        WithName("api-gateway").
        WithConditionType("APIGatewayReady").
        WithFeatureGate(feature.NewVersionGate(version, versionConstraints).When(spec.GatewayEnabled)).
        WithPrerequisite(component.DependsOn("DatabaseReady")).
        WithResource(gatewayDep).
        WithResource(gatewaySvc).
        Build()
}
```

### Resource Options

Individual resources can be [feature-gated, read-only, or auxiliary](docs/component.md#resource-registration-options)
within a component.

```go
// Feature-gated: created when enabled, deleted when disabled.
builder.WithResource(metricsExporter, component.GatedBy(metricsGate), component.Auxiliary())
builder.WithResource(externalCRD, component.ReadOnly())
```

### Built-in Primitives

The framework ships with primitives for the most common Kubernetes resource types. Each primitive provides a typed
builder, mutation system, and the appropriate lifecycle interfaces for its category.

| Category         | Primitives                                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------- |
| **Workload**     | Deployment, StatefulSet, DaemonSet, ReplicaSet, Pod                                                                       |
| **Task**         | Job, CronJob                                                                                                              |
| **Static**       | ConfigMap, Secret, ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding, PodDisruptionBudget, NetworkPolicy |
| **Integration**  | Service, Ingress, PersistentVolume, PersistentVolumeClaim, HorizontalPodAutoscaler                                        |
| **Unstructured** | Static, Workload, Task, and Integration variants for any GVK without a built-in wrapper                                   |

For details on each primitive, see [Resource Primitives](docs/primitives.md).

## Resource Lifecycle Interfaces

Resource primitives implement behavioral interfaces that the component layer uses for status aggregation:

| Interface         | Behavior                                          | Example resources                            |
| ----------------- | ------------------------------------------------- | -------------------------------------------- |
| `Alive`           | Observable health with rolling-update awareness   | Deployments, StatefulSets, DaemonSets        |
| `Graceful`        | Time-bounded convergence with degradation         | Workloads or integrations with slow rollouts |
| `Suspendable`     | Controlled deactivation (scale to zero or delete) | Workloads, task primitives                   |
| `Completable`     | Run-to-completion tracking                        | Jobs                                         |
| `Operational`     | External dependency readiness                     | Services, Ingresses, Gateways, CronJobs      |
| `DataExtractable` | Post-reconciliation data harvest                  | Any resource exposing status fields          |
| `Guardable`       | Precondition gating before resource application   | Resources dependent on prior resource state  |

## Implementing a Custom Resource

You can wrap any Kubernetes object, including custom CRDs, by implementing the `Resource` interface:

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
aggregation. The framework provides generic building blocks in `pkg/generic` that handle reconciliation mechanics,
mutation sequencing, and suspension so you can wrap any custom CRD without reimplementing these from scratch.

See the [Custom Resource Implementation Guide](docs/custom-resource.md) for a complete walkthrough.

## Documentation

| Document                                    | Description                                                             |
| ------------------------------------------- | ----------------------------------------------------------------------- |
| [Component Framework](docs/component.md)    | Reconciliation lifecycle, condition model, grace periods, suspension    |
| [Resource Primitives](docs/primitives.md)   | Primitive categories, Server-Side Apply, mutation system                |
| [Custom Resources](docs/custom-resource.md) | Implementing custom resource wrappers using the generic building blocks |
| [Guidelines](docs/guidelines.md)            | Recommended patterns for structuring operators built with the framework |
| [Compatibility](docs/compatibility.md)      | Supported Kubernetes and controller-runtime versions, version policy    |

## Contributing

Contributions are welcome. Please open an issue to discuss significant changes before submitting a pull request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes
4. Open a pull request against `main`

All new code should include tests. The project uses [testify](https://github.com/stretchr/testify),
[Ginkgo](https://github.com/onsi/ginkgo) and [Gomega](https://github.com/onsi/gomega) for testing.

```bash
go test ./...
```

## Further Reading

- [The Missing Layers in Your Kubernetes Operator](https://medium.com/@sourcehawk/the-missing-layers-in-your-kubernetes-operator-306ee8633350) -
  a walkthrough of common structural problems in Kubernetes operators and how the framework addresses them.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
