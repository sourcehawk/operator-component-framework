# Version-matrix golden generation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional, test-only helper that sweeps a consumer-supplied version universe, classifies versions into
behaviorally-distinct gating regimes by firing-set, generates the minimal goldens covering them, asserts per-fixture
mutation gating, and proves every registered mutation is accounted for.

**Architecture:** A read-only `MutationInspector` interface on the framework (`generic.BaseResource` +
`*component.Component`) surfaces registered mutation names and the firing-set at a built version. An exported
`golden.Serialize*` reuses the existing serializer. A new test-only package `pkg/testing/goldengen` wraps a built unit
through adapters, sweeps `(fixture, version)`, groups versions by firing-set into regimes, writes one golden per regime
plus a manifest, asserts gating, and offers a user-invoked accounting assertion. An optional `LoadMatrix` reads the
whole declaration from YAML.

**Tech Stack:** Go (generics), testify (`require`/`assert`), `sigs.k8s.io/yaml`, `k8s.io/apimachinery` runtime scheme,
controller-runtime `client.Object`.

---

## PR breakdown and dependency graph

The work lands on `feature/version-matrix-goldens` as four sub-PRs, each targeting the feature branch:

- **PR1 — #132** (label `task`): framework introspection + serializer export. No dependencies.
- **PR2 — #133** (label `feature`): goldengen core. Depends on PR1.
- **PR3 — #134** (label `feature`): accounting + example + docs. Depends on PR2.
- **PR4 — #135** (label `feature`): YAML matrix loader. Depends on PR2.

```
#132 ──> #133 ──┬──> #134
                └──> #135
```

PR3 and PR4 are independent of each other and may run in parallel once PR2 has merged into the feature branch.

## Contracts

| Name                   | Producer | Consumer   | Shape                                                                                                                               | Realization                                      |
| ---------------------- | -------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| `MutationInspector`    | #132     | #133       | `interface { RegisteredMutations() []string; FiringSet() ([]string, error) }` in `pkg/component/concepts`                           | Merges to feature branch before #133 branches    |
| `golden.Serialize*`    | #132     | #133       | `Serialize(client.Object, *runtime.Scheme) ([]byte, error)`, `SerializeComponent([]client.Object, *runtime.Scheme) ([]byte, error)` | Merges to feature branch before #133 branches    |
| `goldengen` public API | #133     | #134, #135 | `Config[T]`, `Fixture[T]`, `Expect`, `Unit`, `Generator[T]`, `New`, `Run`, `Resource`, `Component`                                  | Merges to feature branch before #134/#135 branch |

All three are sequential merge dependencies (no pre-merge stub PRs needed): each consumer branches from the
post-producer state of the feature branch. PR3 and PR4 share only PR2's public API; they expose nothing to each other,
so there is no contract between them.

## Conventions

- **Package & files.** New package `pkg/testing/goldengen`. One responsibility per file: `unit.go` (the `Unit`
  interface + `Resource`/`Component` adapters), `config.go` (`Config`/`Fixture`/`Expect` types + validation),
  `classify.go` (firing-set grouping + representative selection), `generator.go` (`New`/`Run`/`AssertComplete`),
  `manifest.go` (manifest type + emit), `loadmatrix.go` (PR4). Test file per source file (`*_test.go`), black-box
  `package goldengen_test` where the public surface suffices.
- **Test framework.** Match `pkg/testing/golden/golden_test.go`: standard `testing` + testify `require`/`assert`,
  table-driven subtests via `t.Run`. No Ginkgo in this package. Never `t.Fatal`; use `require`.
- **Errors.** Wrap with `fmt.Errorf("...: %w", err)`; validation errors are plain `fmt.Errorf` with the offending
  name/value quoted.
- **GoDoc.** Every exported symbol gets a GoDoc comment. The package gets a doc comment on `package goldengen` in one
  file.
- **Determinism.** Firing-set comparison and the regime signature sort names; `RegisteredMutations`/`FiringSet` preserve
  registration order. Representative version is the first in supplied `Versions` order within a regime.
- **Prose.** Docs and GoDoc avoid em dashes and double-dashes; direct, precise phrasing.

## File structure

Created:

- `pkg/testing/goldengen/unit.go`, `config.go`, `classify.go`, `generator.go`, `manifest.go`, `loadmatrix.go` (+ tests)
- `pkg/component/concepts/mutation_inspector.go`
- `docs/testing.md`
- `examples/version-matrix/...` (worked example, PR3)

Modified:

- `pkg/generic/resource_base.go` (introspection methods)
- `pkg/component/component.go` (component union + interface assertion)
- `pkg/testing/golden/golden.go` (exported `Serialize`/`SerializeComponent`; refactor internal callers to reuse)
- The 22 primitive `Resource` types under `pkg/primitives/*/resource.go` (2-method delegation each)
- `README.md`, `CLAUDE.md` (doc table + reference lists, PR3)

---

# PR1 (#132): framework introspection + serializer export

Branch: `git switch feature/version-matrix-goldens && git switch -c pr/132-mutation-introspection`

### Task 1.1: Define the `MutationInspector` interface

**Files:**

- Create: `pkg/component/concepts/mutation_inspector.go`

- [ ] **Step 1: Write the interface**

```go
package concepts

// MutationInspector surfaces, read-only, the mutations registered on a built
// resource or component and which of them fire at the version it was built at.
//
// All built-in primitives satisfy this through generic.BaseResource, and the
// component aggregates its managed resources. It is an inert capability: nothing
// in the reconcile path calls it, so importing it costs nothing at runtime.
type MutationInspector interface {
	// RegisteredMutations returns the deduplicated Names of every mutation
	// registered on the unit, independent of the version it was built at.
	RegisteredMutations() []string

	// FiringSet returns the Names of registered mutations whose gate is enabled
	// for the version the unit was built at. A mutation with a nil gate fires
	// unconditionally and is always included. It returns an error if any gate's
	// Enabled evaluation fails, since a swallowed gate error would silently
	// misclassify a version regime.
	FiringSet() ([]string, error)
}
```

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./pkg/component/concepts/...` Expected: success.

- [ ] **Step 3: Commit**

```bash
git add pkg/component/concepts/mutation_inspector.go
git commit -m "feat(concepts): add MutationInspector interface (#132)"
```

### Task 1.2: Implement introspection on `generic.BaseResource`

**Files:**

- Modify: `pkg/generic/resource_base.go`
- Test: `pkg/generic/resource_base_inspect_test.go`

- [ ] **Step 1: Write the failing test**

```go
package generic_test

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// staticGate is a feature.Gate with a fixed outcome for tests.
type staticGate struct {
	enabled bool
	err     error
}

func (g staticGate) Enabled() (bool, error) { return g.enabled, g.err }

// noopMutator is a minimal FeatureMutator for exercising BaseResource.
type noopMutator struct{}

func (noopMutator) NextFeature()  {}
func (noopMutator) Apply() error  { return nil }

func newBase(mutations ...generic.Mutation[*noopMutator]) *generic.BaseResource[*corev1.ConfigMap, *noopMutator] {
	return &generic.BaseResource[*corev1.ConfigMap, *noopMutator]{
		DesiredObject: &corev1.ConfigMap{},
		NewMutator:    func(*corev1.ConfigMap) *noopMutator { return &noopMutator{} },
		IdentityFunc:  func(client.Object) string { return "cm" }, // adjust to actual IdentityFunc signature
		Mutations:     mutations,
	}
}

func TestBaseResourceRegisteredMutations(t *testing.T) {
	base := newBase(
		generic.Mutation[*noopMutator]{Name: "A"},
		generic.Mutation[*noopMutator]{Name: "B"},
		generic.Mutation[*noopMutator]{Name: "A"}, // duplicate
	)
	assert.Equal(t, []string{"A", "B"}, base.RegisteredMutations())
}

