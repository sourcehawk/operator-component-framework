# Guidelines

Recommendations for structuring operators built with the framework. These are not hard rules. They reflect patterns that
are effective and pitfalls that are easy to walk into.

## Table of Contents

- [Represent Desired State in the Baseline Object](#represent-desired-state-in-the-baseline-object)
- [One Component Per Logical Condition](#one-component-per-logical-condition)
- [Keep Controllers Thin](#keep-controllers-thin)
- [Resource Registration Order Is Execution Order](#resource-registration-order-is-execution-order)
- [Mutation Ordering and Container Name Dependencies](#mutation-ordering-and-container-name-dependencies)
- [Use Data Extraction and Guards for Resource Dependencies](#use-data-extraction-and-guards-for-resource-dependencies)
  - [Prefer stable values for guard conditions](#prefer-stable-values-for-guard-conditions)
- [Use Prerequisites for Cross-Component Dependencies](#use-prerequisites-for-cross-component-dependencies)
- [Use Component Feature Gates for Optional Components](#use-component-feature-gates-for-optional-components)
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

### Revert mutations vs. forward mutations

The baseline-as-latest approach means that every structural version change requires a new legacy mutation that reverts
the baseline to the older shape. This is real friction: update the baseline, write a revert mutation, add golden files.
It is natural to wonder whether the opposite direction (baseline stays at the original shape, forward mutations patch it
to the latest version) would be less work.

In practice, the revert direction is easier to maintain:

- **Adding a revert mutation does not require changing existing ones.** Each revert mutation handles one version step:
  the v2 revert turns v3 back into v2, and the v1 revert turns v2 back into v1. They do execute in order (newest first),
  but the v1 revert was written when v2 was the baseline, and it still works because the v2 revert restores the shape it
  expects. When you drop support for v1, you delete one mutation and nothing else changes.
- **Forward mutations have fragile ordering dependencies.** A v3 forward patch might assume that the v2 patch already
  ran (e.g. it expects a container name or port layout that only exists after v2's mutation). Delete the v2 mutation
  when you drop support, and v3 breaks silently because its precondition is gone.
- **You read the baseline more often than you update it.** Structural changes to a resource happen occasionally; reading
  the resource definition happens constantly. With baseline-as-latest, a new contributor opens the file and sees the
  current shape at a glance. With baseline-as-original, understanding the current shape requires mentally replaying
  every forward mutation in order.
- **The two mutation categories have different lifecycles.** Revert mutations are backward compatibility: temporary by
  nature, and they shrink as you drop old versions. Feature mutations (tracing, metrics, debug logging) are
  cross-cutting concerns with a longer lifecycle. Forward mutations mix both categories in the same pipeline, making it
  harder to tell which mutations are temporary compatibility shims and which are permanent features.
- **Where the revert approach costs more.** Each structural version change requires writing a new revert mutation. This
  is the tradeoff. But the friction is also a forcing function: it makes the backward-compatibility decision explicit
  rather than letting old shapes silently persist as the baseline drifts from reality.

The number of revert mutations is bounded by the number of supported versions. Most operators support two or three
concurrent versions. When a version falls out of support, its revert mutation is deleted cleanly. Forward mutation
stacks tend to grow indefinitely because removing a forward mutation requires proving that nothing downstream depends on
it.

### Backward compatibility mutations in practice

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
    WithMutation(BackwardCompatV1Container(owner.Spec.Version)).
    WithMutation(TracingFeature(owner.Spec.TracingEnabled)).
    Build()
```

The backward compat mutation rolls the baseline back for older versions:

```go
func BackwardCompatV1Container(version string) deployment.Mutation {
    return deployment.Mutation{
        Name: "BackwardCompatV1Container",
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

Naming the function `BackwardCompat<version><what>` makes the pattern immediately recognizable. When scanning a builder
chain, `BackwardCompatV1Container` tells you exactly what it does and why it exists without reading the implementation.

`LessThan` here is a user-provided implementation of `feature.VersionConstraint` that wraps a semver comparison. The
interface requires a single `Enabled(version string) (bool, error)` method, so you can use any semver library to
implement your constraints.

For version 2.0 and above, the gate is inactive and the baseline is applied as-is. For older versions, the mutation
adjusts the container name and ports back to the legacy shape. The mutation is explicitly about backward compatibility,
gated on the versions that need it, and will stop running entirely once those versions are no longer supported.

### Verifying backward compatibility mutations

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
golden files, then review the diff. The current version's golden file updates to reflect the new shape, but older
version golden files should stay unchanged. If a baseline change accidentally breaks a backward compat mutation, the
snapshot diff shows exactly what shifted.

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

When two components depend on each other (e.g., the web interface needs the database to be ready before it can be
created), use [prerequisites](#use-prerequisites-for-cross-component-dependencies) to express that dependency
declaratively. Guards and data extraction work within a single component's resource list; prerequisites work between
components.

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

## Mutation Ordering and Container Name Dependencies

Mutations within a resource are also applied in registration order. Each mutation gets its own feature scope, and later
mutations see the resource as modified by all earlier mutations. This is normally invisible because most mutations are
independent. It becomes visible when a backward compat mutation renames a container and a feature mutation needs to
target that container by name.

Consider a deployment where the baseline container is named `"app"` (v2+), and a backward compat mutation renames it to
`"server"` for versions before 2.0. A new mutation that sets `LOG_LEVEL=debug` on the application container faces a
question: does it target `"app"` or `"server"`?

The answer depends on registration order, and there are two rules that eliminate the problem.

### Use broad selectors for version-independent mutations

If a mutation applies to all versions regardless of container name, use `AllContainers()`, `EnsureContainerEnvVar`, or
`EnsureContainerArg`. These selectors never reference a name, so they work whether or not a backward compat rename has
fired. No ordering constraint is needed.

```go
// TracingSidecar uses EnsureContainerEnvVar (wraps AllContainers) and is order-insensitive.
func TracingSidecarMutation(enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "Tracing",
        Feature: feature.NewVersionGate("any", nil).When(enabled),
        Mutate: func(m *deployment.Mutator) error {
            m.EnsureContainer(corev1.Container{
                Name:  "jaeger-agent",
                Image: "jaegertracing/jaeger-agent:1.28",
            })
            m.EnsureContainerEnvVar(corev1.EnvVar{
                Name:  "JAEGER_AGENT_HOST",
                Value: "localhost",
            })
            return nil
        },
    }
}
```

### Register name-specific mutations before backward compat renames

When a mutation must target a specific container by name, register it before the backward compat mutation that renames
it. Registered in that position, the mutation sees the baseline name because the rename has not fired yet. Its edits
carry through the rename because the backward compat mutation only overwrites specific fields (`Name`, `Ports`), not the
entire container.

```go
// DebugLogging targets ContainerNamed("app"), so it must come before BackwardCompatV1Container.
func DebugLoggingMutation(enabled bool) deployment.Mutation {
    return deployment.Mutation{
        Name:    "DebugLogging",
        Feature: feature.NewVersionGate("any", nil).When(enabled),
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

The registration order makes the constraint explicit:

```go
res, err := deployment.NewBuilder(BaseDeployment(owner)).
    WithMutation(DebugLoggingMutation(owner.Spec.EnableDebugLogging)).   // targets "app" by name
    WithMutation(BackwardCompatV1Container(owner.Spec.Version)).        // renames "app" → "server" for v1
    WithMutation(TracingSidecarMutation(owner.Spec.EnableTracing)).     // uses AllContainers, order-insensitive
    Build()
```

For v2+, the backward compat mutation is inactive and `DebugLogging` sets the env var on `"app"`. For v1, `DebugLogging`
sets the env var on `"app"`, then `BackwardCompatV1Container` renames the container to `"server"` and resets its ports.
The env var survives because the rename does not touch `Env`.

### Ordering with multiple backward compat mutations

When multiple backward compat mutations exist, the same chained revert ordering from
[Revert mutations vs. forward mutations](#revert-mutations-vs-forward-mutations) applies: register the newest first
(closest to the baseline) and the oldest last. The additional constraint here is that feature mutations targeting a
container by name must come before the backward compat mutations that rename it. Feature mutations using broad selectors
can go anywhere.

```go
res, err := deployment.NewBuilder(BaseDeployment(owner)).                     // baseline is v3
    WithMutation(DebugLoggingMutation(owner.Spec.EnableDebugLogging)).        // must come before backward compat renames
    WithMutation(BackwardCompatV2Container(owner.Spec.Version)).              // reverts v3 → v2 for < 3.0.0
    WithMutation(BackwardCompatV1Container(owner.Spec.Version)).              // reverts v2 → v1 for < 2.0.0
    WithMutation(TracingSidecarMutation(owner.Spec.EnableTracing)).           // order-insensitive (broad selector)
    Build()
```

When you add a new version that changes the resource structure, update the baseline and insert the new backward compat
mutation before the existing ones.

### What to avoid

Do not work around the ordering problem by matching multiple names:

```go
// Anti-pattern: couples this mutation to knowledge of legacy naming.
m.EditContainers(selectors.ContainersNamed("app", "server"), func(ce *editors.ContainerEditor) error {
    ce.EnsureEnvVar(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
    return nil
})
```

This works today but breaks if a future version renames the container again. The mutation now needs to track every name
the container has ever had. Instead, target the baseline name and
[register the mutation before the backward compat rename](#register-name-specific-mutations-before-backward-compat-renames).

### When a backward compat mutation replaces the entire container

The carry-through property depends on the backward compat mutation only overwriting specific fields. If a backward
compat mutation replaces the entire container (sets all fields, not just `Name` and `Ports`), edits from earlier
mutations are lost. In that case, the mutation is effectively a full override and later mutations should target the
post-rename name via version gating rather than relying on ordering.

See the [mutations-and-gating example](../examples/mutations-and-gating/) for a working demonstration of these patterns.

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

### Prefer stable values for guard conditions

A guard re-evaluates on every reconcile. If the extracted value it depends on is unstable (it can disappear, change, or
transiently become empty), the guard will re-block after the dependent resource has already been created. In most cases
this is not intentional. The resource is already running, but the guard now reports `Blocked` and skips reconciliation
for everything after it.

Good candidates for guard conditions are values that appear once and remain stable: a status field written by a
controller (an ARN, a provisioned IP, a generated credential reference). Poor candidates are values that fluctuate
during normal operation, such as replica counts, transient annotations, or fields that get cleared during rolling
updates.

If you genuinely need to react to a value disappearing after initial creation, that is a valid use case, but it should
be a deliberate design choice rather than an accidental side effect of choosing an unstable extraction target.

## Use Prerequisites for Cross-Component Dependencies

When one component cannot start until another is ready, use a prerequisite on the dependent component rather than
orchestrating the ordering in the controller.

```go
dbComp, err := component.NewComponentBuilder().
    WithName("database").
    WithConditionType("DatabaseReady").
    WithResource(statefulSet, component.ResourceOptions{}).
    Build()

webComp, err := component.NewComponentBuilder().
    WithName("web-interface").
    WithConditionType("WebInterfaceReady").
    WithPrerequisite(component.DependsOn("DatabaseReady")).
    WithResource(deployment, component.ResourceOptions{}).
    Build()
```

The web-interface component will not reconcile any resources until the `DatabaseReady` condition on the owner is `True`.
Once the component passes through to normal reconciliation for the first time, the prerequisite is permanently passed
and never re-evaluated.

This is the right tool when a component needs something to exist before it can be created. It is not the right tool for
ongoing health dependencies. If the database goes down after the web interface is already running, the web interface
component continues reconciling its own resources. The database's condition reflects the problem, and the web
interface's condition reflects its own health independently. Conflating the two would lose the granularity that separate
components provide.

**Guards vs. prerequisites:** Guards are for resource dependencies within a single component (resource B depends on data
from resource A). Prerequisites are for startup dependencies between components. Guards re-evaluate every reconcile;
prerequisites evaluate only until the component's first successful reconciliation.

## Use Component Feature Gates for Optional Components

When an entire component should only exist based on a feature flag, use a component-level feature gate rather than
conditionally building the component in the controller.

```go
comp, err := component.NewComponentBuilder().
    WithName("monitoring").
    WithConditionType("MonitoringReady").
    WithFeatureGate(feature.NewVersionGate(owner.Spec.Version, nil).When(owner.Spec.MonitoringEnabled)).
    WithResource(exporterDeployment, component.ResourceOptions{}).
    WithResource(exporterService, component.ResourceOptions{}).
    Build()
```

When the gate is disabled, the framework deletes all of the component's resources and reports `True/Disabled`. When
re-enabled, the component reconciles normally. This is different from resource-level feature gating, which controls
individual resources within a component. Use a component gate when the entire component is conditional; use resource
gates when only some resources within the component are conditional.

A disabled component gate takes precedence over suspension. If both the gate and suspension are active, the component is
treated as disabled (resources deleted), not suspended (resources scaled down).

## Mutations Describe Intent, Not Observation

Mutations operate on the desired object, not the server's current state. A mutation should be a pure function of the
owner spec and other static inputs available at build time. It should never try to read the resource's live cluster
state to decide what to write.

This is not just a style preference. Within a single resource, the framework runs mutations **before** data extraction.
A data extractor registered on the same builder as a mutation will not have executed yet when that mutation runs. Any
closure variable populated by the extractor will still hold its zero value.

Data extractors exist to pass observed state from an **earlier** resource to a **later** resource's guards and
mutations. They are not a mechanism for feeding a resource's own live state back into its own mutations. If you find
yourself wanting to do that, reconsider the design: the mutation is likely encoding observation rather than intent.

A well-written mutation produces the same desired state for the same owner spec, regardless of what currently exists in
the cluster. This aligns with Server-Side Apply's declarative model and keeps the reconciliation loop predictable.

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
this, skips setting the owner reference, and emits an Info log noting the skipped reference and its garbage collection
implications.

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

## Further Reading

For a deeper look at the structural problems these guidelines address, see
[The Missing Layers in Your Kubernetes Operator](https://medium.com/@sourcehawk/the-missing-layers-in-your-kubernetes-operator-306ee8633350).
