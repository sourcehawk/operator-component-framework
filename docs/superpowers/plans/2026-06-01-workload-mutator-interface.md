# Workload-kind-agnostic Mutator Interface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Export a framework interface `primitives.WorkloadMutator` and per-kind `LiftMutation` adapters so consumers
can write one workload-kind-agnostic mutation and apply it across StatefulSet, Deployment, and DaemonSet.

**Architecture:** A new top-level `pkg/primitives` package holds the shared editing-surface interface (the intersection
of the three pod-workload mutators). Each primitive package adds a compile-time conformance guard and a `LiftMutation`
adapter that wraps a `feature.Mutation[primitives.WorkloadMutator]` into its own defined `Mutation` type. No
reconciliation behavior changes; this is type plumbing plus drift protection.

**Tech Stack:** Go generics, Ginkgo is used elsewhere but these packages use plain `testing` + testify
(`assert`/`require`), matching the existing `pkg/primitives/*/mutator_test.go` style.

---

### Task 1: Define `primitives.WorkloadMutator` and conformance guards

**Files:**

- Create: `pkg/primitives/workload_mutator.go`
- Modify: `pkg/primitives/statefulset/mutator.go` (add guard after the `Mutation` type, line 13 area)
- Modify: `pkg/primitives/deployment/mutator.go` (add guard after the `Mutation` type, line 13 area)
- Modify: `pkg/primitives/daemonset/mutator.go` (add guard after the `Mutation` type, line 13 area)

- [ ] **Step 1: Create the interface**

`pkg/primitives/workload_mutator.go`:

```go
// Package primitives hosts cross-kind contracts shared by the concrete primitive
// packages under pkg/primitives. It depends only on the mutation editor and
// selector packages, never on its own subpackages, so the subpackages can import
// it without creating an import cycle.
package primitives

import (
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	corev1 "k8s.io/api/core/v1"
)

// WorkloadMutator is the editing surface shared by every pod-workload mutator:
// *statefulset.Mutator, *deployment.Mutator, and *daemonset.Mutator. It lets a
// consumer express one workload-kind-agnostic mutation (for example, emitting a
// shared set of environment variables on the application container) and apply it
// to any of those kinds through the per-package LiftMutation adapters.
//
// It is exactly the intersection of the three concrete mutators' editing methods.
// Kind-specific operations are intentionally excluded and remain on the concrete
// types: the spec editors (EditStatefulSetSpec, EditDeploymentSpec,
// EditDaemonSetSpec), EnsureReplicas (DaemonSets have no replicas), and the
// StatefulSet-only VolumeClaimTemplate methods. The lifecycle methods Apply and
// NextFeature are also excluded; they are driven by the framework, not by an
// emitter.
type WorkloadMutator interface {
	EditContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error)
	EditInitContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error)
	EnsureContainer(container corev1.Container)
	RemoveContainer(name string)
	RemoveContainers(names []string)
	EnsureInitContainer(container corev1.Container)
	RemoveInitContainer(name string)
	RemoveInitContainers(names []string)
	EditPodSpec(edit func(*editors.PodSpecEditor) error)
	EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error)
	EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error)
	EnsureContainerEnvVar(ev corev1.EnvVar)
	RemoveContainerEnvVar(name string)
	RemoveContainerEnvVars(names []string)
	EnsureContainerArg(arg string)
	RemoveContainerArg(arg string)
	RemoveContainerArgs(args []string)
}
```

- [ ] **Step 2: Verify the new package builds**

Run: `go build ./pkg/primitives/` Expected: success, no output.

- [ ] **Step 3: Add the conformance guard to each concrete mutator**

In `pkg/primitives/statefulset/mutator.go`, add the `primitives` import to the import block and insert immediately after
the `type Mutation feature.Mutation[*Mutator]` line:

```go
// Compile-time guarantee that *Mutator satisfies the shared workload editing
// surface. If a future change renames or removes a shared method, this breaks
// the build here instead of drifting silently in downstream consumers.
var _ primitives.WorkloadMutator = (*Mutator)(nil)
```