func TestBaseResourceFiringSet(t *testing.T) {
	base := newBase(
		generic.Mutation[*noopMutator]{Name: "Always"},                                  // nil gate -> fires
		generic.Mutation[*noopMutator]{Name: "On", Feature: staticGate{enabled: true}},   // fires
		generic.Mutation[*noopMutator]{Name: "Off", Feature: staticGate{enabled: false}}, // does not fire
	)
	firing, err := base.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always", "On"}, firing)
}

func TestBaseResourceFiringSetGateError(t *testing.T) {
	base := newBase(
		generic.Mutation[*noopMutator]{Name: "Bad", Feature: staticGate{err: errors.New("boom")}},
	)
	_, err := base.FiringSet()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bad")
}
```

Note: confirm `FeatureMutator`'s real method set (open `pkg/generic/mutate_helper.go` / the `FeatureMutator` definition)
and the exact `IdentityFunc` signature before finalizing `noopMutator`/`newBase`. Adjust the stub to satisfy the real
interface; do not change production types to fit the test.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/generic/ -run TestBaseResource -v` Expected: FAIL — `RegisteredMutations`/`FiringSet` undefined.

- [ ] **Step 3: Implement the methods**

Add to `pkg/generic/resource_base.go`:

```go
// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the resource, in registration order, independent of the version it was built
// at. It satisfies concepts.MutationInspector.
func (r *BaseResource[T, M]) RegisteredMutations() []string {
	seen := make(map[string]struct{}, len(r.Mutations))
	names := make([]string, 0, len(r.Mutations))
	for _, m := range r.Mutations {
		if _, ok := seen[m.Name]; ok {
			continue
		}
		seen[m.Name] = struct{}{}
		names = append(names, m.Name)
	}
	return names
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the resource was built at, in registration order. A mutation with a nil
// Feature fires unconditionally. It returns an error if any gate's Enabled
// evaluation fails. It satisfies concepts.MutationInspector.
func (r *BaseResource[T, M]) FiringSet() ([]string, error) {
	firing := make([]string, 0, len(r.Mutations))
	for _, m := range r.Mutations {
		if m.Feature == nil {
			firing = append(firing, m.Name)
			continue
		}
		enabled, err := m.Feature.Enabled()
		if err != nil {
			return nil, fmt.Errorf("evaluating gate for mutation %q: %w", m.Name, err)
		}
		if enabled {
			firing = append(firing, m.Name)
		}
	}
	return firing, nil
}
```

(`fmt` is already imported in this file.)

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/generic/ -run TestBaseResource -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/generic/resource_base.go pkg/generic/resource_base_inspect_test.go
git commit -m "feat(generic): implement MutationInspector on BaseResource (#132)"
```

### Task 1.3: Aggregate introspection on `*component.Component`

**Files:**

- Modify: `pkg/component/component.go`
- Test: `pkg/component/component_inspect_test.go`

- [ ] **Step 1: Write the failing test**

Build a component with two managed resources and one read-only resource (use the existing test helpers in
`pkg/component/` for constructing a component and fake resources; mirror how `component` tests build
`reconcileResources`). Assert:

```go
func TestComponentRegisteredMutationsUnion(t *testing.T) {
	// managed res1 registers {"A","B"}, managed res2 registers {"B","C"},
	// read-only res3 registers {"X"} and must be excluded.
	c := buildComponentWithResources(t, /* res1, res2, readOnly(res3) */)
	assert.ElementsMatch(t, []string{"A", "B", "C"}, c.RegisteredMutations())
}

func TestComponentFiringSetUnion(t *testing.T) {
	// res1 firing {"A"}, res2 firing {"C"}; union is {"A","C"}, read-only excluded.
	c := buildComponentWithResources(t, /* ... */)
	firing, err := c.FiringSet()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A", "C"}, firing)
}

func TestComponentFiringSetPropagatesError(t *testing.T) {
	// a managed resource whose FiringSet errors makes the component error.
	c := buildComponentWithResources(t, /* erroring res */)
	_, err := c.FiringSet()
	require.Error(t, err)
}
```

Use a fake resource type that implements `component.Resource` + `concepts.MutationInspector`. Follow the construction
pattern already used by `Component.Preview` tests; if none exists, build the component via its public builder and
register fakes with `WithResource`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/component/ -run TestComponent.*Mutations -v` and `-run TestComponentFiringSet -v` Expected: FAIL —
methods undefined.

- [ ] **Step 3: Implement on Component**

Add to `pkg/component/component.go` (mirroring the `Preview` iteration: skip read-only, type-assert each managed
resource):

```go
// RegisteredMutations returns the deduplicated union of the registered mutation
// Names across the component's managed (non read-only) resources, in resource
// registration order. Resources that do not implement concepts.MutationInspector
// contribute nothing. It satisfies concepts.MutationInspector.
func (c *Component) RegisteredMutations() []string {
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, entry := range c.reconcileResources {
		if entry.Options.ReadOnly {
			continue
		}
		inspector, ok := entry.Resource.(concepts.MutationInspector)
		if !ok {
			continue
		}
		for _, name := range inspector.RegisteredMutations() {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	return names
}

// FiringSet returns the deduplicated union of the firing-set Names across the
// component's managed (non read-only) resources, in resource registration order.
// It returns an error if any managed resource's FiringSet evaluation fails. It
// satisfies concepts.MutationInspector.
func (c *Component) FiringSet() ([]string, error) {
	seen := make(map[string]struct{})
	firing := make([]string, 0)
	for _, entry := range c.reconcileResources {
		if entry.Options.ReadOnly {
			continue
		}
		inspector, ok := entry.Resource.(concepts.MutationInspector)
		if !ok {
			continue
		}
		names, err := inspector.FiringSet()
		if err != nil {
			return nil, fmt.Errorf("firing set for resource %q: %w", entry.Resource.Identity(), err)
		}
		for _, name := range names {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			firing = append(firing, name)
		}
	}
	return firing, nil
}
```

