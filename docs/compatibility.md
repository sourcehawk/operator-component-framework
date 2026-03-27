# Compatibility

## Supported Versions

The framework is tested against the following version combinations:

| Framework | controller-runtime | k8s.io/\* | Kubernetes | Go   | Status  |
| --------- | ------------------ | --------- | ---------- | ---- | ------- |
| main      | v0.23.x            | v0.35.x   | 1.35       | 1.25 | Primary |
| main      | v0.22.x            | v0.34.x   | 1.34       | 1.25 | Tested  |
| main      | v0.21.x            | v0.33.x   | 1.33       | 1.25 | Tested  |
| main      | v0.20.x            | v0.32.x   | 1.32       | 1.25 | Tested  |
| main      | v0.19.x            | v0.31.x   | 1.31       | 1.25 | Tested  |
| main      | v0.18.x            | v0.30.x   | 1.30       | 1.25 | Tested  |

**Primary** is the version combination used in `go.mod` and in the main CI pipeline. **Tested** versions are verified
weekly by the compatibility CI workflow.

## Version Policy

The framework targets the latest stable controller-runtime release as its primary dependency. Compatibility is tested
against the five prior controller-runtime minor versions, back to v0.18 (Kubernetes 1.30). When a new Kubernetes minor
version is released and controller-runtime publishes a matching release, the oldest tested version is dropped from the
matrix but may still be supported.

## How Compatibility Is Tested

The [compatibility workflow](../.github/workflows/compatibility.yml) runs weekly and on demand. For each version
combination in the matrix, it:

1. Swaps the `controller-runtime` and `k8s.io/*` dependencies to the target versions using `go get`.
2. Runs `go mod tidy` to resolve transitive dependencies.
3. Verifies that the entire module compiles (`go build ./...`).
4. Builds all examples (`make build-examples`).
5. Runs the full unit and envtest test suite (`make test`).

The Makefile automatically detects the correct envtest binary version from the `k8s.io/api` module version, so no manual
configuration is needed when testing against different Kubernetes versions.

## Pinning Your Kubernetes and controller-runtime Versions

When you `go get` this framework, Go's [Minimum Version Selection](https://go.dev/ref/mod#minimal-version-selection)
(MVS) will pull your `controller-runtime` and `k8s.io/*` dependencies up to at least the versions declared in the
framework's `go.mod`. If you are already on newer versions, Go will keep yours. But if you are on older versions, MVS
will bump them.

To prevent this and stay on your current versions, pin them in your `go.mod` **after** adding the framework:

```bash
# 1. Add the framework
go get github.com/sourcehawk/operator-component-framework@latest

# 2. Pin your desired controller-runtime and k8s versions
go get sigs.k8s.io/controller-runtime@v0.19.0 \
  k8s.io/api@v0.31.0 \
  k8s.io/apimachinery@v0.31.0 \
  k8s.io/client-go@v0.31.0 \
  k8s.io/apiextensions-apiserver@v0.31.0

# 3. Clean up
go mod tidy
```

The `go get` calls in step 2 write explicit `require` directives into your `go.mod`, which override MVS for those
modules. As long as those directives remain, future `go get` of the framework will not bump them.

This works because the framework's public API surface uses abstract interfaces (`client.Object`, `client.Client`) that
remain stable across controller-runtime minor versions. The compatibility CI verifies that this downgrade path compiles
and passes tests.