The import to add (alongside the existing `feature`, `editors`, `selectors` imports):

```go
"github.com/sourcehawk/operator-component-framework/pkg/primitives"
```

Repeat the identical guard line and import in `pkg/primitives/deployment/mutator.go` and
`pkg/primitives/daemonset/mutator.go`.

- [ ] **Step 4: Verify all three satisfy the interface**

Run: `go build ./...` Expected: success. This compiles the three `var _ primitives.WorkloadMutator = (*Mutator)(nil)`
guards, proving conformance. (If you temporarily rename a shared method on one mutator, this build fails with a "does
not implement" error, which is the drift protection working.)

- [ ] **Step 5: Commit**

```bash
git add pkg/primitives/workload_mutator.go pkg/primitives/statefulset/mutator.go pkg/primitives/deployment/mutator.go pkg/primitives/daemonset/mutator.go
git commit -m "feat(primitives): shared WorkloadMutator interface with conformance guards"
```

---

### Task 2: Add `statefulset.LiftMutation` with tests

**Files:**

- Modify: `pkg/primitives/statefulset/mutator.go` (append the function)
- Test: `pkg/primitives/statefulset/mutator_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/primitives/statefulset/mutator_test.go`. Add the `feature` and `primitives` imports to the test file's
import block if not present:

```go
type stubGate bool

func (g stubGate) Enabled() (bool, error) { return bool(g), nil }

func newSingleContainerSTS() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
}

func TestLiftMutation_CarriesAndInvokes(t *testing.T) {
	called := false
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name: "emit-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			called = true
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, "emit-env", lifted.Name)
	assert.Nil(t, lifted.Feature)
	require.NotNil(t, lifted.Mutate)

	sts := newSingleContainerSTS()
	m := NewMutator(sts)
	require.NoError(t, lifted.Mutate(m))
	require.NoError(t, m.Apply())

	assert.True(t, called)
	require.Len(t, sts.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", sts.Spec.Template.Spec.Containers[0].Env[0].Name)
}

func TestLiftMutation_GateDisabledIsNoOp(t *testing.T) {
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name:    "gated",
		Feature: stubGate(false),
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, stubGate(false), lifted.Feature)

	sts := newSingleContainerSTS()
	m := NewMutator(sts)
	require.NoError(t, feature.Mutation[*Mutator](lifted).ApplyIntent(m))
	require.NoError(t, m.Apply())

	assert.Empty(t, sts.Spec.Template.Spec.Containers[0].Env)
}

func TestLiftMutation_NilMutatePreserved(t *testing.T) {
	lifted := LiftMutation(feature.Mutation[primitives.WorkloadMutator]{Name: "nilmut"})
	assert.Nil(t, lifted.Mutate)

	err := feature.Mutation[*Mutator](lifted).ApplyIntent(NewMutator(newSingleContainerSTS()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutation handler of nilmut is nil")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/primitives/statefulset/ -run TestLiftMutation -v` Expected: FAIL with "undefined: LiftMutation".

- [ ] **Step 3: Implement `LiftMutation`**

Append to `pkg/primitives/statefulset/mutator.go` (the `primitives` import was already added in Task 1):

```go
// LiftMutation adapts a workload-kind-agnostic mutation into a StatefulSet
// Mutation so it can be registered with the builder's WithMutation. Name and
// Feature gating carry over unchanged. A nil Mutate is preserved, so ApplyIntent
// still reports it by name rather than panicking.
func LiftMutation(m feature.Mutation[primitives.WorkloadMutator]) Mutation {
	lifted := Mutation{Name: m.Name, Feature: m.Feature}
	if m.Mutate != nil {
		lifted.Mutate = func(mut *Mutator) error { return m.Mutate(mut) }
	}
	return lifted
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/primitives/statefulset/ -run TestLiftMutation -v` Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/primitives/statefulset/mutator.go pkg/primitives/statefulset/mutator_test.go
git commit -m "feat(statefulset): LiftMutation adapter for WorkloadMutator"
```

---

### Task 3: Add `deployment.LiftMutation` with tests

**Files:**

- Modify: `pkg/primitives/deployment/mutator.go` (append the function)
- Test: `pkg/primitives/deployment/mutator_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/primitives/deployment/mutator_test.go`. Add the `feature` and `primitives` imports to the test file's
import block if not present:

```go
type stubGate bool

func (g stubGate) Enabled() (bool, error) { return bool(g), nil }

func newSingleContainerDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
}