Confirm `concepts` and `fmt` are imported in `component.go` (both already are).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/component/ -run TestComponent -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/component/component.go pkg/component/component_inspect_test.go
git commit -m "feat(component): aggregate MutationInspector across managed resources (#132)"
```

### Task 1.4: Delegate introspection on every primitive `Resource`

Each primitive `Resource` wraps an unexported `base` field (a generic resource type that embeds `BaseResource`, so the
two methods are promoted). Add the identical delegation block to every primitive so the built primitive satisfies
`concepts.MutationInspector`.

**Files (modify each):** `pkg/primitives/<kind>/resource.go` for every kind: `clusterrole`, `clusterrolebinding`,
`configmap`, `cronjob`, `daemonset`, `deployment`, `hpa`, `ingress`, `job`, `networkpolicy`, `pdb`, `pod`, `pv`, `pvc`,
`replicaset`, `role`, `rolebinding`, `secret`, `service`, `serviceaccount`, `statefulset`, `unstructured`.

- [ ] **Step 1: Add a compile-time assertion + delegation to `statefulset` first (TDD anchor)**

In `pkg/primitives/statefulset/resource.go`, add:

```go
// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the StatefulSet, independent of version. It satisfies
// concepts.MutationInspector so the resource can be introspected for version-matrix
// golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the StatefulSet was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}
```

Add a compile-time interface assertion (in the same file, or a shared `var _` line):

```go
var _ concepts.MutationInspector = (*Resource)(nil)
```

- [ ] **Step 2: Write a test that the built primitive satisfies the interface and reports firing-set**

`pkg/primitives/statefulset/resource_inspect_test.go`:

```go
func TestStatefulSetMutationInspector(t *testing.T) {
	res, err := statefulset.NewBuilder(baseStatefulSet()).
		WithMutation(statefulset.Mutation{Name: "Always"}).
		Build()
	require.NoError(t, err)

	var insp concepts.MutationInspector = res
	assert.Equal(t, []string{"Always"}, insp.RegisteredMutations())
	firing, err := insp.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always"}, firing)
}
```

(Use the package's existing builder/test helpers for `baseStatefulSet()`; a mutation with a nil `Mutate` is fine for
introspection, but if `Build` applies mutations and rejects a nil handler, give it
`Mutate: func(*statefulset.Mutator) error { return nil }`.)

- [ ] **Step 3: Run — expect pass for statefulset**

Run: `go test ./pkg/primitives/statefulset/ -run TestStatefulSetMutationInspector -v` Expected: PASS.

- [ ] **Step 4: Apply the same delegation block + `var _` assertion to the remaining 21 primitives**

For each remaining kind, add the two methods (identical body `return r.base.RegisteredMutations()` /
`return r.base.FiringSet()`, GoDoc adjusted to the kind name) and the
`var _ concepts.MutationInspector = (*Resource)(nil)` assertion. Ensure `concepts` is imported (it already is in every
primitive `resource.go`). If a primitive's wrapper field is not named `base`, match the local name.

- [ ] **Step 5: Build all primitives**

Run: `go build ./pkg/primitives/...` Expected: success (the `var _` assertions catch any primitive missed).

- [ ] **Step 6: Commit**

```bash
git add pkg/primitives/
git commit -m "feat(primitives): delegate MutationInspector on every primitive resource (#132)"
```

### Task 1.5: Export the golden serializer

**Files:**

- Modify: `pkg/testing/golden/golden.go`
- Test: `pkg/testing/golden/serialize_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSerializeMatchesGoldenPath(t *testing.T) {
	dep := testDeployment() // existing helper in golden_test.go
	out, err := golden.Serialize(dep, testScheme())
	require.NoError(t, err)

	// Byte-identical to what CompareYAML writes.
	path := filepath.Join(t.TempDir(), "dep.yaml")
	require.NoError(t, golden.CompareYAML(path, fakePreviewer{dep}, golden.WithScheme(testScheme()), golden.Update(true)))
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(out))
}

func TestSerializeComponentJoinsDocuments(t *testing.T) {
	objs := []client.Object{testDeployment(), testConfigMap()}
	out, err := golden.SerializeComponent(objs, testScheme())
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(out), "kind:"))
	assert.Contains(t, string(out), "---\n")
}
```

(If `golden_test.go` is `package golden` white-box, place these in the same package and drop the `golden.` qualifier, or
add a `fakePreviewer` if not present.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/golden/ -run TestSerialize -v` Expected: FAIL — `Serialize`/`SerializeComponent` undefined.

- [ ] **Step 3: Add exported wrappers and refactor internal callers to reuse them**

In `golden.go`:

```go
// Serialize marshals a client.Object to the canonical golden YAML form: TypeMeta
// resolved (from the object or the scheme), and zero-value noise fields stripped.
// It is the same serialization CompareYAML uses, exported for tools that generate
// goldens out of band.
func Serialize(obj client.Object, scheme *runtime.Scheme) ([]byte, error) {
	return serializeObject(obj, scheme)
}

// SerializeComponent marshals multiple objects into a single multi-document YAML
// stream (--- separated, in the given order), matching CompareComponentYAML.
func SerializeComponent(objs []client.Object, scheme *runtime.Scheme) ([]byte, error) {
	docs := make([][]byte, 0, len(objs))
	for i, obj := range objs {
		doc, err := serializeObject(obj, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize object %d to YAML: %w", i, err)
		}
		docs = append(docs, doc)
	}
	return bytes.Join(docs, []byte("---\n")), nil
}
```

Refactor `CompareComponentYAML` to call `SerializeComponent` instead of inlining the loop (DRY), keeping its behavior
identical:

```go
actual, err := SerializeComponent(objs, cfg.scheme)
if err != nil {
	return fmt.Errorf("failed to serialize component objects to YAML: %w", err)
}
return compareOrUpdate(path, actual, cfg.update)
```

- [ ] **Step 4: Run to verify it passes (and existing golden tests still pass)**

Run: `go test ./pkg/testing/golden/ -v` Expected: PASS (new + existing).

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/golden/golden.go pkg/testing/golden/serialize_test.go
git commit -m "feat(golden): export Serialize and SerializeComponent (#132)"
```

### Task 1.6: PR1 verification and open PR

- [ ] **Step 1: Full gate**

Run: `make all` Expected: lint, fmt, and tests pass.

- [ ] **Step 2: Open PR1 into the feature branch**

```bash
git push -u origin pr/132-mutation-introspection
gh pr create -R sourcehawk/operator-component-framework --base feature/version-matrix-goldens --head pr/132-mutation-introspection --title "Expose registered mutations and firing-set; export golden serializer" --body "Towards #132"
```

(Follow `feature-dev-workflow:opening-a-pull-request` for the body; `Towards #132`, not `Closes`, because it targets the
feature branch.)

---

# PR2 (#133): goldengen core

Branch off the feature branch after PR1 has merged into it:
`git switch feature/version-matrix-goldens && git pull && git switch -c pr/133-goldengen-core`.

### Task 2.1: `Unit` interface and adapters

**Files:**

- Create: `pkg/testing/goldengen/unit.go`
- Test: `pkg/testing/goldengen/unit_test.go`

- [ ] **Step 1: Write the failing test**

```go
package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceAdapter(t *testing.T) {
	res := buildStatefulSetWith(t /* mutations */) // returns *statefulset.Resource
	u := goldengen.Resource(res, testScheme())

	assert.Equal(t, []string{"Always"}, u.RegisteredMutations())
	firing, err := u.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always"}, firing)
	y, err := u.RenderYAML()
	require.NoError(t, err)
	assert.Contains(t, string(y), "kind: StatefulSet")
}

func TestComponentAdapter(t *testing.T) {
	c := buildComponentWith(t /* ... */)
	u := goldengen.Component(c, testScheme())
	y, err := u.RenderYAML()
	require.NoError(t, err)
	assert.Contains(t, string(y), "---") // multi-doc when >1 resource
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestResourceAdapter -v` Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Implement `unit.go`**

