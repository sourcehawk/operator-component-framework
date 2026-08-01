---
name: testing-operators
description:
  Use when writing or updating tests for an operator built on the operator-component-framework - the three test layers,
  mutation unit tests, golden snapshot tests (pkg/testing/golden), version-matrix golden generation
  (pkg/testing/goldengen), the YAML matrix loader, and integration helpers (pkg/testing/integration).
---

# Testing Operators

## The three layers

Test a component from the inside out. Each layer asserts something the layer below cannot:

| Layer         | What you assert                                                        | Tool                                                       |
| ------------- | ---------------------------------------------------------------------- | ---------------------------------------------------------- |
| **Mutation**  | one mutation makes the field changes you intend, on a baseline         | testify, against `Preview()`                               |
| **Resource**  | the right mutations fire for a spec, and the rendered output is pinned | `golden` for a snapshot, `goldengen.Resource` for coverage |
| **Component** | the whole component renders the resources you expect, applied together | `golden.AssertComponentYAML`, or `goldengen.Component`     |

Two framework packages back this: `pkg/testing/golden` for single-build snapshot tests, and `pkg/testing/goldengen` for
declarative coverage across versions and specs. Both are opt-in and import nothing into the reconcile path, so a
consumer that does not test against them pays nothing.

## Mutation tests

A mutation is a pure function: given a baseline object, it makes a specific, isolated set of field changes. Test it as
an input/output pair, not against a golden file. Build a minimal baseline primitive with only the mutation under test,
preview it, and assert the fields it changed:

```go
func TestDebugLoggingMutation(t *testing.T) {
    res, err := deployment.NewBuilder(baseDeployment()).
        WithMutation(features.DebugLoggingMutation(true)).
        Build()
    require.NoError(t, err)

    dep, err := res.Preview()
    require.NoError(t, err)

    container := dep.(*appsv1.Deployment).Spec.Template.Spec.Containers[0]
    assert.Contains(t, container.Env, corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
}
```

There is no golden file at this layer: the assertion states intent directly, against the specific fields the mutation is
documented to change. Share a minimal `baseDeployment()` / `baseConfigMap()` baseline across a package's mutation tests
in a `helpers_test.go` so each test declares only what it exercises.

## Golden snapshots

`golden` renders a built primitive or component to canonical YAML and compares it against a checked-in file. What gets
snapshotted is the full rendered desired state, not the mutation list or internal builder state: serialization resolves
`TypeMeta` (from the object or a supplied scheme) and strips zero-value noise, so the golden reflects only meaningful
desired state.

Typed Kubernetes objects (built-in primitives and standard `k8s.io/api` types) do not populate `TypeMeta` on their own.
Serializing one without a scheme fails with an incomplete-`TypeMeta` error, so pass `golden.WithScheme(scheme)` to every
`AssertYAML` and `AssertComponentYAML` call; the scheme only needs to register the types being serialized.

`AssertYAML` accepts a `golden.Previewer` (`Preview() (client.Object, error)`); `AssertComponentYAML` accepts a
`golden.ComponentPreviewer` (`Preview() ([]client.Object, error)`). All built-in primitives satisfy `Previewer` through
`generic.BaseResource`, and a built `*component.Component` satisfies `ComponentPreviewer` directly.

```go
var update = flag.Bool("update", false, "update golden files")

func TestDeploymentGolden(t *testing.T) {
    res, err := resources.NewDeploymentResource(owner)
    require.NoError(t, err)

    previewer, ok := res.(golden.Previewer)
    require.True(t, ok)
    golden.AssertYAML(t, "testdata/deployment.yaml", previewer,
        golden.WithScheme(scheme), golden.Update(*update))
}
```

### Regeneration

`golden.Update(*update)` overwrites the golden file instead of comparing. Generate once, inspect the diff, then commit:

```bash
go test ./path/to/pkg -run TestDeploymentGolden -update
go test ./path/to/pkg -run TestDeploymentGolden
```

The `-update` flag goes after the package path: `go test -update ./...` passes `-update` to `go test` itself, which
rejects it. Golden files live in a `testdata/` directory next to the test file (Go excludes `testdata/` from the build).

Non-`testing.T` variants (`CompareYAML`, `CompareComponentYAML`) return a `*MismatchError` carrying a unified diff
instead of failing a test, for use outside a test body. `golden.Serialize` and `golden.SerializeComponent` produce the
canonical YAML bytes directly when you need them out of band; `goldengen` is built on exactly these two functions.

### Why mutation names matter

`goldengen` (and golden introspection generally) classifies which mutations fired by reading a resource's
`RegisteredMutations()` and `FiringSet()`, the `concepts.MutationInspector` interface every built resource and component
implements. Those sets are keyed by mutation name, and the name is what `Requires`/`Forbids` assertions, `Exclude`
entries, and the completeness check all refer to. A mutation with an empty name is itself a completeness violation. Give
every mutation a stable, descriptive name (for example `PeerDiscovery/V2`): it is the identifier the whole coverage and
gating story is built on, and it is what shows up in the generated manifest when a reviewer reads a gating diff.

## goldengen version matrices

A resource with version-gated mutations behaves differently across versions, but only where a gate actually flips.
`goldengen` sweeps a declared set of versions and specs, groups the versions by which mutations fire (a "regime"), and
writes one golden per distinct regime instead of one golden per version, then proves through `AssertComplete` that every
registered mutation was asserted somewhere.

