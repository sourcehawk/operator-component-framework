---
name: using-primitives
description:
  Use when creating or editing Kubernetes resource primitives with the operator-component-framework - primitive builders
  and categories, baseline desired state, the mutation system, boolean and version feature gating (NewBooleanGate,
  NewVersionGate), mutation editors, container selectors, server-side apply behaviour including an Apply rejected with
  "field not declared in schema", workload-kind-agnostic mutations (WorkloadMutator), declared data on primitive
  builders (ExtractInto, WithDataGuard, WithOptionalData), and unstructured primitives.
---

# Using Primitives

## What a primitive is

A primitive wraps a specific Kubernetes kind (`Deployment`, `ConfigMap`, and so on) and encapsulates a desired-state
baseline, a mutation surface, lifecycle integration (readiness, grace handling, suspension), and Server-Side Apply.
Every primitive implements `component.Resource` and may implement one or more lifecycle interfaces to participate in a
component's status aggregation.

The framework groups primitives into four categories by runtime behavior, and the category determines which lifecycle
interfaces a primitive implements:

- **Static**: `ConfigMap`, `Secret`, `ServiceAccount`, RBAC objects, `PodDisruptionBudget`. Desired state is mostly
  fixed; ready as soon as it exists.
- **Workload**: `Deployment`, `StatefulSet`, `DaemonSet`. Long-running processes requiring runtime convergence;
  implement `Alive`, `Graceful`, and `Suspendable`.
- **Task**: `Job`. Short-lived operations that run to completion; implement `Completable` and `Suspendable`.
- **Integration**: `Service`, `Ingress`, `CronJob`, `HPA`. Readiness depends on a controller the operator does not own;
  implement `Operational`, and may also implement `Graceful` or `Suspendable`.

## Baseline plus mutations

This is the framework's central idiom: **the baseline object holds version-independent desired state; every
version-dependent or optional field is applied by a named mutation.** The object you hand a builder (for example
`deployment.NewBuilder(base)`) represents only the shape that never changes across versions or feature toggles. Anything
that depends on the owner's spec version, a feature flag, or a runtime condition belongs in a mutation registered with
`WithMutation`, never hardcoded into the baseline.

```go
base := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{Name: "web-server", Namespace: owner.Namespace},
    Spec: appsv1.DeploymentSpec{
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
        },
    },
}

resource, err := deployment.NewBuilder(base).
    WithMutation(ConfigMutation(owner.Spec.Version)).
    Build()
```

Keeping version-dependent fields out of the baseline is what makes gating, golden testing, and mutation composition
work: a mutation can be introspected, asserted on by name, and enabled or disabled independently of every other mutation
touching the same resource. A baseline that already contains a version-specific value cannot be gated, tested in
isolation, or turned off for an older owner version.

## The mutation system

A mutation is a `feature.Mutation[T]`, where `T` is the primitive's mutator type:

```go
type Mutation[T any] struct {
    Name    string // unique within the resource; used in gating and error reporting
    Feature Gate   // optional; nil means apply unconditionally
    Mutate  func(T) error
}
```

Each primitive package defines its own concrete alias (`deployment.Mutation`, `configmap.Mutation`, and so on) over this
generic type. Register mutations with `WithMutation`, which preserves registration order and is a no-op when called with
no arguments. Mutation names must be unique within a resource: `Build()` fails if two mutations share a `Name`, because
the name is what gating and error reporting refer to.

Mutations do not touch the Kubernetes object directly. Each `Mutate` records intent through typed editors, and the
framework replays every recorded edit in a single controlled pass during `Apply()`. Features apply in registration
order; within one feature's pass, edits run in a fixed category order (object metadata, spec, pod-template metadata, pod
spec, container presence, container edits, init-container presence, init-container edits for pod-workload kinds)
regardless of the order methods were called inside `Mutate`. Later features observe the object as already modified by
earlier ones.

**Boolean gates** (`feature.NewBooleanGate(cond)`) make a mutation conditional on a runtime value, typically a field in
the owner's spec:

```go
gate := feature.NewBooleanGate(len(spec.ExtraEnv) > 0)
```

**Version gates** (`feature.NewVersionGate(currentVersion, constraints)`) enable a mutation only for versions matching
every `feature.VersionConstraint` in the slice. `NewBooleanGate` is shorthand for `NewVersionGate("", nil).When(b)`, and
version and boolean gating combine freely via `.When(...)`, since every condition must be true for the gate to enable:

```go
func MetricsFeatureMutation(version string, enabled bool) configmap.Mutation {
    return configmap.Mutation{
        Name:    "metrics-feature",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *configmap.Mutator) error {
            return m.MergeYAML("app.yaml", "metrics:\n  enabled: true\n  port: 9090\n")
        },
    }
}
```

A common pattern pairs mutually exclusive constraints (`>= V` and `< V`) so exactly one variant of a mutation fires for
any given version.

## Editors and selectors

Editors are scoped, typed APIs for modifying one part of a resource; a mutator hands one to your callback, you record
changes, the framework applies them during the plan-and-apply pass. Groups of editors:

- **Container editors** (`ContainerEditor`), for env vars, args, resources, and probes, selected by a container
  selector.
- **Pod-shaping editors** (`PodSpecEditor`, `ObjectMetaEditor`) shared by all pod-workload kinds.
- **Kind-specific spec editors** (`DeploymentSpecEditor`, `ServiceSpecEditor`, `IngressSpecEditor`, ...), one per kind.
- **Data editors** (`ConfigMapDataEditor`, `SecretDataEditor`) and **RBAC editors** (`PolicyRulesEditor`,
  `BindingSubjectsEditor`).

Every editor exposes `.Raw()`, returning a pointer to the underlying Kubernetes struct for fields the typed API does not
cover; using it is safe because the edit still stays scoped to that editor's target and runs inside the controlled apply
pass. Container selectors, in `pkg/mutation/selectors`, decide which containers an editor targets: `AllContainers()`,
`ContainerNamed(name)`, `ContainersNamed(names...)`, `ContainerNotNamed(name)`, `ContainersNotNamed(names...)`,
`ContainerAtIndex(i)`. A selector is evaluated against a snapshot taken at the start of the container phase, after that
feature's own presence operations, so one mutation can add a container and configure it in the same pass.

For the full method surface of any editor or selector, see `pkg/mutation/editors` and `pkg/mutation/selectors`; each
per-kind reference file also documents the editors relevant to that kind.

## Workload-kind-agnostic mutations

`*deployment.Mutator`, `*statefulset.Mutator`, and `*daemonset.Mutator` share the same container, init-container,
pod-spec, pod-template-metadata, object-metadata, environment-variable, and argument editing methods.
`primitives.WorkloadMutator` is the interface covering exactly that shared surface. Reach for it when the same mutation
(a sidecar, an env var, a label) needs to apply across more than one workload kind: write the emitter once against
`primitives.WorkloadMutator`, then lift it onto each concrete builder with that package's `LiftMutation` adapter, which
carries `Name` and `Feature` through unchanged:

```go
func authEnv() feature.Mutation[primitives.WorkloadMutator] {
    return feature.Mutation[primitives.WorkloadMutator]{
        Name: "auth-env",
        Mutate: func(m primitives.WorkloadMutator) error {
            m.EnsureContainerEnvVar(corev1.EnvVar{Name: "AUTH_MODE", Value: "oidc"})
            return nil
        },
    }
}

backend.WithMutation(statefulset.LiftMutation(authEnv()))
frontend.WithMutation(deployment.LiftMutation(authEnv()))
agent.WithMutation(daemonset.LiftMutation(authEnv()))
```

The interface deliberately omits what is not common to all three kinds: per-kind spec editors (`EditDeploymentSpec`,
`EditStatefulSetSpec`, `EditDaemonSetSpec`), `EnsureReplicas` (no replica field on DaemonSet), and StatefulSet-only
VolumeClaimTemplate methods. Reach for the concrete mutator type for those.

## Declared data on primitive builders