```go
// Package goldengen is a test-only helper that sweeps a consumer-supplied version
// universe, classifies versions into behaviorally-distinct gating regimes by
// firing-set, generates the minimal goldens covering them, asserts per-fixture
// mutation gating, and proves every registered mutation is accounted for.
//
// It is opt-in: a consumer that does not import it pays nothing, and the core
// Build/ApplyIntent path never references it.
package goldengen

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Unit is a built resource or component the generator can introspect and render.
type Unit interface {
	// RegisteredMutations returns the deduplicated Names of every registered mutation.
	RegisteredMutations() []string
	// FiringSet returns the Names of mutations enabled at the built version.
	FiringSet() ([]string, error)
	// RenderYAML renders the unit's desired state as canonical golden YAML.
	RenderYAML() ([]byte, error)
}

// ResourcePreviewer is a built single-resource primitive: it can be introspected
// and rendered to one client.Object. Every built-in primitive satisfies it.
type ResourcePreviewer interface {
	concepts.MutationInspector
	concepts.Previewable
}

// ComponentPreviewer is a built component: introspectable and rendered to many
// client.Objects. *component.Component satisfies it.
type ComponentPreviewer interface {
	RegisteredMutations() []string
	FiringSet() ([]string, error)
	Preview() ([]client.Object, error)
}

type resourceUnit struct {
	res    ResourcePreviewer
	scheme *runtime.Scheme
}

// Resource adapts a built primitive resource to a Unit, serializing through the
// golden package with the given scheme.
func Resource(res ResourcePreviewer, scheme *runtime.Scheme) Unit {
	return resourceUnit{res: res, scheme: scheme}
}

func (u resourceUnit) RegisteredMutations() []string    { return u.res.RegisteredMutations() }
func (u resourceUnit) FiringSet() ([]string, error)     { return u.res.FiringSet() }

func (u resourceUnit) RenderYAML() ([]byte, error) {
	obj, err := u.res.Preview()
	if err != nil {
		return nil, fmt.Errorf("preview resource: %w", err)
	}
	return golden.Serialize(obj, u.scheme)
}

type componentUnit struct {
	comp   ComponentPreviewer
	scheme *runtime.Scheme
}

// Component adapts a built component to a Unit, serializing its managed resources
// into a multi-document YAML stream through the golden package.
func Component(comp ComponentPreviewer, scheme *runtime.Scheme) Unit {
	return componentUnit{comp: comp, scheme: scheme}
}

func (u componentUnit) RegisteredMutations() []string { return u.comp.RegisteredMutations() }
func (u componentUnit) FiringSet() ([]string, error) { return u.comp.FiringSet() }

func (u componentUnit) RenderYAML() ([]byte, error) {
	objs, err := u.comp.Preview()
	if err != nil {
		return nil, fmt.Errorf("preview component: %w", err)
	}
	return golden.SerializeComponent(objs, u.scheme)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestResourceAdapter -v` and `-run TestComponentAdapter -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/unit.go pkg/testing/goldengen/unit_test.go
git commit -m "feat(goldengen): Unit interface and resource/component adapters (#133)"
```

### Task 2.2: `Config`/`Fixture`/`Expect` types and validation

**Files:**

- Create: `pkg/testing/goldengen/config.go`
- Test: `pkg/testing/goldengen/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestConfigValidate(t *testing.T) {
	valid := goldengen.Config[*corev1.ConfigMap]{
		Dir:      "td",
		Versions: []string{"1.0.0", "2.0.0"},
		Scheme:   testScheme(),
		Fixtures: []goldengen.Fixture[*corev1.ConfigMap]{{
			Name: "default", Spec: &corev1.ConfigMap{},
			Requires: []goldengen.Expect{{Name: "A"}, {Name: "B", For: "2.0.0"}},
		}},
		Build: func(string, *corev1.ConfigMap) (goldengen.Unit, error) { return nil, nil },
	}
	require.NoError(t, valid.Validate())

	t.Run("for not in versions", func(t *testing.T) {
		bad := valid
		bad.Fixtures = []goldengen.Fixture[*corev1.ConfigMap]{{
			Name: "default", Spec: &corev1.ConfigMap{},
			Requires: []goldengen.Expect{{Name: "B", For: "9.9.9"}},
		}}
		err := bad.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "9.9.9")
	})

	t.Run("empty versions", func(t *testing.T) {
		bad := valid
		bad.Versions = nil
		require.Error(t, bad.Validate())
	})

	t.Run("nil build", func(t *testing.T) {
		bad := valid
		bad.Build = nil
		require.Error(t, bad.Validate())
	})

	t.Run("duplicate fixture name", func(t *testing.T) {
		bad := valid
		bad.Fixtures = append(bad.Fixtures, bad.Fixtures[0])
		require.Error(t, bad.Validate())
	})
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestConfigValidate -v` Expected: FAIL — types/`Validate` undefined.

- [ ] **Step 3: Implement `config.go`**

```go
package goldengen

import "fmt"

// Expect is an assertion that a named mutation fires (Requires) or does not fire
// (Forbids) over a fixture's version sweep. For is optional; when set it must be a
// concrete version drawn from Config.Versions and pins the assertion to that
// version instead of quantifying over the whole sweep.
type Expect struct {
	Name string
	For  string
}

// Fixture is one spec and the gating expectations the author asserts for it.
type Fixture[T any] struct {
	Name     string
	Spec     T
	Requires []Expect
	Forbids  []Expect
}

// Config declares the whole version matrix.
type Config[T any] struct {
	Dir      string
	Versions []string
	Exclude  []string
	Scheme   *runtime.Scheme
	Fixtures []Fixture[T]
	Build    func(version string, spec T) (Unit, error)
}

// Validate checks the static invariants of the config before any sweep runs:
// non-empty Versions, a non-nil Build, unique fixture names, and every Expect.For
// being a member of Versions.
func (c Config[T]) Validate() error {
	if len(c.Versions) == 0 {
		return fmt.Errorf("goldengen: Versions must not be empty")
	}
	if c.Build == nil {
		return fmt.Errorf("goldengen: Build must not be nil")
	}
	if c.Dir == "" {
		return fmt.Errorf("goldengen: Dir must not be empty")
	}

	known := make(map[string]struct{}, len(c.Versions))
	for _, v := range c.Versions {
		known[v] = struct{}{}
	}

	seenFixture := make(map[string]struct{}, len(c.Fixtures))
	for _, f := range c.Fixtures {
		if f.Name == "" {
			return fmt.Errorf("goldengen: fixture name must not be empty")
		}
		if _, dup := seenFixture[f.Name]; dup {
			return fmt.Errorf("goldengen: duplicate fixture name %q", f.Name)
		}
		seenFixture[f.Name] = struct{}{}

		for _, e := range append(append([]Expect{}, f.Requires...), f.Forbids...) {
			if e.Name == "" {
				return fmt.Errorf("goldengen: fixture %q has an expectation with an empty Name", f.Name)
			}
			if e.For != "" {
				if _, ok := known[e.For]; !ok {
					return fmt.Errorf("goldengen: fixture %q expectation %q has For %q not in Versions", f.Name, e.Name, e.For)
				}
			}
		}
	}
	return nil
}
```

Add `"k8s.io/apimachinery/pkg/runtime"` to the import block.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestConfigValidate -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/config.go pkg/testing/goldengen/config_test.go
git commit -m "feat(goldengen): config types and validation (#133)"
```

### Task 2.3: Firing-set classification and representative selection

**Files:**

- Create: `pkg/testing/goldengen/classify.go`
- Test: `pkg/testing/goldengen/classify_test.go`

A regime is a maximal group of swept versions sharing an identical firing-set. The signature is the sorted firing-set
joined; the representative is the first version in supplied order within the group.

- [ ] **Step 1: Write the failing test**

```go
func TestClassifyRegimes(t *testing.T) {
	// versions in supplied (ascending) order; firing-sets keyed by version.
	versions := []string{"1.0.0", "1.1.0", "2.0.0"}
	firing := map[string][]string{
		"1.0.0": {"A"},
		"1.1.0": {"A"},      // same regime as 1.0.0
		"2.0.0": {"A", "B"}, // new regime
	}
	regimes := goldengen.ClassifyRegimes(versions, firing)
	require.Len(t, regimes, 2)
	assert.Equal(t, "1.0.0", regimes[0].Representative)
	assert.Equal(t, []string{"1.0.0", "1.1.0"}, regimes[0].Versions)
	assert.Equal(t, []string{"A"}, regimes[0].Firing)
	assert.Equal(t, "2.0.0", regimes[1].Representative)
	assert.Equal(t, []string{"A", "B"}, regimes[1].Firing)
}

