# Version-matrix golden generation — design

Status: draft for review
Date: 2026-06-01
Issue: sourcehawk/operator-component-framework#129

## Problem

Consumers render version-spanning resources by registering version-gated mutations and letting each `feature.Gate`
decide whether it fires for the concrete `spec.version`. Today they verify this with hand-picked whole-resource goldens
at a few hand-chosen versions. A real consumer audit (camunda-operator zeebe) found two failure modes this leaves open:

- **Silent regime gaps.** A key was correct for 8.9 but a no-op for 8.8, and the goldens only covered 8.9, so nothing
  caught it. Coverage of version regimes is guessed, not derived.
- **No proof a mutation is exercised where assumed.** A registered mutation can sit dead, or fire for the wrong
  fixture/version, with every existing test green.

We want an optional, test-only helper, driven by one declarative matrix, that (a) derives the minimal goldens covering
every behaviorally-distinct version regime, and (b) asserts intentionally that every mutation fires for the fixtures and
versions the author assumed, and that every registered mutation is consciously either covered or excluded.

## Goals

- Derive the minimal set of goldens per fixture: one per behaviorally-distinct version regime, not one per version.
- Assert per-fixture that each mutation fires exactly when the author assumed (`Requires`/`Forbids`, optionally pinned
  to a concrete version via `For`), catching mis-targeted or off-by-a-boundary gates.
- Assert totality: every registered mutation is either required by some fixture or explicitly excluded; nothing falls
  through silently.
- Emit a reviewable coverage manifest mapping each `(fixture, regime)` to its representative version and firing-set.
- Be strictly opt-in. A consumer that does not import the helper pays nothing; the core `Build`/`ApplyIntent` path never
  references it.

## Non-goals

- Cross-package coverage aggregation. A package-scoped `TestMain` covers a component whose tests live in one package;
  spanning packages is out of scope.
- Deriving version boundaries. The framework never computes or parses gate constraints (see the hard constraint below).
- Changing how mutations are registered, gated, or applied. This feature only reads existing state.
- Encoding `Build` or `Scheme` as data. The optional YAML matrix loader makes the declaration data-driven, but `Build`
  (which registers the version-gated mutations under test) and `Scheme` are behaviour and Go objects respectively, and
  stay in Go.

## Hard constraint — the framework stays black-box about gating

The framework must not introspect constraints. The only surface it may use is the existing
`feature.Gate.Enabled() (bool, error)` (and, for version gates, the constraint already bound at build time). No boundary
extraction, no assuming constraints are parseable strings, no reaching into how a consumer encodes versions.
Consequences that shape the whole design:

- **The candidate version universe is supplied by the consumer** as `[]string`. The framework never derives boundaries;
  it only evaluates gates and classifies.
- **Classify by firing-set, not rendered output.** Two versions are the same regime iff the vector of `Enabled()`
  results across the mutations is identical. Classifying by raw bytes is wrong: any mutation that embeds the version
  verbatim (e.g. an image tag `app:{version}`) makes every version render differently, exploding to one class per
  version. Firing-set grouping is immune — such a mutation fires in every class, so it never splits them — and is the
  correct granularity: a value interpolated from the version is not a regime boundary.

## The layered model this enables

Four non-overlapping layers, no redundancy:

1. **Per-mutation unit tests** (consumer already has these): what each mutation puts in the object.
2. **Per-fixture gating** (new): each mutation fires exactly when the author assumed, catching mis-targeted or
   off-by-a-boundary gates.
3. **Integration golden** (the version-matrix value golden): the *merged* render per `(fixture, regime)` is correct
   (ordering, an override shadowing a default, no accidental key collision), which the per-mutation tests cannot see.
4. **Accounting / totality**: every registered mutation is either required by some fixture or explicitly excluded.

## Design

### Framework additions (public, agnostic, read-only)

A new introspection interface in `pkg/component/concepts`:

```go
// MutationInspector surfaces, read-only, the mutations registered on a built
// resource or component and which of them fire at the version it was built at.
type MutationInspector interface {
    // RegisteredMutations returns the deduplicated Names of every registered
    // mutation, independent of version.
    RegisteredMutations() []string
    // FiringSet returns the Names of registered mutations whose gate is enabled
    // for the version the unit was built at. It returns an error if any gate's
    // Enabled() evaluation fails, since a swallowed gate error would silently
    // misclassify a regime.
    FiringSet() ([]string, error)
}
```

- Implemented once on `generic.BaseResource`, iterating its existing `Mutations []feature.Mutation[M]`: a nil `Feature`
  fires unconditionally; otherwise the result of `Feature.Enabled()`. Every primitive inherits it through the same
  delegation it already uses for `Preview`.
- Implemented on `*component.Component` as the **union** across its non-read-only resources' results.
- These are inert getters: they execute only when called, so the optionality requirement holds.

To let the test-only helper reuse the existing serialization (so a generated golden is byte-identical to a hand-written
`golden.AssertYAML`/`AssertComponentYAML` one), the `pkg/testing/golden` package exports thin wrappers over its existing
private serializer:

```go
func Serialize(obj client.Object, scheme *runtime.Scheme) ([]byte, error)
func SerializeComponent(objs []client.Object, scheme *runtime.Scheme) ([]byte, error)
```

The existing public `golden` API is unchanged.

### The test-only package `pkg/testing/goldengen`

```go
type Expect struct {
    Name string
    For  string // optional; a concrete version drawn from Versions. empty = quantified over the sweep.
}

type Fixture[T any] struct {
    Name     string
    Spec     T
    Requires []Expect // must fire; their names form the coverage backbone
    Forbids  []Expect // must not fire; targeted, opt-in
}

type Config[T any] struct {
    Dir      string
    Versions []string                          // consumer-supplied candidate universe
    Exclude  []string                          // mutations consciously not covered
    Scheme   *runtime.Scheme                   // used for serialization / TypeMeta
    Fixtures []Fixture[T]
    Build    func(version string, spec T) (Unit, error)
}

type Unit interface {
    RegisteredMutations() []string
    FiringSet() ([]string, error)
    RenderYAML() ([]byte, error)
}

func New[T any](cfg Config[T]) *Generator[T]
func (g *Generator[T]) Run(t *testing.T)             // gating + minimal goldens + manifest, honors -update
func (g *Generator[T]) AssertComplete(code int) int  // accounting; called from the consumer's TestMain
```

A built unit satisfies `Unit` through thin adapters, so serialization stays centralized in `golden` and primitives gain
only the two introspection methods:

```go
func Resource(r ResourcePreviewer) Unit   // wraps a built primitive resource
func Component(c ComponentPreviewer) Unit  // wraps a built component
```

The scheme used by `RenderYAML` comes from `Config.Scheme`, threaded into the adapter by the generator, so the consumer
states it once.

### Optional YAML matrix loader

The typed `Config[T]` is the core API. On top of it, an optional loader lets the entire declaration — everything except
the `Build` closure and `Scheme` — live in a `matrix.yaml`. The matrix is pure data, so it maps naturally to YAML;
encoding it this way also lets a consumer reuse real example or e2e CR manifests as fixtures, which is especially
natural at the component level where a fixture is just the top-level CR.

```go
// LoadMatrix reads a matrix file, unmarshals each fixture spec into a fresh T,
// and returns a Config[T] ready to run once Build and Scheme are supplied.
func LoadMatrix[T any](path string, newSpec func() T, build func(version string, spec T) (Unit, error), scheme *runtime.Scheme) (Config[T], error)
```

`newSpec` returns a freshly allocated `T` (e.g. `func() *MyCluster { return &MyCluster{} }`) so each fixture's spec
unmarshals into its own typed value; `T` is generic, so the loader cannot allocate it itself.

The matrix file shape mirrors `Config` minus the Go-only fields:

```yaml
versions: ["8.7.0", "8.8.2", "8.9.0", "8.10.0-alpha1"]
exclude: ["Pre8p5RestAPI", "Pre8p8OrchestrationIdentity"]
fixtures:
  - name: default
    spec: # inline CR ...
      apiVersion: my.group/v1
      kind: MyCluster
      spec: { ... }
    requires:
      - { name: ContainerImage }
      - { name: ClusterEnv/Unified89, for: "8.9.0" }
    forbids:
      - { name: ClusterEnv/Unified89, for: "8.8.2" }
  - name: s3
    specFile: fixtures/s3-cluster.yaml # ... or an external manifest, reusing a real CR
    requires:
      - { name: BackupStorageEnv/S3 }
    forbids:
      - { name: BackupStorageEnv/GCS }
```

A fixture supplies its CR either inline under `spec:` or by reference via `specFile:` (a path resolved relative to the
matrix file). Exactly one of the two is set per fixture; supplying both or neither is a load error. `Dir` may be set in
the matrix or supplied programmatically after load. The loader validates the same invariants `Run`/`AssertComplete` do
(e.g. `for` values, once the version sweep is known), surfacing typos at load time where it can.

The loader is itself opt-in: a consumer who prefers the typed `Config[T]` never imports it and writes no YAML.

### `Expect.For` semantics (a clean lattice; `For` is optional on both lists)

| entry      | `For` empty                   | `For = v`             |
| ---------- | ----------------------------- | --------------------- |
| `Requires` | fires at **some** swept version | fires **at** `v`      |
| `Forbids`  | fires at **no** swept version   | does **not** fire at `v` |

Bare `Requires` is existential, which proves "fires somewhere" but cannot catch a boundary that is off (a gate written
`>= 8.8` instead of `>= 8.9` still fires at 8.9). `For` pins the point, so a regime split is expressible from both sides
and the "correct for 8.9, no-op for 8.8" failure is caught.

`For` must be a concrete version drawn from `Versions` (validated; errors on typo or stale value). Never a range or a
re-statement of the gate constraint — `For: ">= 8.9.0"` would test the gate against a copy of itself, a tautology. A
literal point is an independent oracle.

### The four assertions, derived from one config

1. **Per-fixture gating** (`Run`). For each fixture, sweep `Versions`, build each unit, read `FiringSet()`. Each
   `Requires` holds (existential or `For`-pinned) and each `Forbids` holds (negated lattice).
2. **Minimal goldens** (`Run`, honors `-update`). Per fixture, group its swept versions by firing-set; write one value
   golden per regime at a deterministic representative version.
3. **Accounting / totality** (`AssertComplete`). `union(all Requires names) ∪ Exclude == RegisteredMutations`. Anything
   registered but neither required nor excluded fails as "unaccounted mutation X". A stale `Exclude` or `Requires` name
   not present in `RegisteredMutations` also fails. Empty mutation names are rejected — an unnamed mutation cannot be
   accounted for.
4. **Coverage manifest** (emitted by `Run`). `(fixture, regime) -> representative version -> firing-set`, written to
   `<Dir>/manifest.yaml` as a reviewable artifact and the independent "behaviorally-distinct regimes" map.

### Representative version selection

The representative for a regime is the **first version in the consumer's supplied `Versions` order** that falls in that
regime's firing-set group. No version parsing, no sorting — fully black-box, consistent with "the framework never
derives boundaries." When the consumer lists `Versions` ascending (the documented convention), the representative is the
lowest version of each regime, which is exactly the **inclusive boundary** where the regime begins; a `>= v` vs `> v`
gate bug therefore renders at `v` and is caught for free.

### Golden naming and manifest

- Goldens are named by representative version: `<Dir>/<fixture>/<version>.yaml`. Readable in diffs and reviews. When a
  new mutation splits a regime, the manifest diff explains the shift as "one regime split into two."
- `<Dir>/manifest.yaml` is always emitted and committed. Per fixture it lists each regime with its `representative`
  version and `firing` (the actual fired mutation names). It is the source of truth for which regimes exist and why, and
  doubles as the capability-audit map a consumer otherwise builds by hand.

## Design guards (keep it sound and non-brittle)

- **Do not require `firing-set ⊆ Requires`.** A fixture is not forced to list every mutation that fires for it
  (including always-on ones). `Requires` is the backbone — list the distinctive mutations; the always-on set is required
  once by a base fixture. `Forbids` and `For` are opt-in for the invariants that matter; the full per-fixture firing-set
  lives in the manifest. This avoids the brittleness of restating every render.