func TestLiftMutation_CarriesAndInvokes(t *testing.T) {
	called := false
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name: "emit-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			called = true
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, "emit-env", lifted.Name)
	assert.Nil(t, lifted.Feature)
	require.NotNil(t, lifted.Mutate)

	dep := newSingleContainerDeployment()
	m := NewMutator(dep)
	require.NoError(t, lifted.Mutate(m))
	require.NoError(t, m.Apply())

	assert.True(t, called)
	require.Len(t, dep.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", dep.Spec.Template.Spec.Containers[0].Env[0].Name)
}

func TestLiftMutation_GateDisabledIsNoOp(t *testing.T) {
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name:    "gated",
		Feature: stubGate(false),
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, stubGate(false), lifted.Feature)

	dep := newSingleContainerDeployment()
	m := NewMutator(dep)
	require.NoError(t, feature.Mutation[*Mutator](lifted).ApplyIntent(m))
	require.NoError(t, m.Apply())

	assert.Empty(t, dep.Spec.Template.Spec.Containers[0].Env)
}

func TestLiftMutation_NilMutatePreserved(t *testing.T) {
	lifted := LiftMutation(feature.Mutation[primitives.WorkloadMutator]{Name: "nilmut"})
	assert.Nil(t, lifted.Mutate)

	err := feature.Mutation[*Mutator](lifted).ApplyIntent(NewMutator(newSingleContainerDeployment()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutation handler of nilmut is nil")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/primitives/deployment/ -run TestLiftMutation -v` Expected: FAIL with "undefined: LiftMutation".

- [ ] **Step 3: Implement `LiftMutation`**

Append to `pkg/primitives/deployment/mutator.go` (the `primitives` import was already added in Task 1):

```go
// LiftMutation adapts a workload-kind-agnostic mutation into a Deployment
// Mutation so it can be registered with the builder's WithMutation. Name and
// Feature gating carry over unchanged. A nil Mutate is preserved, so ApplyIntent
// still reports it by name rather than panicking.
func LiftMutation(m feature.Mutation[primitives.WorkloadMutator]) Mutation {
	lifted := Mutation{Name: m.Name, Feature: m.Feature}
	if m.Mutate != nil {
		lifted.Mutate = func(mut *Mutator) error { return m.Mutate(mut) }
	}
	return lifted
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/primitives/deployment/ -run TestLiftMutation -v` Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/primitives/deployment/mutator.go pkg/primitives/deployment/mutator_test.go
git commit -m "feat(deployment): LiftMutation adapter for WorkloadMutator"
```

---

### Task 4: Add `daemonset.LiftMutation` with tests

**Files:**

- Modify: `pkg/primitives/daemonset/mutator.go` (append the function)
- Test: `pkg/primitives/daemonset/mutator_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/primitives/daemonset/mutator_test.go`. Add the `feature` and `primitives` imports to the test file's
import block if not present:

```go
type stubGate bool

func (g stubGate) Enabled() (bool, error) { return bool(g), nil }

func newSingleContainerDaemonSet() *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
}

func TestLiftMutation_CarriesAndInvokes(t *testing.T) {
	called := false
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name: "emit-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			called = true
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, "emit-env", lifted.Name)
	assert.Nil(t, lifted.Feature)
	require.NotNil(t, lifted.Mutate)

	ds := newSingleContainerDaemonSet()
	m := NewMutator(ds)
	require.NoError(t, lifted.Mutate(m))
	require.NoError(t, m.Apply())

	assert.True(t, called)
	require.Len(t, ds.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", ds.Spec.Template.Spec.Containers[0].Env[0].Name)
}

func TestLiftMutation_GateDisabledIsNoOp(t *testing.T) {
	agnostic := feature.Mutation[primitives.WorkloadMutator]{
		Name:    "gated",
		Feature: stubGate(false),
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}

	lifted := LiftMutation(agnostic)
	assert.Equal(t, stubGate(false), lifted.Feature)

	ds := newSingleContainerDaemonSet()
	m := NewMutator(ds)
	require.NoError(t, feature.Mutation[*Mutator](lifted).ApplyIntent(m))
	require.NoError(t, m.Apply())

	assert.Empty(t, ds.Spec.Template.Spec.Containers[0].Env)
}

func TestLiftMutation_NilMutatePreserved(t *testing.T) {
	lifted := LiftMutation(feature.Mutation[primitives.WorkloadMutator]{Name: "nilmut"})
	assert.Nil(t, lifted.Mutate)

	err := feature.Mutation[*Mutator](lifted).ApplyIntent(NewMutator(newSingleContainerDaemonSet()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutation handler of nilmut is nil")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/primitives/daemonset/ -run TestLiftMutation -v` Expected: FAIL with "undefined: LiftMutation".

- [ ] **Step 3: Implement `LiftMutation`**

Append to `pkg/primitives/daemonset/mutator.go` (the `primitives` import was already added in Task 1):

```go
// LiftMutation adapts a workload-kind-agnostic mutation into a DaemonSet
// Mutation so it can be registered with the builder's WithMutation. Name and
// Feature gating carry over unchanged. A nil Mutate is preserved, so ApplyIntent
// still reports it by name rather than panicking.
func LiftMutation(m feature.Mutation[primitives.WorkloadMutator]) Mutation {
	lifted := Mutation{Name: m.Name, Feature: m.Feature}
	if m.Mutate != nil {
		lifted.Mutate = func(mut *Mutator) error { return m.Mutate(mut) }
	}
	return lifted
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/primitives/daemonset/ -run TestLiftMutation -v` Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/primitives/daemonset/mutator.go pkg/primitives/daemonset/mutator_test.go
git commit -m "feat(daemonset): LiftMutation adapter for WorkloadMutator"
```

---

### Task 5: Cross-kind behavioral test (the issue's actual scenario)

**Files:**

- Test: `pkg/primitives/workload_mutator_test.go` (create, external test package)

This test imports both `statefulset` and `deployment`, which import `primitives`. Putting it in the external
`primitives_test` package avoids any import cycle and proves one emitter drives two workload kinds.

- [ ] **Step 1: Write the test**

`pkg/primitives/workload_mutator_test.go`:

```go
package primitives_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// emitShared is a single workload-kind-agnostic emitter used for both kinds.
func emitShared() feature.Mutation[primitives.WorkloadMutator] {
	return feature.Mutation[primitives.WorkloadMutator]{
		Name: "shared-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "SHARED", Value: "x"})
			return nil
		},
	}
}

func TestWorkloadMutator_OneEmitterTwoKinds(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			},
		},
	}
	sm := statefulset.NewMutator(sts)
	require.NoError(t, statefulset.LiftMutation(emitShared()).Mutate(sm))
	require.NoError(t, sm.Apply())
	require.Len(t, sts.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", sts.Spec.Template.Spec.Containers[0].Env[0].Name)

	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			},
		},
	}
	dm := deployment.NewMutator(dep)
	require.NoError(t, deployment.LiftMutation(emitShared()).Mutate(dm))
	require.NoError(t, dm.Apply())
	require.Len(t, dep.Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, "SHARED", dep.Spec.Template.Spec.Containers[0].Env[0].Name)
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./pkg/primitives/ -run TestWorkloadMutator_OneEmitterTwoKinds -v` Expected: PASS.

- [ ] **Step 3: Run the full primitives suite to confirm no regressions**

Run: `go test ./pkg/primitives/...` Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/primitives/workload_mutator_test.go
git commit -m "test(primitives): one WorkloadMutator emitter across two workload kinds"
```