A matrix is warranted once a resource or component has version-gated mutations whose behavior needs to be pinned across
more than one version: asserting one golden per version at that point is wasteful (versions inside the same regime
produce identical output) and, more importantly, does not prove where the behavior boundary actually is. A single golden
snapshot is enough when there is nothing version-gated to sweep, or you only care about one fixed version's output;
reach for a matrix specifically to lock down a gate boundary, and pin both sides of it (`Requires` at the version after
the boundary, `Forbids` at the version before) so the boundary itself is asserted, not just "fires somewhere".

Declare the sweep with `goldengen.Config[T]` (`Dir`, `Versions`, `Fixtures`, `Exclude`, `Build`), where `Build` adapts a
version-and-spec pair into a `goldengen.Unit` via `goldengen.Resource(res, scheme)` or
`goldengen.Component(comp, scheme)`:

```go
var gen = goldengen.New(goldengen.Config[*app.ExampleApp]{
    Dir:      "testdata/version_matrix",
    Versions: []string{"1.0.0", "1.5.0", "2.0.0"},
    Fixtures: []goldengen.Fixture[*app.ExampleApp]{{
        Name: "default",
        Spec: defaultCluster(),
        Requires: []goldengen.Expect{
            {Name: "ContainerImage"},
            {Name: "PeerDiscovery/PreV2", For: "1.5.0"},
            {Name: "PeerDiscovery/V2", For: "2.0.0"},
        },
        Forbids: []goldengen.Expect{
            {Name: "PeerDiscovery/V2", For: "1.5.0"},
        },
    }},
    Build: func(version string, spec *app.ExampleApp) (goldengen.Unit, error) {
        c := spec.DeepCopyObject().(*app.ExampleApp)
        c.Spec.Version = version
        res, err := resources.NewStatefulSetResource(c)
        if err != nil {
            return nil, err
        }
        return goldengen.Resource(res, scheme), nil
    },
})
```

`Build` must deep-copy the incoming spec before setting the version: it is called once per version for the same fixture,
and the spec is shared across that sweep. List `Versions` ascending; the representative golden for a regime is named
after the first version (in supplied order) that belongs to it, so ascending order puts each golden's filename on the
lower inclusive boundary of its gating range.

Wire a sweep into a normal test:

```go
func TestVersionMatrix(t *testing.T) {
    gen.WithUpdate(*update)
    gen.Run(t)
}

func TestMain(m *testing.M) {
    os.Exit(gen.AssertComplete(m.Run()))
}
```

`Run` validates the config, builds every fixture at every version, checks each `Requires`/`Forbids` during the sweep,
then writes (under `-update`) or compares one golden per regime plus a reviewable `manifest.yaml` (per fixture, each
regime's representative version, the versions it covers, and its firing set). `AssertComplete`, called from `TestMain`,
is a separate, registration-based check: it fails when a registered mutation is named in neither `Requires` nor
`Exclude`, or when a `Requires`/`Exclude` name matches nothing registered. Registering a new version-gated mutation
therefore fails the suite until it is asserted or deliberately excluded.

## YAML matrix loader

`goldengen.LoadMatrix[T](path, newSpec, build)` loads a matrix's `Dir`, `Versions`, `Fixtures` (with their `Requires` /
`Forbids` / `Exclude`) from a YAML file instead of Go source, keeping the version universe and fixture data separate
from the build logic, which still lives in the `build` callback passed to `LoadMatrix`. Each fixture supplies its spec
either inline under `spec:` or from an external file under `specFile:`, exactly one of the two. Reach for the YAML
loader when the matrix data (versions, fixtures, gating expectations) is more naturally maintained as data files than as
a Go literal, for example when non-Go-writing maintainers curate fixtures, or the same matrix shape is reused across
several resources with only the data changing. `LoadMatrix` returns a validated `Config[T]`; wrap it with
`goldengen.New(cfg)` and call `Run` exactly as with a Go-declared config. It errors if a fixture sets both `spec` and
`specFile` (or neither), if a `for` value is not in `versions`, or if a spec fails to unmarshal into `T`.

## Anti-patterns

- **Regenerating goldens to make a failing test pass without reading the diff.** A golden mismatch means the rendered
  output changed; run with `-update` only after confirming the new output is the output you intend, not as a reflex to
  clear a red test.
- **Asserting on implementation internals instead of rendered output.** Golden and goldengen tests exist to pin what
  gets applied to the cluster; asserting on builder state or mutation call counts instead of the previewed object misses
  the thing that actually matters to a consumer.
- **Skipping the version matrix for version-gated mutations.** A single golden at one version cannot prove where a gate
  boundary sits; a version-gated mutation without a matrix (or without `Requires`/`Forbids` pinning both sides of its
  boundary) is unverified at the versions that matter most.

## Ground truth

The consumer's resolved module version is the source of truth, not these docs. Before asserting an exact signature,
method name, or option:

1. Read the framework version from the consumer's `go.mod` entry for
   `github.com/sourcehawk/operator-component-framework`.
2. Verify the symbol with `go doc github.com/sourcehawk/operator-component-framework/pkg/<package> <Symbol>`.

The reference files bundled with this skill match the framework version this plugin shipped with. When they disagree
with `go doc`, `go doc` wins.

## References

- `references/testing.md`: full testing documentation. Read when you need exact `goldengen` config field semantics, the
  completeness accounting rules, or the YAML matrix file format.