Every primitive builder participates in a component's declared data flow the same way: a package-level
`ExtractInto(builder, cell, fn)` function declares that the resource produces a `concepts.Data[V]` cell (package-level
because a Go method cannot introduce the value type parameter), and the builder methods `WithDataGuard(cells...)` and
`WithOptionalData(cells...)` declare its reads. Component `Build()` validates that every read has a producer registered
earlier. The mechanics, consumption modes, and validation rules live in the `ocf:building-components` skill; verify a
kind's exact `ExtractInto` signature with `go doc` on its package.

## Server-side apply

The framework reconciles with Server-Side Apply: each primitive builds its desired state (baseline plus all active
mutations) and patches it with `client.Apply`, sending only the fields the operator declares. Server-managed defaults
and fields set by other controllers or webhooks are left untouched. The field manager name is derived as
`"{Owner.GetKind()}/{componentName}/{Owner.GetUID()}"`, and the framework applies with forced ownership, taking control
of conflicting fields from other managers while leaving fields it does not include with their current owners. This is
what lets primitives coexist with other controllers touching the same resource without a perpetual-update fight over
stripped server defaults. The owner's UID makes each owner a distinct manager: when the framework sets a controller
reference (the default), two owners of one kind rendering the same object do not silently take each other's fields; the
second owner's apply is rejected because of its second controller reference. For `Unowned()` resources, or where a scope
mismatch prevents the owner reference, the second owner's forced apply still takes the fields it declares;
`managedFields` names each owner, but the framework does not detect the contention unless you register the resource with
`component.BlockOnForeignController()`. That option reports `Blocked` and names the owner whose controller reference is
on the live object, instead of applying. Two owners that both apply without a controller reference stay undetected. When
the readable manager would exceed the API server's 128-character limit, the framework uses the hex-encoded SHA-256 of it
instead (64 characters, deterministic, still distinct per owner), so long kinds or component names show up in
`managedFields` as a hash.

**A Go type that overstates the CRD schema breaks Apply.** The API server's field manager types the patch against the
target's OpenAPI schema before merging anything, so an undeclared field fails the whole apply and the server returns:

```text
failed to create typed patch object: .spec.nodeSets[0].volumeClaimTemplates[0].status:
field not declared in schema
```

The built-in primitives are unaffected, since the API server's schema for a built-in kind matches its Go type. The
failure belongs to wrapped third-party CRDs whose Go type **uses a core struct as a field type**. A CRD declaring
`spec.nodeSets[].volumeClaimTemplates` as `[]corev1.PersistentVolumeClaim` marshals `status: {}` inside every template
while its own schema declares no `status` there. `Update` prunes the field silently, which is why the error only appears
after a move to SSA. The struct tag is not the missing piece: `PersistentVolumeClaim.Status` does carry
`json:"status,omitempty"`, and `omitempty` simply has no effect on a struct value in `encoding/json`.

The framework offers no hook to rewrite an object between mutation and apply. The two honest options are a
`client.Client` decorator installed in `ReconcileContext.Client` that deletes the named undeclared paths from the patch
(here `status` inside each `volumeClaimTemplates` entry) and decodes the server's response back into the typed object,
or managing the kind through the unstructured primitives below, whose content map holds only the fields you put in it.
The `ocf:custom-resource-wrappers` skill carries both in full, including why the response must be decoded back: the
framework keeps that same object as the resource's desired state, and the status handlers read it after the apply.

## Cluster-scoped and unstructured primitives

A primitive for a cluster-scoped kind (`ClusterRole`, `ClusterRoleBinding`, `PersistentVolume`) must call
`MarkClusterScoped()` on its `BaseBuilder`, which inverts the namespace check: the builder rejects a namespace instead
of requiring one, and the primitive's identity function omits the namespace segment.

Unstructured primitives (`pkg/primitives/unstructured/{static,workload,integration,task}`) are the escape hatch for
Kubernetes objects with no Go type, for example external CRDs. One variant exists per category, implementing the
matching lifecycle interfaces; since the framework cannot know the object's semantics, the builders default to
generic-safe behavior (no grace handler means always `Healthy`; no suspension handler means `Suspended` with a no-op
mutation). All variants share a single `Mutator` and an `UnstructuredContentEditor` for nested-field edits.