---

### Task 6: Documentation and final verification

**Files:**

- Modify: `docs/primitives.md` (add a subsection on the shared workload editing surface)
- Modify: `CLAUDE.md` (add the new package to the "Source to read" list)
- Check: `examples/` (grep for a dual-kind example)

- [ ] **Step 1: Add a docs subsection**

Open `docs/primitives.md`, find the mutation system section (search for "Mutation" / the workload mutator discussion).
Add this subsection at the end of that section:

````markdown
### Workload-kind-agnostic mutations

`*statefulset.Mutator`, `*deployment.Mutator`, and `*daemonset.Mutator` share the same container, init-container,
pod-spec, pod-template-metadata, object-metadata, environment-variable, and argument editing methods.
`primitives.WorkloadMutator` is the framework interface covering exactly that shared surface, so a single mutation can
target any pod-workload kind.

Write the emitter once against the interface, then lift it into each kind's `Mutation` with that package's
`LiftMutation` adapter before registering it:

```go
func emitAuthEnv() feature.Mutation[primitives.WorkloadMutator] {
	return feature.Mutation[primitives.WorkloadMutator]{
		Name: "auth-env",
		Mutate: func(m primitives.WorkloadMutator) error {
			m.EnsureContainerEnvVar(corev1.EnvVar{Name: "AUTH_MODE", Value: "oidc"})
			return nil
		},
	}
}

zeebeSts.WithMutation(statefulset.LiftMutation(emitAuthEnv()))
gatewayDeploy.WithMutation(deployment.LiftMutation(emitAuthEnv()))
```

