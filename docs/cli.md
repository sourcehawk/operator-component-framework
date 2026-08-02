# CLI

`ocf` generates the custom-resource wrapper pattern that [Custom Resources](custom-resource.md) describes. Templates are
embedded in the binary, so generated code matches the framework version the CLI was built from.

## Installation

```bash
go install github.com/sourcehawk/operator-component-framework/cmd/ocf@latest
ocf version
```

`ocf version` prints the framework version the binary was built from, so you can confirm which template set a generated
package came from.

## `ocf scaffold wrapper`

`ocf scaffold wrapper` generates a custom-resource wrapper package for a Kubernetes kind the built-in primitives do not
cover.

| Flag               | Required | Default                                                                        | Meaning                                                              |
| ------------------ | -------- | ------------------------------------------------------------------------------ | -------------------------------------------------------------------- |
| `--type`           | yes      |                                                                                | Wrapped Go type as `<import-path>.<TypeName>`, split on the last dot |
| `--variant`        | yes      |                                                                                | `static`, `workload`, `task`, or `integration`                       |
| `--group`          | yes      |                                                                                | API group. Pass `--group ""` for core API group types                |
| `--version`        | no       | last import-path segment when it looks like an API version; required otherwise | API version                                                          |
| `--kind`           | no       | the type name                                                                  | Kind used in the identity string                                     |
| `--cluster-scoped` | no       | `false`                                                                        | Omit the namespace segment and require an empty namespace            |
| `--alias`          | no       | derived                                                                        | Import alias for the wrapped type's package                          |
| `--package`        | no       | lowercased kind                                                                | Go package name of the generated package                             |
| `--out`            | no       | `./<package>`                                                                  | Output directory                                                     |
| `--force`          | no       | `false`                                                                        | Write into a non-empty directory                                     |

### Choosing a variant