## Built-in primitives

Each kind below has a per-kind reference file at `references/primitives/<kind>.md` documenting its builder, mutations,
editors, and suspension/lifecycle behavior.

| Primitive                           | Category    | Reference file                                |
| ----------------------------------- | ----------- | --------------------------------------------- |
| `pkg/primitives/deployment`         | Workload    | `references/primitives/deployment.md`         |
| `pkg/primitives/statefulset`        | Workload    | `references/primitives/statefulset.md`        |
| `pkg/primitives/replicaset`         | Workload    | `references/primitives/replicaset.md`         |
| `pkg/primitives/daemonset`          | Workload    | `references/primitives/daemonset.md`          |
| `pkg/primitives/pod`                | Workload    | `references/primitives/pod.md`                |
| `pkg/primitives/job`                | Task        | `references/primitives/job.md`                |
| `pkg/primitives/cronjob`            | Integration | `references/primitives/cronjob.md`            |
| `pkg/primitives/configmap`          | Static      | `references/primitives/configmap.md`          |
| `pkg/primitives/secret`             | Static      | `references/primitives/secret.md`             |
| `pkg/primitives/role`               | Static      | `references/primitives/role.md`               |
| `pkg/primitives/rolebinding`        | Static      | `references/primitives/rolebinding.md`        |
| `pkg/primitives/pdb`                | Static      | `references/primitives/pdb.md`                |
| `pkg/primitives/clusterrole`        | Static      | `references/primitives/clusterrole.md`        |
| `pkg/primitives/clusterrolebinding` | Static      | `references/primitives/clusterrolebinding.md` |
| `pkg/primitives/serviceaccount`     | Static      | `references/primitives/serviceaccount.md`     |
| `pkg/primitives/service`            | Integration | `references/primitives/service.md`            |
| `pkg/primitives/pv`                 | Integration | `references/primitives/pv.md`                 |
| `pkg/primitives/pvc`                | Integration | `references/primitives/pvc.md`                |
| `pkg/primitives/hpa`                | Integration | `references/primitives/hpa.md`                |
| `pkg/primitives/ingress`            | Integration | `references/primitives/ingress.md`            |
| `pkg/primitives/networkpolicy`      | Static      | `references/primitives/networkpolicy.md`      |
| `pkg/primitives/unstructured/*`     | all four    | `references/primitives/unstructured.md`       |

## Anti-patterns

- **Hand-writing a structural interface for a shared mutation instead of using `primitives.WorkloadMutator`.**
  Duplicating the same emitter per kind (or inventing a narrower ad hoc interface) drifts as soon as one copy is edited
  and the others are not; write it once against `WorkloadMutator` and lift it with `LiftMutation`.
- **Unnamed mutations.** An empty or reused `Name` collides at `Build()` and defeats the tooling that asserts which
  mutations fire at which versions; always give a mutation a unique, descriptive name.
- **Putting gated values in the baseline.** A version-dependent or feature-dependent field written directly into the
  baseline object cannot be toggled off, gated by version, or asserted on independently: it silently applies to every
  owner version. Move it into a named, gated mutation instead.

## Ground truth

The consumer's resolved module version is the source of truth, not these docs. Before asserting an exact signature,
method name, or option:

1. Read the framework version from the consumer's `go.mod` entry for
   `github.com/sourcehawk/operator-component-framework`.
2. Verify the symbol with `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

The reference files bundled with this skill match the framework version this plugin shipped with. When they disagree
with `go doc`, `go doc` wins.

## References

- `references/primitives.md`: concepts shared across every primitive, including categories, lifecycle interfaces, the
  mutation system, gating, editors, selectors, Server-Side Apply, and cluster-scoped and unstructured primitives.
- `references/primitives/<kind>.md`: per-kind builders, mutations, editors, and suspension behavior. Read the specific
  kind's file before writing a mutation against it.
