# Guidelines

Recommendations for structuring operators built with the framework. These are not hard rules. They reflect patterns that
are effective and pitfalls that are easy to walk into.

## Table of Contents

- [Represent Desired State in the Baseline Object](#represent-desired-state-in-the-baseline-object)
- [One Component Per Logical Condition](#one-component-per-logical-condition)
- [Keep Controllers Thin](#keep-controllers-thin)
- [Resource Registration Order Is Execution Order](#resource-registration-order-is-execution-order)
- [Use Data Extraction and Guards for Resource Dependencies](#use-data-extraction-and-guards-for-resource-dependencies)
- [Mutations Describe Intent, Not Observation](#mutations-describe-intent-not-observation)
- [Understand Participation Modes](#understand-participation-modes)
- [Use Feature Gating for Conditional Resources](#use-feature-gating-for-conditional-resources)
- [Grace Periods Are Convergence Time](#grace-periods-are-convergence-time)
- [Handle Cluster-Scoped Resources Explicitly](#handle-cluster-scoped-resources-explicitly)
- [Name Conditions for the Audience Reading Them](#name-conditions-for-the-audience-reading-them)

## Represent Desired State in the Baseline Object

The core object passed to a primitive builder should represent the latest desired state of the resource. When the
baseline evolves, mutations adapt to the new baseline, not the other way around. The baseline should never be held back
at a legacy shape to accommodate existing mutations. Mutations layer cross-cutting concerns and conditional features on
top of whatever the current baseline is.

```go
dep := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      owner.Name + "-web",
        Namespace: owner.Namespace,
        Labels:    map[string]string{"app": owner.Name},
    },
    Spec: appsv1.DeploymentSpec{
        Replicas: ptr.To(owner.Spec.Replicas),
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

res, err := deployment.NewBuilder(dep).
    WithMutation(TracingFeature(owner.Spec.TracingEnabled)).
    WithMutation(MetricsFeature(owner.Spec.MetricsEnabled)).
    Build()
```

The baseline captures the structural truth of the resource: its name, namespace, labels, selector, replica count, and
primary container image. Mutations handle orthogonal concerns like injecting a tracing sidecar or adding metrics
annotations. Each mutation is independently gated and does not depend on the baseline having been set up in a particular
way.

### Why this is worth the effort

The alternative is to start from a minimal or legacy object and build up the current shape through mutations:

```go
dep := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      owner.Name + "-web",
        Namespace: owner.Namespace,
    },
}

res, err := deployment.NewBuilder(dep).
    WithMutation(SetLabels(owner)).
    WithMutation(SetReplicas(owner)).
    WithMutation(SetSelector(owner)).
    WithMutation(SetImage(owner)).
    WithMutation(TracingFeature(owner.Spec.TracingEnabled)).
    Build()
```

This feels simpler at first because every field goes through the same mechanism. But over time it creates problems:

- **The baseline tells you nothing.** Reading the code requires tracing through every mutation to understand what the
  resource actually looks like. A new contributor cannot glance at the object literal and know the shape of the
  deployment.
- **Mutation ordering becomes load-bearing for structural fields.** `SetSelector` must run before anything that depends
  on the selector existing. `SetImage` must run before a version-aware mutation that patches the image tag. These
  ordering constraints are invisible and fragile. Cross-cutting mutations (tracing, metrics) should be
  order-independent, but mixing them with structural mutations means everything is implicitly ordered.
- **The baseline becomes frozen at a legacy shape.** When a new version of your operator changes the resource's
  structure (adds a port, changes the container name, adopts a new volume layout), you face a choice: update the
  baseline and fix the mutations that assumed the old shape, or add another mutation to patch the baseline forward. The
  second choice is easier in the moment, but each time you take it the baseline drifts further from reality. Eventually
  you have an empty shell with a stack of mutations that must run in the right order to produce a valid object.

When the baseline represents the latest desired state, these problems go away. The baseline is readable on its own.
Mutations are genuinely independent because they operate on a complete, valid object. When the resource shape changes,
you update the baseline and adjust any mutations that assumed the old shape. The mutations that need adjusting are only
the ones gated on legacy versions, and those mutations are explicitly about backward compatibility rather than silently
load-bearing.

### Legacy version mutations in practice

Suppose version 2.0 of your application renamed its container from `"server"` to `"app"` and added a health check port.
The baseline reflects the latest shape:

```go
dep := &appsv1.Deployment{
    // ...
    Spec: appsv1.DeploymentSpec{
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{
                    {
                        Name:  "app",                                          // Current name
                        Image: fmt.Sprintf("my-app:%s", owner.Spec.Version),
                        Ports: []corev1.ContainerPort{
                            {Name: "http", ContainerPort: 8080},
                            {Name: "health", ContainerPort: 8081},             // Added in 2.0
                        },
                    },
                },
            },
        },
    },
}

res, err := deployment.NewBuilder(dep).
    WithMutation(LegacyContainerName(owner.Spec.Version)).
    WithMutation(TracingFeature(owner.Spec.TracingEnabled)).
    Build()
```

The legacy mutation rolls the baseline back for older versions:

```go
func LegacyContainerName(version string) deployment.Mutation {
    return deployment.Mutation{
        Name: "legacy-container-name",
        Feature: feature.NewVersionGate(version, []feature.VersionConstraint{
            LessThan("2.0.0"),
        }),
        Mutate: func(m *deployment.Mutator) error {
            m.EditContainers(selectors.ContainerNamed("app"), func(e *editors.ContainerEditor) error {
                e.Raw().Name = "server"
                e.Raw().Ports = []corev1.ContainerPort{
                    {Name: "http", ContainerPort: 8080},   // No health port before 2.0
                }
                return nil
            })
            return nil
        },
    }
}
```

`LessThan` here is a user-provided implementation of `feature.VersionConstraint` that wraps a semver comparison. The
interface requires a single `Enabled(version string) (bool, error)` method, so you can use any semver library to
implement your constraints.

For version 2.0 and above, the gate is inactive and the baseline is applied as-is. For older versions, the mutation
adjusts the container name and ports back to the legacy shape. The mutation is explicitly about backward compatibility,
gated on the versions that need it, and will stop running entirely once those versions are no longer supported.

The key difference from the alternative approach: the baseline was updated to the 2.0 shape, and the mutation adapts
backward. If instead you had kept the baseline at the 1.x shape and added a mutation to patch it forward to 2.0, every
future version change would stack another forward-patching mutation on top, and the baseline would never reflect
reality.

### Verifying legacy mutations

When you update the baseline, you need confidence that older versions still produce the same object they did before. The
framework provides a `golden` package for this. `AssertYAML` accepts any resource that implements `PreviewObject`,
renders it to YAML, and compares the result against a golden file.

```go
import "github.com/sourcehawk/operator-component-framework/pkg/testing/golden"

var update = flag.Bool("update", false, "update golden files")

func TestDeploymentShape(t *testing.T) {
    tests := []struct {
        name    string
        version string
        golden  string
    }{
        {name: "v1.9", version: "1.9.0", golden: "testdata/deployment-v1.9.0.yaml"},
        {name: "v2.0", version: "2.0.0", golden: "testdata/deployment-v2.0.0.yaml"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            owner := &v1alpha1.MyApp{
                Spec: v1alpha1.MyAppSpec{Version: tt.version},
            }

            res, err := buildDeployment(owner)
            require.NoError(t, err)

            golden.AssertYAML(t, tt.golden, res, golden.Update(*update))
        })
    }
}
```

Each version you care about gets a golden file. When the baseline evolves, run `go test -update` to regenerate the
golden files, then review the diff. The current version's golden file updates to reflect the new shape, but legacy
version golden files should stay unchanged. If a baseline change accidentally breaks a legacy mutation, the snapshot
diff shows exactly what shifted.

A reasonable heuristic for the boundary: if a field is always present regardless of feature flags or version, it belongs
in the baseline. If it is conditional, it belongs in a mutation.

## One Component Per Logical Condition

Each component reports exactly one condition on the owner CRD's status. If your operator needs to report `DatabaseReady`
and `WebInterfaceReady` independently, those are two components.

```go
dbComp, err := component.NewComponentBuilder().
    WithName("database").
    WithConditionType("DatabaseReady").
    WithResource(statefulSet, component.ResourceOptions{}).
    WithResource(dbService, component.ResourceOptions{}).
    Build()

webComp, err := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(deployment, component.ResourceOptions{}).
    WithResource(ingress, component.ResourceOptions{}).
    Build()
```

Separate components give users and monitoring systems granular observability: "the database is down" is a different
signal from "the web interface is scaling." A problem in one component does not mask the status of another.

The cost is coordination. If two components depend on each other (e.g., the web interface needs the database to be
ready), you handle that dependency in the controller rather than within a single component, since guards and data
extraction only work within a single component's resource list.

### When to split vs. combine

**Split** when:

- Users would ask "is the database ready?" and "is the web interface ready?" as separate questions.
- Resources can be independently healthy, degraded, or suspended.
- Failure in one group should not mask the status of another.

**Combine** when:

- Resources only make sense as a unit (a deployment and its service, a job and its configmap).
- Reporting separate conditions would add noise without actionable information.
- Resources share guards or data extraction chains that would be awkward to split across components.

A deployment and its associated service are a common example of resources worth combining: the service has no useful
"ready" semantics independent of the deployment it fronts. Reporting them as one condition (`WebInterfaceReady`) is
clearer than splitting them into `DeploymentReady` and `ServiceReady`.

## Keep Controllers Thin

Controllers should fetch the owner, decide which components to build, and call `Reconcile()`. Business logic, resource
construction, and feature decisions belong in components and their resource builders.

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    owner := &v1alpha1.MyApp{}
    if err := r.Get(ctx, req.NamespacedName, owner); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    comp, err := buildWebComponent(owner)
    if err != nil {
        return reconcile.Result{}, err
    }

    return reconcile.Result{}, comp.Reconcile(ctx, component.ReconcileContext{
        Client:   r.Client,
        Scheme:   r.Scheme,
        Recorder: r.Recorder,
        Metrics:  r.Metrics,
        Owner:    owner,
    })
}
```

This keeps controller logic trivial to test (there is almost nothing to test) and makes component construction functions
independently testable as pure functions: owner in, component out, no cluster required.

## Resource Registration Order Is Execution Order

Resources are reconciled in the exact order they are registered with `WithResource()`. This is deliberate: guards and
data extractors depend on it.

If resource B needs data extracted from resource A, register A first:

```go
comp, err := component.NewComponentBuilder().
    WithName("cloud-resources").
    WithConditionType("CloudReady").
    WithResource(roleRes, component.ResourceOptions{}).    // Applied first, ARN extracted
    WithResource(bucketRes, component.ResourceOptions{}).  // Guard checks ARN, applied second
    Build()
```

Reading the `WithResource()` calls top to bottom tells you the execution order. There is no implicit dependency graph to
reconstruct. The flip side is that reordering these calls can silently break data flow between guards and extractors.
Document the dependency when it exists.

## Use Data Extraction and Guards for Resource Dependencies

When one resource depends on data from another, use a data extractor on the first resource and a guard on the second. Do
not assume a resource is ready simply because it was registered earlier.

```go
var roleARN string

roleRes, _ := static.NewBuilder(newCloudRole(owner)).
    WithDataExtractor(func(obj uns.Unstructured) error {
        arn, _, _ := unstructured.NestedString(obj.Object, "status", "arn")
        roleARN = arn
        return nil
    }).
    Build()

bucketRes, _ := static.NewBuilder(newCloudBucket(owner)).
    WithGuard(func(_ uns.Unstructured) (concepts.GuardStatusWithReason, error) {
        if roleARN == "" {
            return concepts.GuardStatusWithReason{
                Status: concepts.GuardStatusBlocked,
                Reason: "waiting for cloud provider role ARN",
            }, nil
        }
        return concepts.GuardStatusWithReason{Status: concepts.GuardStatusUnblocked}, nil
    }).
    Build()
```

The guard prevents the dependent resource from being applied until its precondition is met, and a blocked guard surfaces
as a `Blocked` condition reason so users can see why a resource has not been created yet. The shared variable
(`roleARN`) is scoped to the reconciliation call, which prevents state leakage between reconciles.

## Mutations Describe Intent, Not Observation

Mutations operate on the desired object, not the server's current state. If you need to make decisions based on the
resource's live state in the cluster, use a data extractor to observe it and feed the result into a mutation through a
closure variable.

```go
var currentReplicas int32

res, _ := deployment.NewBuilder(baseline).
    WithDataExtractor(func(d appsv1.Deployment) error {
        currentReplicas = d.Status.ReadyReplicas
        return nil
    }).
    WithMutation(deployment.Mutation{
        Name: "scale-aware-annotation",
        Mutate: func(m *deployment.Mutator) error {
            m.EditPodTemplateMetadata(func(meta *editors.ObjectMetaEditor) error {
                meta.EnsureAnnotation("app.example.com/last-known-ready",
                    fmt.Sprintf("%d", currentReplicas))
                return nil
            })
            return nil
        },
    }).
    Build()
```

This keeps mutations predictable: they always produce the same desired state for the same inputs, regardless of what
currently exists in the cluster, which aligns with Server-Side Apply's declarative model.

## Understand Participation Modes

`ParticipationModeAuxiliary` means "reconciled but not required for health." It does not mean "skipped." A failing
auxiliary resource still fails the reconciliation. The only difference is that an auxiliary resource's health status
does not affect whether the component condition becomes Ready.

```go
opts, _ := component.NewResourceOptionsBuilder().
    Auxiliary().
    Build()

comp, _ := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(deployment, component.ResourceOptions{}).  // Required for Ready
    WithResource(metricsExporter, opts).                    // Not required for Ready
    Build()
```

Use `Auxiliary` for resources that provide supporting functionality (metrics exporters, debug sidecars, optional
integrations) where their health should not block the component from reporting Ready.

**Exception**: a blocked guard always contributes to the condition regardless of participation mode. A blocked guard
halts the reconciliation pipeline, and that must be visible in the condition.

## Use Feature Gating for Conditional Resources

When an entire resource should only exist based on a feature flag or version constraint, use the resource options
builder with a feature gate rather than conditionally calling `WithResource()` in the controller.

```go
tracingGate := feature.NewVersionGate(owner.Spec.Version, nil).When(owner.Spec.TracingEnabled)

opts, _ := component.NewResourceOptionsBuilder().
    WithFeatureGate(tracingGate).
    Build()

comp, _ := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(deployment, component.ResourceOptions{}).
    WithResource(jaegerSidecar, opts).
    Build()
```

When the gate evaluates to disabled, the framework deletes the resource if it exists. This handles the full lifecycle:
creation when enabled, deletion when disabled. Note that deletion is immediate on the next reconcile, so if you need
graceful decommissioning, handle that before disabling the gate.

## Grace Periods Are Convergence Time

A component in `Creating` or `Updating` for a few minutes during a rolling update is normal, not a failure. Grace
periods give the component time to converge before the framework escalates the condition to `Degraded` or `Down`.

```go
comp, _ := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithResource(deployment, component.ResourceOptions{}).
    WithGracePeriod(5 * time.Minute).
    Build()
```

Set the grace period based on how long the resource legitimately takes to converge. A deployment with a large image pull
or a slow readiness probe needs a longer grace period than a configmap update. A very long grace period delays detection
of genuine failures, so choose a value that reflects expected convergence time, not a safety margin.

## Handle Cluster-Scoped Resources Explicitly

When a namespace-scoped owner manages cluster-scoped resources (like `ClusterRole` or `ClusterRoleBinding`), the
framework cannot set an owner reference because Kubernetes does not allow cross-scope ownership. The framework detects
this and skips the owner reference silently.

This means cluster-scoped resources will not be garbage collected when the owner is deleted. Handle cleanup explicitly
using `Delete: true` in resource options or a finalizer on the owner CRD:

```go
comp, _ := component.NewComponentBuilder().
    WithName("rbac").
    WithConditionType("RBACReady").
    WithResource(clusterRole, component.ResourceOptions{Delete: true}).
    Build()
```

## Name Conditions for the Audience Reading Them

Condition types appear in `kubectl get` output and in monitoring dashboards. Name them for the person or system
consuming that output, not for the internal implementation.

**Prefer**:

- `WebInterfaceReady`
- `DatabaseReady`
- `MigrationComplete`

**Avoid**:

- `DeploymentReconciled`
- `StatefulSetHealthy`
- `JobFinished`

The audience cares about the feature, not the Kubernetes resource type backing it. A condition named
`DeploymentReconciled` tells a user nothing about what capability is affected.