`--variant` selects the generic resource category the wrapper builds on. Each category maps to a different set of
lifecycle interfaces, so the fields your wrapped kind reports readiness through determine which one fits. See
[Choose a resource category](custom-resource.md#1-choose-a-resource-category) for the full explanation.

| Category        | Generic type                  | Lifecycle interfaces                                                     | Use when                                               |
| --------------- | ----------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------ |
| **Workload**    | `generic.WorkloadResource`    | `Alive`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable`       | Long-running processes with replica-based health       |
| **Static**      | `generic.StaticResource`      | `Guardable`, `DataExtractable`                                           | Configuration objects with no runtime health semantics |
| **Task**        | `generic.TaskResource`        | `Completable`, `Suspendable`, `Guardable`, `DataExtractable`             | Run-to-completion workloads                            |
| **Integration** | `generic.IntegrationResource` | `Operational`, `Graceful`, `Suspendable`, `Guardable`, `DataExtractable` | External-dependency objects (services, ingresses)      |

## Worked example

This scaffolds a wrapper for cert-manager's `Certificate` CRD, an external-dependency object, so it uses the integration
variant:

```bash
ocf scaffold wrapper \
  --type github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1.Certificate \
  --variant integration \
  --group cert-manager.io
```

```
Generated integration wrapper package "certificate" in ./certificate:
  builder.go
  builder_test.go
  mutator.go
  resource.go

Next steps:
  1. Run go mod tidy so github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1 resolves in your module.
  2. Run go test ./certificate/... to verify the generated package.
  3. Replace the scaffolded default handlers in builder.go with Certificate-specific logic.
```

Neither `--kind`, `--package`, nor `--version` were passed: the kind defaults to `Certificate` (the type name), the
package defaults to `certificate` (the lowercased kind), and the version defaults to `v1` (the import path's last
segment, which matches the API-version pattern). The four generated files:

- `builder.go` registers the scaffolded default handlers, exposes the fluent configuration API (`WithMutation`,
  `WithGuard`, `WithDataGuard`, `WithOptionalData`, the `WithCustom*` status setters), and `Build()` returns the
  `Resource`.
- `builder_test.go` tests `Build()` validation, that a registered mutation applies through the mutator, declared data
  extraction with `ExtractInto`, and `WithDataGuard`/`WithOptionalData` gating.
- `mutator.go` defines `Mutator`, which records metadata and object edits and applies them in a single pass when
  `Apply()` runs.
- `resource.go` defines `Resource`, which delegates every lifecycle method to the generic base: `Identity`, `Object`,
  `Mutate`, the variant's status and suspension methods, `GuardStatus`, `ExtractData`, `ProducedData`, `ConsumedData`,
  `RecordObservation`, `Preview`, `RegisteredMutations`, and `FiringSet`.

Follow the printed next steps: run `go mod tidy` so `github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1`
resolves in your module, then `go test ./certificate/...` to verify the generated package builds and passes before you
start replacing the scaffolded defaults.

## Import handling

`--alias` defaults to a derived name when omitted: the sanitized second-to-last import-path segment concatenated with
the last segment, lowercased and with every character that cannot appear in a Go identifier stripped. In the example
above, `github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1` derives `certmanagerv1` from `certmanager` and
`v1`. When the last segment does not look like an API version, the sanitized last segment is used alone. Pass `--alias`
to override the derived name. If no valid Go identifier can be derived at all, `ocf` exits with an error and `--alias`
must be passed explicitly.

`--version` defaults to the import path's last segment only when that segment looks like an API version: a lowercase `v`
followed by digits, optionally followed by `alpha` or `beta` and more digits, for example `v1`, `v2beta1`, or
`v1alpha3`. When the last segment does not match, for example an import path ending in `/api` or `/types`, `ocf` exits
with `--version is required` and you must pass `--version` explicitly.

`ocf` never edits your `go.mod`. It only prints a next-steps block telling you to run `go mod tidy` (or `go get`) if
your module does not already depend on the wrapped type's package.

## What the generated defaults do

Which `Default*Handler` functions get generated depends on the variant. Static generates none of them: it has no status,
grace, or suspension semantics, so its builder registers nothing beyond mutation and data-extraction support.

| Handler                           | Generated for               | Reports unconditionally                                   |
| --------------------------------- | --------------------------- | --------------------------------------------------------- |
| `DefaultConvergingStatusHandler`  | Workload, Task              | Workload: `Healthy`. Task: `Completed`.                   |
| `DefaultOperationalStatusHandler` | Integration                 | `Operational`                                             |
| `DefaultGraceStatusHandler`       | Workload, Integration       | `Healthy`                                                 |
| `DefaultSuspensionStatusHandler`  | Workload, Task, Integration | `Suspended`                                               |
| `DefaultSuspendMutationHandler`   | Workload, Task, Integration | No mutation; the object is left untouched while suspended |
| `DefaultDeleteOnSuspendHandler`   | Workload, Task, Integration | `false`; the object is kept, not deleted                  |

Every reason string these handlers return starts with "Scaffolded default", so they are easy to grep for once you start
replacing them. The status handler (`DefaultConvergingStatusHandler` or `DefaultOperationalStatusHandler`) is required
by the generic layer: `Build()` fails without one, so the scaffold registers it in `NewBuilder` and the builder's setter
can only replace it, never clear it. If you replace the status handler, keep the grace handler consistent with it: see
[Keeping convergence and grace consistent](custom-resource.md#keeping-convergence-and-grace-consistent).

## What the CLI does not check

`ocf` never loads the target Go package. It does not verify that `--type` refers to a real, exported struct, that the
type satisfies `client.Object`, or that your module depends on the package that defines it. A wrong `--type`, a type
that does not satisfy `client.Object`, or a missing module dependency all surface at your first `go build`.

## Regenerating

`ocf scaffold wrapper` refuses to write into a non-empty output directory unless `--force` is passed. With `--force`, it
overwrites the four generated files (`builder.go`, `builder_test.go`, `mutator.go`, `resource.go`) and leaves every
other file in the directory alone.