func TestClassifyOrderIndependentSignature(t *testing.T) {
	// firing-set order must not split a regime.
	versions := []string{"1.0.0", "2.0.0"}
	firing := map[string][]string{"1.0.0": {"A", "B"}, "2.0.0": {"B", "A"}}
	regimes := goldengen.ClassifyRegimes(versions, firing)
	assert.Len(t, regimes, 1)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestClassify -v` Expected: FAIL — `ClassifyRegimes`/`Regime` undefined.

- [ ] **Step 3: Implement `classify.go`**

```go
package goldengen

import (
	"sort"
	"strings"
)

// Regime is a group of swept versions sharing an identical firing-set.
type Regime struct {
	Representative string   // first version in supplied order within the group
	Versions       []string // all versions in this regime, in supplied order
	Firing         []string // the shared firing-set, sorted
}

// signature is the order-independent key identifying a firing-set.
func signature(firing []string) string {
	sorted := append([]string(nil), firing...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// ClassifyRegimes groups versions (in supplied order) by identical firing-set.
// Regimes are returned in order of first appearance; the representative of each is
// the first version in supplied order belonging to it.
func ClassifyRegimes(versions []string, firing map[string][]string) []Regime {
	index := make(map[string]int) // signature -> regime index
	regimes := make([]Regime, 0)
	for _, v := range versions {
		sorted := append([]string(nil), firing[v]...)
		sort.Strings(sorted)
		sig := strings.Join(sorted, "\x00")
		if i, ok := index[sig]; ok {
			regimes[i].Versions = append(regimes[i].Versions, v)
			continue
		}
		index[sig] = len(regimes)
		regimes = append(regimes, Regime{
			Representative: v,
			Versions:       []string{v},
			Firing:         sorted,
		})
	}
	return regimes
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestClassify -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/classify.go pkg/testing/goldengen/classify_test.go
git commit -m "feat(goldengen): firing-set classification into regimes (#133)"
```

### Task 2.4: Gating assertions (Requires/Forbids lattice)

**Files:**

- Create: `pkg/testing/goldengen/gating.go`
- Test: `pkg/testing/goldengen/gating_test.go`

Lattice over a fixture's per-version firing-sets:

- `Requires` empty `For`: name fires at some swept version.
- `Requires` `For=v`: name fires at `v`.
- `Forbids` empty `For`: name fires at no swept version.
- `Forbids` `For=v`: name does not fire at `v`.

- [ ] **Step 1: Write the failing test**

```go
func TestCheckGating(t *testing.T) {
	firing := map[string][]string{ // version -> firing-set
		"8.8.0": {"Always", "Pre89"},
		"8.9.0": {"Always", "Unified89"},
	}
	f := goldengen.Fixture[int]{
		Name: "default",
		Requires: []goldengen.Expect{
			{Name: "Always"},                       // existential: fires somewhere
			{Name: "Unified89", For: "8.9.0"},       // pinned
			{Name: "Pre89", For: "8.8.0"},
		},
		Forbids: []goldengen.Expect{
			{Name: "Unified89", For: "8.8.0"},       // not before boundary
			{Name: "Pre89", For: "8.9.0"},           // not after boundary
		},
	}
	require.NoError(t, goldengen.CheckGating(f, []string{"8.8.0", "8.9.0"}, firing))
}

func TestCheckGatingFailures(t *testing.T) {
	firing := map[string][]string{"8.9.0": {"Always"}}
	versions := []string{"8.9.0"}

	t.Run("required existential missing", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Requires: []goldengen.Expect{{Name: "Ghost"}}}
		err := goldengen.CheckGating(f, versions, firing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Ghost")
	})
	t.Run("required pinned missing", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Requires: []goldengen.Expect{{Name: "Ghost", For: "8.9.0"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
	t.Run("forbidden existential fires", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Forbids: []goldengen.Expect{{Name: "Always"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
	t.Run("forbidden pinned fires", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Forbids: []goldengen.Expect{{Name: "Always", For: "8.9.0"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestCheckGating -v` Expected: FAIL — `CheckGating` undefined.

- [ ] **Step 3: Implement `gating.go`**

```go
package goldengen

import "fmt"

// firesAt reports whether name is in the firing-set at version v.
func firesAt(firing map[string][]string, v, name string) bool {
	for _, n := range firing[v] {
		if n == name {
			return true
		}
	}
	return false
}

// firesSomewhere reports whether name is in the firing-set at any swept version.
func firesSomewhere(firing map[string][]string, versions []string, name string) bool {
	for _, v := range versions {
		if firesAt(firing, v, name) {
			return true
		}
	}
	return false
}

// CheckGating verifies a fixture's Requires/Forbids expectations against its
// per-version firing-sets. It returns a descriptive error on the first violation.
func CheckGating[T any](f Fixture[T], versions []string, firing map[string][]string) error {
	for _, e := range f.Requires {
		if e.For == "" {
			if !firesSomewhere(firing, versions, e.Name) {
				return fmt.Errorf("fixture %q: required mutation %q never fires across the version sweep", f.Name, e.Name)
			}
			continue
		}
		if !firesAt(firing, e.For, e.Name) {
			return fmt.Errorf("fixture %q: required mutation %q does not fire at %s", f.Name, e.Name, e.For)
		}
	}
	for _, e := range f.Forbids {
		if e.For == "" {
			if firesSomewhere(firing, versions, e.Name) {
				return fmt.Errorf("fixture %q: forbidden mutation %q fires somewhere across the version sweep", f.Name, e.Name)
			}
			continue
		}
		if firesAt(firing, e.For, e.Name) {
			return fmt.Errorf("fixture %q: forbidden mutation %q fires at %s", f.Name, e.Name, e.For)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestCheckGating -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/gating.go pkg/testing/goldengen/gating_test.go
git commit -m "feat(goldengen): Requires/Forbids gating assertions (#133)"
```

### Task 2.5: Manifest type and emit

**Files:**

- Create: `pkg/testing/goldengen/manifest.go`
- Test: `pkg/testing/goldengen/manifest_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestManifestYAML(t *testing.T) {
	m := goldengen.Manifest{
		Fixtures: []goldengen.FixtureManifest{{
			Name: "default",
			Regimes: []goldengen.RegimeManifest{
				{Representative: "8.8.0", Firing: []string{"Always", "Pre89"}},
				{Representative: "8.9.0", Firing: []string{"Always", "Unified89"}},
			},
		}},
	}
	out, err := m.YAML()
	require.NoError(t, err)
	assert.Contains(t, string(out), "name: default")
	assert.Contains(t, string(out), "representative: 8.8.0")
	assert.Contains(t, string(out), "Unified89")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestManifest -v` Expected: FAIL.

- [ ] **Step 3: Implement `manifest.go`**

```go
package goldengen

import "sigs.k8s.io/yaml"

// RegimeManifest records one behaviorally-distinct regime of a fixture.
type RegimeManifest struct {
	Representative string   `json:"representative"`
	Versions       []string `json:"versions"`
	Firing         []string `json:"firing"`
}

// FixtureManifest records all regimes derived for one fixture.
type FixtureManifest struct {
	Name    string           `json:"name"`
	Regimes []RegimeManifest `json:"regimes"`
}

// Manifest is the reviewable coverage map: per fixture, the distinct gating
// regimes with their representative version and firing-set.
type Manifest struct {
	Fixtures []FixtureManifest `json:"fixtures"`
}

// YAML renders the manifest as deterministic YAML.
func (m Manifest) YAML() ([]byte, error) {
	return yaml.Marshal(m)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestManifest -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/manifest.go pkg/testing/goldengen/manifest_test.go
git commit -m "feat(goldengen): coverage manifest type (#133)"
```

### Task 2.6: Generator `New` + `Run` (sweep, gating, goldens, manifest)

**Files:**

- Create: `pkg/testing/goldengen/generator.go`
- Test: `pkg/testing/goldengen/generator_test.go`

- [ ] **Step 1: Write the failing test (end-to-end with -update behavior)**

```go
func TestGeneratorRunWritesGoldensAndManifest(t *testing.T) {
	dir := t.TempDir()
	cfg := configForConfigMap(dir) // two versions producing two regimes; see helper below
	gen := goldengen.New(cfg)

	// First run with update writes goldens + manifest.
	goldengen.SetUpdate(true) // or pass via an Option; see implementation note
	gen.Run(t)

	assert.FileExists(t, filepath.Join(dir, "default", "1.0.0.yaml"))
	assert.FileExists(t, filepath.Join(dir, "default", "2.0.0.yaml"))
	assert.FileExists(t, filepath.Join(dir, "manifest.yaml"))

	// Second run without update compares clean.
	goldengen.SetUpdate(false)
	gen.Run(t)
}
```

Implementation note for `-update`: the generator reads an update flag. Match the golden package convention — expose
`func (g *Generator[T]) WithUpdate(bool) *Generator[T]` and let the consumer wire their own `-update` flag
(`gen.WithUpdate(*update)` before `Run`), rather than a package-global. Rewrite the test to use `gen.WithUpdate(true)`
accordingly (drop `SetUpdate`).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/testing/goldengen/ -run TestGeneratorRun -v` Expected: FAIL.

- [ ] **Step 3: Implement `generator.go`**

```go
package goldengen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
)

// Generator runs a version matrix: per-fixture gating assertions, minimal goldens,
// and a coverage manifest, all derived from one Config.
type Generator[T any] struct {
	cfg    Config[T]
	update bool
}

// New creates a Generator from a Config.
func New[T any](cfg Config[T]) *Generator[T] {
	return &Generator[T]{cfg: cfg}
}

// WithUpdate controls whether Run overwrites goldens and the manifest. Wire it to
// a -update test flag: gen.WithUpdate(*update).
func (g *Generator[T]) WithUpdate(enabled bool) *Generator[T] {
	g.update = enabled
	return g
}

// sweep builds every fixture at every version and returns, per fixture, the
// per-version firing-sets and the built units (for rendering representatives).
type fixtureSweep struct {
	firing map[string][]string // version -> firing-set
	units  map[string]Unit     // version -> built unit
}

func (g *Generator[T]) sweepFixture(f Fixture[T]) (fixtureSweep, error) {
	sw := fixtureSweep{firing: map[string][]string{}, units: map[string]Unit{}}
	for _, v := range g.cfg.Versions {
		unit, err := g.cfg.Build(v, f.Spec)
		if err != nil {
			return sw, fmt.Errorf("build fixture %q at %s: %w", f.Name, v, err)
		}
		firing, err := unit.FiringSet()
		if err != nil {
			return sw, fmt.Errorf("firing set for fixture %q at %s: %w", f.Name, v, err)
		}
		sw.firing[v] = firing
		sw.units[v] = unit
	}
	return sw, nil
}

// Run validates the config, asserts per-fixture gating, and writes (or compares)
// one golden per regime plus the coverage manifest. It honors WithUpdate.
func (g *Generator[T]) Run(t *testing.T) {
	t.Helper()
	if err := g.cfg.Validate(); err != nil {
		t.Fatalf("goldengen: invalid config: %v", err)
	}

	manifest := Manifest{}
	for _, f := range g.cfg.Fixtures {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			sw, err := g.sweepFixture(f)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if err := CheckGating(f, g.cfg.Versions, sw.firing); err != nil {
				t.Errorf("%v", err)
			}

			regimes := ClassifyRegimes(g.cfg.Versions, sw.firing)
			fm := FixtureManifest{Name: f.Name}
			for _, r := range regimes {
				unit := sw.units[r.Representative]
				y, err := unit.RenderYAML()
				if err != nil {
					t.Fatalf("render fixture %q regime %s: %v", f.Name, r.Representative, err)
				}
				path := filepath.Join(g.cfg.Dir, f.Name, r.Representative+".yaml")
				if err := writeOrCompareGolden(path, y, g.update); err != nil {
					t.Errorf("%v", err)
				}
				fm.Regimes = append(fm.Regimes, RegimeManifest{
					Representative: r.Representative,
					Versions:       r.Versions,
					Firing:         r.Firing,
				})
			}
			manifest.Fixtures = append(manifest.Fixtures, fm)
		})
	}

	manifestYAML, err := manifest.YAML()
	if err != nil {
		t.Fatalf("goldengen: marshal manifest: %v", err)
	}
	if err := writeOrCompareGolden(filepath.Join(g.cfg.Dir, "manifest.yaml"), manifestYAML, g.update); err != nil {
		t.Errorf("%v", err)
	}
}

// writeOrCompareGolden writes the bytes when update is set, otherwise compares
// against the file at path, returning a descriptive error on mismatch.
func writeOrCompareGolden(path string, actual []byte, update bool) error {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create golden dir: %w", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			return fmt.Errorf("write golden %s: %w", path, err)
		}
		return nil
	}
	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("golden %s does not exist; run with -update", path)
	}
	if err != nil {
		return fmt.Errorf("read golden %s: %w", path, err)
	}
	if string(expected) != string(actual) {
		return fmt.Errorf("golden mismatch at %s", path)
	}
	return nil
}
```

Note: `t.Fatalf` inside the generator is acceptable here because it is test infrastructure operating on the consumer's
`*testing.T`, not a production code path; the project's "no `t.Fatal` in tests" rule targets test bodies. Prefer
`t.Errorf` for per-golden assertions so all mismatches surface in one run; reserve `t.Fatalf` for setup failures
(invalid config, build error) that make continuing meaningless. Consider reusing `golden`'s diff in mismatch messages if
a richer diff is wanted; a follow-up can swap `writeOrCompareGolden` for a call into a small exported golden compare
helper.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/testing/goldengen/ -run TestGeneratorRun -v` Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/generator.go pkg/testing/goldengen/generator_test.go
git commit -m "feat(goldengen): Generator Run with sweep, gating, goldens, manifest (#133)"
```

### Task 2.7: PR2 verification and open PR

- [ ] **Step 1:** `make all` — expect pass.
- [ ] **Step 2:** Open PR2 into the feature branch with body `Towards #133` (see
      `feature-dev-workflow:opening-a-pull-request`).

---

# PR3 (#134): accounting + example + docs

Branch off the feature branch after PR2 merges into it.

### Task 3.1: `AssertComplete` accounting

**Files:**

- Modify: `pkg/testing/goldengen/generator.go`
- Test: `pkg/testing/goldengen/accounting_test.go`

Accounting:
`union(all Requires names across fixtures) ∪ Exclude == union(RegisteredMutations across all fixtures' built units)`.
Compute the registered universe by building each fixture once (any version; registration is version-independent) and
unioning `RegisteredMutations`. Failures: a registered name neither required nor excluded ("unaccounted mutation X"); a
stale `Exclude`/`Requires` name not in the registered universe; an empty mutation name in the registered universe.

- [ ] **Step 1: Write the failing test**

```go
func TestAssertComplete(t *testing.T) {
	t.Run("complete passes", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A", "B"}, /*requires*/ []string{"A"}, /*exclude*/ []string{"B"}))
		assert.Equal(t, 0, gen.AssertComplete(0))
	})
	t.Run("unaccounted fails", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A", "B"}, []string{"A"}, nil))
		assert.NotEqual(t, 0, gen.AssertComplete(0)) // B unaccounted
	})
	t.Run("stale exclude fails", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A"}, []string{"A"}, []string{"Ghost"}))
		assert.NotEqual(t, 0, gen.AssertComplete(0))
	})
	t.Run("passes through nonzero code", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A"}, []string{"A"}, nil))
		assert.Equal(t, 7, gen.AssertComplete(7)) // accounting ok, but incoming code is nonzero
	})
}
```

`configWithRegistered` builds a `Config` whose `Build` returns a `Unit` (a fake or a real primitive) registering the
given mutation names, with the given `Requires` (on one fixture) and `Exclude`.

- [ ] **Step 2: Run to verify it fails.** `go test ./pkg/testing/goldengen/ -run TestAssertComplete -v` → FAIL.

- [ ] **Step 3: Implement `AssertComplete`**

```go
// AssertComplete is called from the consumer's TestMain as
// os.Exit(gen.AssertComplete(m.Run())). It returns code unchanged when the
// incoming code is nonzero (tests already failed) or when accounting holds.
// Otherwise it prints the accounting violations and returns a nonzero code.
//
// Accounting holds when union(Requires names) ∪ Exclude equals the universe of
// registered mutation Names, with no empty or unaccounted names.
func (g *Generator[T]) AssertComplete(code int) int {
	if err := g.cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "goldengen: invalid config: %v\n", err)
		return 1
	}

	registered := map[string]struct{}{}
	for _, f := range g.cfg.Fixtures {
		unit, err := g.cfg.Build(g.cfg.Versions[0], f.Spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goldengen: build fixture %q for accounting: %v\n", f.Name, err)
			return 1
		}
		for _, name := range unit.RegisteredMutations() {
			if name == "" {
				fmt.Fprintf(os.Stderr, "goldengen: fixture %q registers a mutation with an empty Name\n", f.Name)
				return 1
			}
			registered[name] = struct{}{}
		}
	}

	accounted := map[string]struct{}{}
	for _, f := range g.cfg.Fixtures {
		for _, e := range f.Requires {
			accounted[e.Name] = struct{}{}
		}
	}
	for _, name := range g.cfg.Exclude {
		accounted[name] = struct{}{}
	}

	var violations []string
	for name := range registered {
		if _, ok := accounted[name]; !ok {
			violations = append(violations, fmt.Sprintf("unaccounted mutation %q (require it in a fixture or add it to Exclude)", name))
		}
	}
	for name := range accounted {
		if _, ok := registered[name]; !ok {
			violations = append(violations, fmt.Sprintf("stale name %q is required or excluded but not registered by any fixture", name))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "goldengen: %s\n", v)
		}
		return 1
	}
	return code
}
```

Add `"sort"` to the imports.

- [ ] **Step 4: Run to verify it passes.** `go test ./pkg/testing/goldengen/ -run TestAssertComplete -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/generator.go pkg/testing/goldengen/accounting_test.go
git commit -m "feat(goldengen): AssertComplete accounting assertion (#134)"
```

### Task 3.2: Worked example under `examples/`

**Files:**

- Create: `examples/version-matrix/` (a small CRD + a primitive build wired through goldengen, with `testdata/`
  goldens + manifest and a `*_test.go`)

- [ ] **Step 1:** Pick the simplest realistic surface: a StatefulSet build with two or three version-gated mutations
      (reuse an existing example's CRD if one fits, e.g. `examples/grace-inconsistency`). Write
      `version_matrix_test.go`:

```go
var update = flag.Bool("update", false, "update golden files")

var gen = goldengen.New(goldengen.Config[*MyCluster]{
	Dir:      "testdata/version_matrix",
	Versions: []string{"8.7.0", "8.8.2", "8.9.0"},
	Scheme:   scheme,
	Exclude:  []string{ /* any below-floor stubs */ },
	Fixtures: []goldengen.Fixture[*MyCluster]{
		{Name: "default", Spec: defaultCluster(), Requires: []goldengen.Expect{
			{Name: "ContainerImage"},
			{Name: "ClusterEnv/Unified89", For: "8.9.0"},
			{Name: "ClusterEnv/Pre89", For: "8.8.2"},
		}, Forbids: []goldengen.Expect{
			{Name: "ClusterEnv/Unified89", For: "8.8.2"},
		}},
	},
	Build: func(v string, c *MyCluster) (goldengen.Unit, error) {
		c.Spec.Version = v
		res, err := statefulset.NewBuilder(baseStatefulSet(c)).
			WithMutation(versionGatedMutations(c)...).
			Build()
		if err != nil {
			return nil, err
		}
		return goldengen.Resource(res, scheme), nil
	},
})

func TestVersionMatrix(t *testing.T) { gen.Run(t).WithUpdate? /* gen.WithUpdate(*update); gen.Run(t) */ }
func TestMain(m *testing.M)          { os.Exit(gen.AssertComplete(m.Run())) }
```

Fix the `TestVersionMatrix` to `gen.WithUpdate(*update); gen.Run(t)`.

- [ ] **Step 2:** Generate goldens: `go test ./examples/version-matrix/ -run TestVersionMatrix -update`. Inspect the
      written `testdata/version_matrix/default/*.yaml` and `manifest.yaml`; confirm regimes match intent.

- [ ] **Step 3:** Run clean: `go test ./examples/version-matrix/...` → PASS. Build examples: `make build-examples` →
      success.

- [ ] **Step 4: Commit**

```bash
git add examples/version-matrix/
git commit -m "docs(examples): version-matrix golden generation example (#134)"
```

### Task 3.3: `docs/testing.md` + reference updates

**Files:**

- Create: `docs/testing.md`
- Modify: `README.md` (link), `CLAUDE.md` (doc table row + `pkg/testing/goldengen` in reference lists)

- [ ] **Step 1:** Write `docs/testing.md` covering: the `golden` helpers
      (`AssertYAML`/`AssertComponentYAML`/`Serialize*`), then `goldengen` — the matrix declaration, the four assertions,
      the firing-set classification and the version-ordering convention (ascending puts representatives on inclusive
      boundaries), the manifest, and the YAML loader (added in PR4; add its section here or in PR4). Verify every symbol
      name against the source.

- [ ] **Step 2:** Add a `docs/testing.md` row to the CLAUDE.md documentation table and add `pkg/testing/goldengen` to
      the "Source to read" list. Add a link from `README.md` testing section.

- [ ] **Step 3:** `make fmt-md` → formats markdown.

- [ ] **Step 4: Commit**

```bash
git add docs/testing.md README.md CLAUDE.md
git commit -m "docs: add docs/testing.md for golden and goldengen (#134)"
```

### Task 3.4: PR3 verification and open PR

- [ ] `make all` and `make build-examples` → pass. Open PR3 into the feature branch, body `Towards #134`.

---

# PR4 (#135): YAML matrix loader

Branch off the feature branch after PR2 merges (independent of PR3).

### Task 4.1: `LoadMatrix`

**Files:**

- Create: `pkg/testing/goldengen/loadmatrix.go`
- Test: `pkg/testing/goldengen/loadmatrix_test.go`
- Test data: `pkg/testing/goldengen/testdata/matrix*.yaml`, `testdata/fixtures/*.yaml`

- [ ] **Step 1: Write the failing test**

```go
func TestLoadMatrixInline(t *testing.T) {
	cfg, err := goldengen.LoadMatrix("testdata/matrix_inline.yaml",
		func() *corev1.ConfigMap { return &corev1.ConfigMap{} },
		buildFn, testScheme())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	assert.Equal(t, []string{"1.0.0", "2.0.0"}, cfg.Versions)
	require.Len(t, cfg.Fixtures, 1)
	assert.Equal(t, "default", cfg.Fixtures[0].Name)
	assert.Equal(t, "from-inline", cfg.Fixtures[0].Spec.Name) // unmarshalled from spec:
	assert.Equal(t, "A", cfg.Fixtures[0].Requires[0].Name)
}

func TestLoadMatrixSpecFile(t *testing.T) {
	cfg, err := goldengen.LoadMatrix("testdata/matrix_specfile.yaml",
		func() *corev1.ConfigMap { return &corev1.ConfigMap{} }, buildFn, testScheme())
	require.NoError(t, err)
	assert.Equal(t, "from-file", cfg.Fixtures[0].Spec.Name) // resolved relative to matrix file
}

func TestLoadMatrixErrors(t *testing.T) {
	_, err := goldengen.LoadMatrix("testdata/matrix_both.yaml", newCM, buildFn, testScheme())
	require.Error(t, err) // spec and specFile both set
	_, err = goldengen.LoadMatrix("testdata/matrix_neither.yaml", newCM, buildFn, testScheme())
	require.Error(t, err) // neither set
	_, err = goldengen.LoadMatrix("testdata/matrix_badfor.yaml", newCM, buildFn, testScheme())
	require.Error(t, err) // For not in versions
}
```

Create the testdata files: `matrix_inline.yaml` (fixture with inline `spec:` whose `metadata.name: from-inline`),
`matrix_specfile.yaml` (`specFile: fixtures/cm.yaml`), `fixtures/cm.yaml` (`metadata.name: from-file`),
`matrix_both.yaml`, `matrix_neither.yaml`, `matrix_badfor.yaml`.

- [ ] **Step 2: Run to verify it fails.** `go test ./pkg/testing/goldengen/ -run TestLoadMatrix -v` → FAIL.

- [ ] **Step 3: Implement `loadmatrix.go`**

```go
package goldengen

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// matrixFile is the on-disk shape of a version matrix, mirroring Config minus the
// Go-only Build and Scheme.
type matrixFile struct {
	Dir      string         `json:"dir"`
	Versions []string       `json:"versions"`
	Exclude  []string       `json:"exclude"`
	Fixtures []matrixFixtureFile `json:"fixtures"`
}

type matrixFixtureFile struct {
	Name     string          `json:"name"`
	Spec     json.RawMessage `json:"spec"`     // inline CR; mutually exclusive with SpecFile
	SpecFile string          `json:"specFile"` // path relative to the matrix file
	Requires []Expect        `json:"requires"`
	Forbids  []Expect        `json:"forbids"`
}

// LoadMatrix reads a YAML matrix file and returns a Config ready to run once Build
// and Scheme are supplied (here as arguments). Each fixture's CR is taken from an
// inline spec or an external specFile (exactly one), unmarshalled into a fresh T
// from newSpec. Expect.For values are validated against the file's versions.
func LoadMatrix[T any](
	path string,
	newSpec func() T,
	build func(version string, spec T) (Unit, error),
	scheme *runtime.Scheme,
) (Config[T], error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config[T]{}, fmt.Errorf("read matrix %s: %w", path, err)
	}
	var mf matrixFile
	if err := yaml.Unmarshal(raw, &mf); err != nil {
		return Config[T]{}, fmt.Errorf("parse matrix %s: %w", path, err)
	}

	baseDir := filepath.Dir(path)
	cfg := Config[T]{
		Dir:      mf.Dir,
		Versions: mf.Versions,
		Exclude:  mf.Exclude,
		Scheme:   scheme,
		Build:    build,
	}

	for _, ff := range mf.Fixtures {
		spec, err := loadFixtureSpec(ff, baseDir, newSpec)
		if err != nil {
			return Config[T]{}, err
		}
		cfg.Fixtures = append(cfg.Fixtures, Fixture[T]{
			Name:     ff.Name,
			Spec:     spec,
			Requires: ff.Requires,
			Forbids:  ff.Forbids,
		})
	}

	if err := cfg.Validate(); err != nil {
		return Config[T]{}, err
	}
	return cfg, nil
}

func loadFixtureSpec[T any](ff matrixFixtureFile, baseDir string, newSpec func() T) (T, error) {
	var zero T
	hasInline := len(ff.Spec) > 0
	hasFile := ff.SpecFile != ""
	if hasInline == hasFile {
		return zero, fmt.Errorf("fixture %q must set exactly one of spec or specFile", ff.Name)
	}

	data := []byte(ff.Spec)
	if hasFile {
		b, err := os.ReadFile(filepath.Join(baseDir, ff.SpecFile))
		if err != nil {
			return zero, fmt.Errorf("fixture %q specFile: %w", ff.Name, err)
		}
		data = b
	}

	spec := newSpec()
	if err := yaml.Unmarshal(data, spec); err != nil {
		return zero, fmt.Errorf("fixture %q spec: %w", ff.Name, err)
	}
	return spec, nil
}
```

Add `"encoding/json"` for `json.RawMessage`. Note: `sigs.k8s.io/yaml` converts YAML to JSON under the hood, so
`json.RawMessage` for the inline `spec` captures the CR as JSON and `yaml.Unmarshal(data, spec)` decodes it into the
typed `T`. Verify this round-trip in the test; if `json.RawMessage` does not capture nested YAML cleanly through
`sigs.k8s.io/yaml`, switch `Spec` to `apiextensionsv1.JSON` or re-marshal the decoded `map[string]any` before
unmarshalling into `T`.

- [ ] **Step 4: Run to verify it passes.** `go test ./pkg/testing/goldengen/ -run TestLoadMatrix -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testing/goldengen/loadmatrix.go pkg/testing/goldengen/loadmatrix_test.go pkg/testing/goldengen/testdata/
git commit -m "feat(goldengen): optional YAML matrix loader (#135)"
```

### Task 4.2: Loader docs + example

- [ ] **Step 1:** Add a "YAML matrix loader" section to `docs/testing.md` (if not already added in PR3) showing a
      `matrix.yaml` with both inline and `specFile` fixtures and the `LoadMatrix` call. `make fmt-md`.
- [ ] **Step 2:** Optionally add a YAML-driven variant to the `examples/version-matrix` example.
- [ ] **Step 3: Commit.** `git commit -m "docs: document the goldengen YAML matrix loader (#135)"`

### Task 4.3: PR4 verification and open PR

- [ ] `make all` → pass. Open PR4 into the feature branch, body `Towards #135`.

---

## Integration

After all four sub-PRs have merged into `feature/version-matrix-goldens`:

- [ ] Run the full gate on the feature branch: `make all`, `make build-examples`.
- [ ] Consider an E2E primitive smoke if the introspection touched primitive build paths in a way unit tests do not
      cover (likely unnecessary; this is read-only metadata).
- [ ] Open the integration PR `feature/version-matrix-goldens` → `main` with body `Closes #131` (see
      `feature-dev-workflow:opening-a-pull-request`).
- [ ] Delete the spec, plan, and state files in the orchestrator's final commit once the epic is closed.

## Deviations from issue #129 (intentional)

- `FiringSet()` returns `([]string, error)` (issue wrote `[]string`): `feature.Gate.Enabled()` returns an error, and
  swallowing it would silently misclassify a regime.
- A built unit becomes a `Unit` through `goldengen.Resource(res, scheme)` / `goldengen.Component(comp, scheme)` adapters
  rather than every primitive implementing `RenderYAML` directly, keeping serialization centralized in `golden`.
- `-update` is wired via `gen.WithUpdate(*update)` rather than a package global, matching the consumer-owns-the-flag
  convention of the `golden` package.

## Self-review notes

- Spec coverage: framework introspection (PR1), classification + goldens + gating + manifest (PR2), accounting +
  example + docs (PR3), YAML loader (PR4) — every spec section maps to a task.
- The `FiringSet` error signature is consistent across `MutationInspector`, `Unit`, and both adapters.
- Method/type names used in later tasks (`ClassifyRegimes`, `Regime`, `CheckGating`, `Manifest`, `RegimeManifest`,
  `Generator.WithUpdate`, `AssertComplete`) match their definitions.
- Per-primitive delegation is enforced by `var _ concepts.MutationInspector = (*Resource)(nil)` in each primitive, so a
  missed primitive fails the build rather than silently lacking the capability.