- **The good brittleness this imposes:** adding a new mutation forces exactly one decision — put its name in some
  fixture's `Requires` or in `Exclude`. You cannot add a mutation and silently forget to cover it.
- `Requires` is existential over the sweep by default, so one fixture swept across versions covers a mutation's several
  regimes without per-version lists; reach for `For` only on boundary-sensitive splits.

## Optionality

- A separate test-only package. Not importing it costs nothing; the core path never references it.
- The completeness/accounting assertion is user-invoked. Go permits one `TestMain` per package and it belongs to the
  consumer, so the framework offers `gen.AssertComplete(code)` for the user to call from their `TestMain`. Not calling
  it means no enforcement.
- The new accessors are inert getters; they do nothing unless called.

## Target consumer surface

```go
var gen = goldengen.New(goldengen.Config[*MyCluster]{
    Dir:      "testdata/version_matrix",
    Versions: []string{"8.7.0", "8.8.2", "8.9.0", "8.10.0-alpha1"},
    Scheme:   scheme,
    Exclude:  []string{ // consciously not covered: below-floor fail-loud stubs
        "Pre8p5RestAPI", "Pre8p8OrchestrationIdentity",
    },
    Fixtures: []goldengen.Fixture[*MyCluster]{
        {Name: "default", Spec: fx0, Requires: []goldengen.Expect{
            {Name: "ContainerImage"}, {Name: "JVMEnv"},
            {Name: "ClusterEnv/Unified89", For: "8.9.0"},
            {Name: "ClusterEnv/Pre89", For: "8.8.2"},
        }, Forbids: []goldengen.Expect{
            {Name: "ClusterEnv/Unified89", For: "8.8.2"},
            {Name: "ClusterEnv/Pre89", For: "8.9.0"},
        }},
        {Name: "s3", Spec: fx1,
            Requires: []goldengen.Expect{{Name: "BackupStorageEnv/S3"}},
            Forbids:  []goldengen.Expect{{Name: "BackupStorageEnv/GCS"}}},
    },
    Build: func(v string, fx *MyCluster) (goldengen.Unit, error) {
        fx.Spec.Version = v
        res, err := statefulset.Build(fx /*, ... */)
        if err != nil {
            return nil, err
        }
        return goldengen.Resource(res), nil
    },
})

func TestVersionMatrix(t *testing.T) { gen.Run(t) }
func TestMain(m *testing.M)           { os.Exit(gen.AssertComplete(m.Run())) }
```

## Risks and mitigations

- **Firing-set classification cost.** The sweep builds each fixture once per version. This is test-time only and bounded
  by `len(Fixtures) * len(Versions)`; acceptable for the candidate universes consumers supply.
- **Representative shift on refinement.** Mitigated by the manifest, which renders a regime split as a readable diff
  rather than an unexplained rename.
- **Empty / duplicate mutation names.** Surfaced by the accounting assertion rather than silently tolerated, so the
  invariant that every mutation is named and accountable is enforced where it matters.

## Alternatives considered

- **Classify regimes by rendered bytes.** Rejected: version interpolation explodes it to one class per version (see the
  hard constraint).
- **Framework derives boundaries by parsing constraints.** Rejected: violates the black-box constraint and couples the
  framework to how consumers encode versions.
- **Name goldens by firing-set hash for stability.** Rejected in favor of readable version-labelled names plus the
  manifest; reviewers value readability and the manifest already absorbs the "why did this split" explanation.
- **Primitives implement `Unit.RenderYAML` directly.** Rejected in favor of `goldengen` adapters so serialization stays
  centralized in `golden` and primitives gain only the two read-only introspection methods.

## Implementation breakdown

Ships as a feature branch with four sub-PRs: framework introspection + serializer export; the `goldengen` core (config,
adapters, firing-set classifier, gating assertions, goldens, manifest); accounting + a worked example + `docs/testing.md`;
and the optional YAML matrix loader (`LoadMatrix`, inline/`specFile` resolution, load-time validation). Ordering and
contracts live in the plan, not here.
