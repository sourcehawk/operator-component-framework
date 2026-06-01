# Workload-kind-agnostic mutator interface

Issue: https://github.com/sourcehawk/operator-component-framework/issues/142

## Problem

`*statefulset.Mutator`, `*deployment.Mutator`, and `*daemonset.Mutator` expose an
almost identical env/container/podspec/metadata editing surface, but they are
unrelated concrete types. The only framework interface spanning them,
`generic.FeatureMutator`, covers just `Apply()` and `NextFeature()`, not the
editing methods.

A consumer that wants one workload-kind-agnostic mutation (for example, a shared
"emit these auth/license/storage env vars on the app container" helper rendered as
a StatefulSet by one component and a Deployment by others) cannot express it
against a framework type. The helper must either be duplicated per workload kind or
routed through a consumer-defined structural interface that mirrors the framework's
method set by hand and silently drifts when the framework changes.

## Solution

Export a framework interface, `primitives.WorkloadMutator`, carrying the shared
editing surface, and a small per-kind adapter that lifts a
`feature.Mutation[primitives.WorkloadMutator]` into each kind's `Mutation` type.

### 1. New package `pkg/primitives` (`package primitives`)

A new top-level package holding the interface. It imports only
`mutation/editors`, `mutation/selectors`, and `corev1`. It never imports its own
subpackages, so the existing `statefulset -> primitives`,
`deployment -> primitives`, and `daemonset -> primitives` import direction stays
acyclic.

```go
// WorkloadMutator is the editing surface shared by every pod-workload mutator
// (*statefulset.Mutator, *deployment.Mutator, *daemonset.Mutator). It lets a
// consumer write one workload-kind-agnostic mutation and apply it to any of them.
type WorkloadMutator interface {
    EditContainers(selectors.ContainerSelector, func(*editors.ContainerEditor) error)
    EditInitContainers(selectors.ContainerSelector, func(*editors.ContainerEditor) error)
    EnsureContainer(corev1.Container)
    RemoveContainer(string)
    RemoveContainers([]string)
    EnsureInitContainer(corev1.Container)
    RemoveInitContainer(string)
    RemoveInitContainers([]string)
    EditPodSpec(func(*editors.PodSpecEditor) error)
    EditPodTemplateMetadata(func(*editors.ObjectMetaEditor) error)
    EditObjectMetadata(func(*editors.ObjectMetaEditor) error)
    EnsureContainerEnvVar(corev1.EnvVar)
    RemoveContainerEnvVar(string)
    RemoveContainerEnvVars([]string)
    EnsureContainerArg(string)
    RemoveContainerArg(string)
    RemoveContainerArgs([]string)
}
```

Deliberately excluded:

- `Apply()` / `NextFeature()`: framework lifecycle, not an emitter's concern. The
  interface stays a pure editing contract.
- `EditStatefulSetSpec` / `EditDeploymentSpec` / `EditDaemonSetSpec`: kind-specific
  editor return types.
- `EnsureReplicas`: absent on the daemonset mutator (DaemonSets have no replicas).
- `EnsureVolumeClaimTemplate` / `RemoveVolumeClaimTemplate`: StatefulSet only.

The result is exactly the intersection across all three existing pod-workload
kinds. A future replica-less kind (for example, a Job-backed workload) can join
without changing the contract.

### 2. Compile-time conformance guards

In each of the three primitive packages:

```go
var _ primitives.WorkloadMutator = (*Mutator)(nil)
```

These live in the child packages, not the parent (the parent importing children
would cycle). They are the key advantage over a consumer-maintained mirror: a
future rename or removal of a shared method breaks the build here, inside the
framework, instead of drifting silently in a downstream operator.

### 3. Per-kind `LiftMutation` adapters

In each of `statefulset`, `deployment`, `daemonset`:

```go
// LiftMutation adapts a workload-kind-agnostic mutation into a <Kind> Mutation
// so it can be registered with WithMutation. Name and Feature gating carry over
// unchanged. A nil Mutate is preserved so ApplyIntent still reports it by name.
func LiftMutation(m feature.Mutation[primitives.WorkloadMutator]) Mutation {
    lifted := Mutation{Name: m.Name, Feature: m.Feature}
    if m.Mutate != nil {
        lifted.Mutate = func(mut *Mutator) error { return m.Mutate(mut) }
    }
    return lifted
}
```

`Mutation` is the package's defined type `type Mutation feature.Mutation[*Mutator]`,
which is exactly what each builder's `WithMutation` accepts.

Call site for the issue's scenario:

```go
func emitAuthEnv() feature.Mutation[primitives.WorkloadMutator] { /* one emitter */ }

zeebeSts.WithMutation(statefulset.LiftMutation(emitAuthEnv()))
gatewayDeploy.WithMutation(deployment.LiftMutation(emitAuthEnv()))
```

## Testing

### Per-kind `LiftMutation` tests (statefulset, deployment, daemonset)

- Name and Feature carry over unchanged.
- The lifted `Mutate` invokes the original with the concrete `*Mutator`, asserted
  through an edit that lands on the object after `Apply()`.
- Gating is respected: a disabled `feature.Gate` makes the lifted mutation a no-op
  through `ApplyIntent`.
- A nil `Mutate` is preserved (the lifted `Mutate` stays nil, so `ApplyIntent`
  reports `mutation handler of <name> is nil` rather than panicking).

### Cross-kind behavioral test

A single `func() feature.Mutation[primitives.WorkloadMutator]` emitter lifted into
both a StatefulSet and a Deployment, applied, asserting the same env var lands on
the app container of both. This guards the feature's actual intent: one emitter,
two workload kinds.

### Conformance

The compile-time `var _` guards are the conformance check and need no runtime test.

## Documentation and housekeeping

Updated in the same change as the code:

- `docs/primitives.md`: a subsection documenting `primitives.WorkloadMutator` and
  the `LiftMutation` pattern, with the shared-emitter example. Run `make fmt-md`.
- `CLAUDE.md`: add the new `pkg/primitives` top-level package to the "Source to
  read" list.
- `examples/`: grep for a natural spot. If an existing example renders both a
  Deployment and a StatefulSet, add a short shared-emitter usage. Otherwise the
  `docs/primitives.md` example suffices and no new example directory is added.
  Confirm `make build-examples` stays green.

No E2E tests: this is a compile-time and type-plumbing change with no new
reconciliation behavior. Unit and cross-kind behavioral tests cover the intent.

## Verification

- `make all` (fmt, lint, test) green.
- `make build-examples` green.