`LiftMutation` carries the mutation's `Name` and feature `Gate` through unchanged. The interface deliberately omits
kind-specific operations (the spec editors, `EnsureReplicas`, and the StatefulSet-only VolumeClaimTemplate methods);
reach for the concrete mutator type when you need those.
````

- [ ] **Step 2: Format the markdown**

Run: `make fmt-md` Expected: success; `docs/primitives.md` reformatted consistently.

- [ ] **Step 3: Add the package to CLAUDE.md**

In `CLAUDE.md`, under "### Source to read", add this entry after the `pkg/primitives/` line:

```markdown
- `pkg/primitives/` (top-level package) — `WorkloadMutator`, the editing surface shared by the pod-workload mutators,
  plus the per-kind `LiftMutation` adapters
```

- [ ] **Step 4: Check examples for a dual-kind usage**

Run: `grep -rln "deployment.NewBuilder\|statefulset.NewBuilder" examples/` Expected: a list of example files. If any
single example builds both a Deployment and a StatefulSet from a shared concern, add a short `LiftMutation` usage there
mirroring the docs snippet. If none does, no example change is needed; the docs snippet is sufficient. Either way, do
not invent a new example directory.

- [ ] **Step 5: Build examples**

Run: `make build-examples` Expected: success (confirms any example edit compiles; a no-op if none was made).

- [ ] **Step 6: Full verification**

Run: `make all` Expected: fmt, lint, and `go test ./...` all green.

- [ ] **Step 7: Commit**

```bash
git add docs/primitives.md CLAUDE.md examples/
git commit -m "docs(primitives): document WorkloadMutator and LiftMutation"
```

---

## Notes for the implementer

- The three `LiftMutation` bodies are identical except for the package's own `Mutator`/`Mutation` types. This is not
  duplication to factor out: each returns its own package's defined `Mutation` type, which is exactly what that
  package's `WithMutation` accepts. A single generic helper would force consumers to convert to the defined type at the
  call site, which is the boilerplate this feature removes.
- `Mutation` in each primitive package is a defined type (`type Mutation feature.Mutation[*Mutator]`), not an alias.
  That is why the gating tests convert with `feature.Mutation[*Mutator](lifted)` before calling `ApplyIntent` (the
  method lives on `feature.Mutation`, and defined types do not inherit it).
- `stubGate` is declared once per test file. If a primitive test file already declares a gate stub, reuse it instead of
  redeclaring (a duplicate declaration in the same package will not compile).
- The conformance guards are the reason the `primitives` import lands in each `mutator.go` in Task 1; Tasks 2 to 4 reuse
  that import for `LiftMutation` and add no new import there.

```

```
