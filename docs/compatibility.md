# Compatibility

## Supported Versions

The framework is tested against the following version combinations:

| Framework | controller-runtime | k8s.io/\* | Kubernetes | Go   | Status  |
| --------- | ------------------ | --------- | ---------- | ---- | ------- |
| main      | v0.23.x            | v0.35.x   | 1.35       | 1.25 | Primary |
| main      | v0.22.x            | v0.34.x   | 1.34       | 1.25 | Tested  |

**Primary** is the version combination used in `go.mod` and in the main CI pipeline. **Tested** versions are verified
weekly by the compatibility CI workflow.

## Version Policy

The framework targets the latest stable controller-runtime release as its primary dependency. Compatibility is tested
against prior controller-runtime minor versions where transitive dependencies remain compatible. When a new Kubernetes
minor version is released and controller-runtime publishes a matching release, the matrix is updated accordingly.

Versions v0.21 and below are incompatible due to multiple transitive dependency module path migrations in the Kubernetes
ecosystem (`structured-merge-diff` v4 to v6, `yaml` package path changes) that cannot be resolved by swapping direct
dependencies alone.

## How Compatibility Is Tested

The
[compatibility workflow](https://github.com/sourcehawk/operator-component-framework/blob/main/.github/workflows/compatibility.yml)
runs weekly on a schedule, on manual dispatch, and on pull requests labeled `compatibility`. For each version
combination in the matrix, it:

1. Swaps the `controller-runtime` and `k8s.io/*` dependencies to the target versions using `go get`, then runs
   `go mod tidy` to resolve transitive dependencies. This step is skipped for the primary (current `go.mod`) entry,
   which is tested as-is.
2. Verifies that the entire module compiles (`go build ./...`).
3. Builds all examples (`make build-examples`).
4. Runs the full unit and envtest test suite (`make test`).

The Makefile automatically detects the correct envtest binary version from the `k8s.io/api` module version, so no manual
configuration is needed when testing against different Kubernetes versions.

## Pinning Your Kubernetes and controller-runtime Versions

When you `go get` this framework, Go's [Minimum Version Selection](https://go.dev/ref/mod#minimal-version-selection)
(MVS) will pull your `controller-runtime` and `k8s.io/*` dependencies up to at least the versions declared in the
framework's `go.mod`. If you are already on newer versions, Go will keep yours. But if you are on older versions, MVS
will bump them.

To prevent this, add `replace` directives to your `go.mod` that pin the versions you need:

```go
// go.mod
replace (
    sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.22.0
    k8s.io/api => k8s.io/api v0.34.0
    k8s.io/apimachinery => k8s.io/apimachinery v0.34.0
    k8s.io/client-go => k8s.io/client-go v0.34.0
    k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.0
)
```

`replace` directives override MVS regardless of what the framework's `go.mod` declares. After adding the directives, run
`go mod tidy` to update the dependency graph.

This works because the framework's public API surface uses abstract interfaces (`client.Object`, `client.Client`) that
remain stable across controller-runtime minor versions. The compatibility CI verifies that this downgrade path compiles
and passes tests.
