# Getting Started

This tutorial builds your first component from scratch: one component that manages a ConfigMap and a Deployment, reports
a single condition on the owner object, and gates one piece of behavior behind a boolean flag. By the end you have a
working reconcile loop and a golden test pinning the rendered output.

Every snippet here is taken from the `mutations-and-gating` example in the repository. If you want the finished code
side by side, open `examples/mutations-and-gating`.

## Requirements

- A controller-runtime project, such as one scaffolded with [Kubebuilder](https://book.kubebuilder.io/). This framework
  is not a replacement for [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime); it is a library
  you use inside a reconciler to manage the layers between the reconciler and the Kubernetes objects it manages.
- An owner CRD type whose status implements `GetStatusConditions() *[]metav1.Condition`. The framework stages and
  flushes conditions through this accessor.
- The module installed:

```bash
go get github.com/sourcehawk/operator-component-framework
```

See [Compatibility](compatibility.md) for the supported Go and controller-runtime versions.

## What you will build

A single component named `example-app` with the condition type `AppReady`. It manages:

- A **ConfigMap** holding the application configuration.
- A **Deployment** running the application container, with one boolean-gated mutation that sets `LOG_LEVEL=debug` when a
  spec flag is enabled.

The reconciler builds the component and hands it to the framework, which applies the resources, aggregates their health,
and writes one condition back to the owner.

## Step 1: Define the owner CRD

The framework only requires one thing of your CRD: the status type exposes its conditions through `GetStatusConditions`.
Here is a minimal owner with a version, one feature flag, and a conditions slice.

```go
package app

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExampleAppSpec defines the desired state of ExampleApp.
type ExampleAppSpec struct {
    // Version of the application to deploy.
    Version string `json:"version"`

    // EnableDebugLogging sets LOG_LEVEL=debug on the application container.
    EnableDebugLogging bool `json:"enableDebugLogging"`

    // Suspended determines whether the application is active.
    Suspended bool `json:"suspended"`
}

// ExampleAppStatus defines the observed state of ExampleApp.
type ExampleAppStatus struct {
    // Conditions store the status of the application's components.
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExampleApp is the owner CRD managed by the operator.
type ExampleApp struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   ExampleAppSpec   `json:"spec,omitempty"`
    Status ExampleAppStatus `json:"status,omitempty"`
}

// GetStatusConditions returns the status conditions for the ExampleApp.
// The framework reads and writes the owner's conditions through this accessor.
func (in *ExampleApp) GetStatusConditions() *[]metav1.Condition {
    return &in.Status.Conditions
}
```

A real CRD also implements `runtime.Object` (the `DeepCopyObject` method and a list type) and registers with a scheme.
Kubebuilder generates that boilerplate for you. The full shared type lives in `examples/shared/app/owner.go`.

## Step 2: Build the ConfigMap primitive

A primitive wraps one Kubernetes object. Start by writing a function that returns the desired-state object, then build a
`component.Resource` from it with the primitive builder.

```go
package resources

import (
    "github.com/sourcehawk/operator-component-framework/pkg/component"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "your.module/app"
)

// BaseConfigMap returns the desired-state ConfigMap for the given owner.
func BaseConfigMap(owner *app.ExampleApp) *corev1.ConfigMap {
    return &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      owner.Name + "-config",
            Namespace: owner.Namespace,
            Labels:    map[string]string{"app": owner.Name},
        },
        Data: map[string]string{
            "app.yaml": "server:\n  port: 8080\n  timeout: 30s\n",
        },
    }
}

// NewConfigMapResource builds the ConfigMap resource.
func NewConfigMapResource(owner *app.ExampleApp) (component.Resource, error) {
    return configmap.NewBuilder(BaseConfigMap(owner)).Build()
}
```

`NewBuilder` takes the object, `Build` returns a `component.Resource` ready to register on a component.

## Step 3: Build the Deployment primitive

The Deployment is built the same way, with one addition: a boolean-gated mutation. A mutation is a named edit that the
framework applies only when its feature gate is active. Defining the edit separately from the baseline keeps the
baseline readable and the conditional behavior testable on its own.

First, the baseline. Define it as the latest version's desired shape and leave version-dependent fields out of it. This
is the baseline-as-latest convention; the [Guidelines](guidelines.md) explain why it pays off.

```go
package resources

import (
    "fmt"

    "github.com/sourcehawk/operator-component-framework/pkg/component"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "your.module/app"
)

// BaseDeployment returns the desired-state Deployment for the given owner.
func BaseDeployment(owner *app.ExampleApp) *appsv1.Deployment {
    return &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      owner.Name + "-app",
            Namespace: owner.Namespace,
            Labels:    map[string]string{"app": owner.Name},
        },
        Spec: appsv1.DeploymentSpec{
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app": owner.Name},
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{"app": owner.Name},
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name:  "app",
                            Image: fmt.Sprintf("my-app:%s", owner.Spec.Version),
                        },
                    },
                },
            },
        },
    }
}
```

Now the mutation. It targets the container named `app` and sets an environment variable. The gate is built with
`feature.NewBooleanGate(enabled)`: a gate with no version constraints whose result is driven purely by the boolean. When
`enabled` is `false`, the framework skips the edit.

```go
package features

import (
    "github.com/sourcehawk/operator-component-framework/pkg/feature"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
    "github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
    "github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
    corev1 "k8s.io/api/core/v1"
)

// DebugLoggingMutation sets LOG_LEVEL=debug on the application container when enabled.
func DebugLoggingMutation(enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "DebugLogging",
        Feature: feature.NewBooleanGate(enabled),
        Mutate: func(m *deployment.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("app"), func(ce *editors.ContainerEditor) error {
                ce.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
                return nil
            })
            return nil
        },
    }
}
```

Register the mutation when building the resource. Mutations apply in registration order.

```go
// NewDeploymentResource builds the Deployment resource with its mutations.
func NewDeploymentResource(owner *app.ExampleApp) (component.Resource, error) {
    return deployment.NewBuilder(BaseDeployment(owner)).
        WithMutation(features.DebugLoggingMutation(owner.Spec.EnableDebugLogging)).
        Build()
}
```

!!! note "Version-gated mutations"

    The same gate does version gating. Pass a non-empty version and a constraint slice instead of a boolean, and the
    mutation fires only when the version satisfies the constraint. This is how backward-compatibility patches are
    expressed without touching the baseline:

    ```go
    Feature: feature.NewVersionGate(owner.Spec.Version, []feature.VersionConstraint{
        mustConstraint("< 2.0.0"), // a VersionConstraint you supply, e.g. backed by a semver library
    }),
    ```

    The framework does not ship a `VersionConstraint` implementation, so you provide one (a few lines wrapping a semver
    library). The
    [`mutations-and-gating` example](https://github.com/sourcehawk/operator-component-framework/tree/main/examples/mutations-and-gating)
    wires a real one in its backward-compatibility mutation. See
    [Version-Gated Mutations](primitives.md#version-gated-mutations) for the full pattern and
    [Guidelines](guidelines.md) for the baseline-as-latest approach.

## Step 4: Compose the component

A component groups resources under one name and one condition type. Register resources in dependency order; they
reconcile in the order you add them.

```go
comp, err := component.NewComponentBuilder().
    WithName("example-app").
    WithConditionType("AppReady").
    WithResource(deployResource).
    WithResource(cmResource).
    Suspend(owner.Spec.Suspended).
    Build()
if err != nil {
    return err
}
```

`Suspend(true)` deactivates the component's suspendable resources (the Deployment scales to zero) without deleting the
component's record of them.

## Step 5: Wire the reconciler

The controller stays thin. It builds the resources, builds the component, and calls `Reconcile`. A `ReconcileContext`
carries the client, scheme, recorders, and owner. A single deferred `component.FlushStatus` persists every staged
condition with one status update at the end of the loop, which keeps controllers with multiple components free of
self-induced update conflicts.

`ReconcileContext` has five fields:

| Field           | Type                        | Notes                                                                                                                                                     |
| --------------- | --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Client`        | `client.Client`             | The controller-runtime client.                                                                                                                            |
| `Scheme`        | `*runtime.Scheme`           | The operator scheme.                                                                                                                                      |
| `EventRecorder` | `events.EventRecorder`      | For Kubernetes events. `GetEventRecorder(name)` on the manager, v0.23 and later.                                                                          |
| `Metrics`       | `component.MetricsRecorder` | Optional. Pass `nil` to skip status-condition metrics.                                                                                                    |
| `APIReader`     | `client.Reader`             | Optional, recommended. `GetAPIReader()` on the manager; `FlushStatus` reads through it on a 409 so the retry sees the live owner, not the informer cache. |
| `Owner`         | `component.OperatorCRD`     | The owner object you fetched. Your CRD satisfies this via `GetStatusConditions`.                                                                          |

!!! note

    `FlushStatus` performs the status write itself: it calls `Client.Status().Update` on the owner (retrying on
    conflict). Do not also call `Status().Update` for the conditions the framework manages, or you will double-write.
    Because the call is deferred, the conditions are flushed even if `Reconcile` returns an error. Pass the components
    you built: on a conflict their condition types stay on the staged owner, while every other condition is refreshed
    from the server.

```go
package app

import (
    "context"

    "github.com/sourcehawk/operator-component-framework/pkg/component"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/events"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller struct {
    client.Client
    Scheme        *runtime.Scheme
    EventRecorder events.EventRecorder
    Metrics       component.MetricsRecorder

    NewDeploymentResource func(*ExampleApp) (component.Resource, error)
    NewConfigMapResource  func(*ExampleApp) (component.Resource, error)
}

func (r *Controller) reconcile(ctx context.Context, owner *ExampleApp) (err error) {
    recCtx := component.ReconcileContext{
        Client:        r.Client,
        Scheme:        r.Scheme,
        EventRecorder: r.EventRecorder,
        Metrics:       r.Metrics,
        APIReader:     r.APIReader,
        Owner:         owner,
    }
    // Declared before the deferred flush so the closure sees the component built below.
    var comps []*component.Component
    defer func() {
        if flushErr := component.FlushStatus(ctx, recCtx, comps); flushErr != nil && err == nil {
            err = flushErr
        }
    }()

    deployResource, err := r.NewDeploymentResource(owner)
    if err != nil {
        return err
    }

    cmResource, err := r.NewConfigMapResource(owner)
    if err != nil {
        return err
    }

    comp, err := component.NewComponentBuilder().
        WithName("example-app").
        WithConditionType("AppReady").
        WithResource(deployResource).
        WithResource(cmResource).
        Suspend(owner.Spec.Suspended).
        Build()
    if err != nil {
        return err
    }
    comps = append(comps, comp)

    return comp.Reconcile(ctx, recCtx)
}
```

The `reconcile` method above takes the owner directly so the framework logic is easy to read and test. In a Kubebuilder
project the generated entry point has the signature `Reconcile(ctx, req)`. Fetch the owner there, handle the not-found
case, and delegate to it:

```go
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    owner := &ExampleApp{}
    if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
        // Ignore not-found: the owner was deleted and its resources are
        // garbage-collected through their owner references.
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    return ctrl.Result{}, r.reconcile(ctx, owner)
}
```

This entry point uses `ctrl "sigs.k8s.io/controller-runtime"` for `ctrl.Request` and `ctrl.Result`. After a reconcile,
the owner's `Status.Conditions` carries one `AppReady` condition reflecting the aggregated health of the Deployment and
ConfigMap.

## Step 6: Test the resource

Pin the resource's rendered output against a checked-in snapshot. Build it through the same factory the reconciler uses,
then golden it: `golden.AssertYAML` does the comparison, `golden.WithScheme` makes the output carry `apiVersion` and
`kind` for typed objects, and the `-update` flag regenerates the snapshot.

```go
package resources_test

import (
    "flag"
    "testing"

    "github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
    "github.com/stretchr/testify/require"
    appsv1 "k8s.io/api/apps/v1"
    "k8s.io/apimachinery/pkg/runtime"

    "your.module/app"
    "your.module/resources"
)

var update = flag.Bool("update", false, "update golden files")

func TestDeploymentResource(t *testing.T) {
    scheme := runtime.NewScheme()
    require.NoError(t, appsv1.AddToScheme(scheme))

    owner := &app.ExampleApp{Spec: app.ExampleAppSpec{Version: "2.0.0", EnableDebugLogging: true}}
    owner.Name = "my-app"
    owner.Namespace = "default"

    // Build the resource exactly as the reconciler does.
    res, err := resources.NewDeploymentResource(owner)
    require.NoError(t, err)

    // The built resource implements golden.Previewer.
    previewer, ok := res.(golden.Previewer)
    require.True(t, ok)
    golden.AssertYAML(t, "testdata/deployment.yaml", previewer, golden.WithScheme(scheme), golden.Update(*update))
}
```

Generate the snapshot, then run the test normally to verify it stays stable:

```bash
go test ./resources -run TestDeploymentResource -update
go test ./resources -run TestDeploymentResource
```

Commit the generated `testdata/deployment.yaml` alongside the test. This resource test is one layer of a fuller
strategy: test each mutation against a baseline, assert which mutations fire at the resource level, and golden the whole
component with `golden.AssertComponentYAML`. See [Testing](testing.md) for all three layers and for version-matrix
generation across supported versions.

## Next steps

- [Component](component.md): the full lifecycle, status model, grace periods, suspension, guards, and prerequisites.
- [Guidelines](guidelines.md): the baseline-as-latest convention, one component per condition, thin controllers, and
  version-gated mutations in practice.
- [Testing](testing.md): golden snapshots in depth and version-matrix golden generation across supported versions.
