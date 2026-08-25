# CLI

`ocf` generates the custom-resource wrapper pattern that [Custom Resources](custom-resource.md) describes and renders
the Grafana dashboards and Prometheus alert rules that [Observability](observability.md) describes. Templates are
embedded in the binary, so generated code and rendered artifacts match the framework version the CLI was built from.

## Installation

```bash
go install github.com/sourcehawk/operator-component-framework/cmd/ocf@latest
ocf version
```

`ocf version` prints the framework version the binary was built from, so you can confirm which template set a generated
package or a rendered dashboard came from. Install at the version your operator builds against (`@v0.x.y` instead of
`@latest`) when the two must agree, as they must for the observability artifacts.

## `ocf observability render`

`ocf observability render` writes the Grafana dashboards and Prometheus alert rules for one operator into
`<out>/dashboards/*.json` and `<out>/alerts/*.yaml`, keyed on the operator's metric namespace.

| Flag                         | Required | Default              | Meaning                                                                                                        |
| ---------------------------- | -------- | -------------------- | -------------------------------------------------------------------------------------------------------------- |
| `--metric-namespace`         | yes      |                      | The argument to `ocm.NewOperatorConditionsGauge`. A letter, then `[A-Za-z0-9_]`, at most 17 characters         |
| `--namespace-label`          | no       | `exported_namespace` | Label carrying the owner's namespace on condition series. Must be a Prometheus label name                      |
| `--alert-format`             | no       | `prometheusrule`     | `prometheusrule` wraps each rule file in a `PrometheusRule` object; `rules` writes plain `groups:` files       |
| `--prometheusrule-namespace` | no       |                      | `metadata.namespace` of the `PrometheusRule` objects                                                           |
| `--prometheusrule-labels`    | no       |                      | Comma-separated `key=value` pairs for `metadata.labels` of the `PrometheusRule` objects, written sorted by key |
| `--out`                      | no       | `./observability`    | Output directory                                                                                               |

```bash
ocf observability render --metric-namespace myoperator \
  --prometheusrule-namespace monitoring --prometheusrule-labels release=kube-prometheus-stack
```

```
Rendered observability artifacts for metric namespace "myoperator" in ./observability:
  dashboards/crd_conditions_browser.json
  dashboards/ocf_operator.json
  alerts/controller_runtime.yaml
  alerts/crd_conditions.yaml
  alerts/managed_resources.yaml
```

Rendered files are regenerated output, so there is no `--force`: every `.json` in `<out>/dashboards/` and every `.yaml`
in `<out>/alerts/` is removed before writing, and files of other types in those directories are left alone. A value that
fails validation is rejected with an error naming the flag, for example
`--metric-namespace "my-operator" is not renderable`, and nothing is written. What each artifact contains, how the
placeholders are substituted and how to install the output is documented in [Observability](observability.md#rendering).

## `ocf scaffold wrapper`

`ocf scaffold wrapper` generates a custom-resource wrapper package for a Kubernetes kind the built-in primitives do not
cover.

| Flag               | Required | Default                                                                        | Meaning                                                                  |
| ------------------ | -------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| `--type`           | yes      |                                                                                | Wrapped Go type as `<import-path>.<TypeName>`, split on the last dot     |
| `--variant`        | yes      |                                                                                | `static`, `workload`, `task`, or `integration`                           |
| `--group`          | yes      |                                                                                | API group as a DNS subdomain. Pass `--group ""` for core API group types |
| `--version`        | no       | last import-path segment when it looks like an API version; required otherwise | API version, for example `v1`, `v2beta1`                                 |
| `--kind`           | no       | the type name                                                                  | Kind used in the identity string                                         |
| `--cluster-scoped` | no       | `false`                                                                        | Omit the namespace segment and require an empty namespace                |
| `--alias`          | no       | derived                                                                        | Import alias for the wrapped type's package                              |
| `--package`        | no       | lowercased kind                                                                | Go package name of the generated package                                 |
| `--out`            | no       | `./<package>`                                                                  | Output directory                                                         |
| `--force`          | no       | `false`                                                                        | Write into a non-empty directory                                         |

### Group and version validation

Both values end up verbatim in the generated identity string, so `ocf` checks them before it writes anything.

`--group` must be a DNS subdomain, the way Kubernetes defines API groups: lowercase letters, digits, `-` and `.`, with
every dot-separated label starting and ending in a letter or digit. `apps`, `cert-manager.io` and
`rbac.authorization.k8s.io` all pass. The one exception is `--group ""`, which selects the core API group and makes the
identity string a bare `<version>/<Kind>/...`.

`--version` must be a lowercase `v` followed by digits, optionally followed by `alpha` or `beta` and more digits, for
example `v1`, `v2beta1`, or `v1alpha3`. The same check applies whether you pass `--version` yourself or `ocf` derives it
from the import path, so an explicit version and a derived one always mean the same thing.

A value that fails either check is rejected with an error naming the flag, for example
`--version "1.0" is not a valid API version`, and no files are written.

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
  `WithGuard`, `WithDataGuard`, `WithOptionalData`, `WithMetricsIdentifier`, the `WithCustom*` status setters), and
  `Build()` returns the `Resource`.
- `builder_test.go` tests `Build()` validation, that a registered mutation applies through the mutator, declared data
  extraction with `ExtractInto`, and `WithDataGuard`/`WithOptionalData` gating.
- `mutator.go` defines `Mutator`, which records metadata and object edits and applies them in a single pass when
  `Apply()` runs.
- `resource.go` defines `Resource`, which delegates every lifecycle method to the generic base: `Identity`, `Object`,
  `Mutate`, `MetricsIdentifier`, the variant's status and suspension methods, `GuardStatus`, `ExtractData`,
  `ProducedData`, `ConsumedData`, `RecordObservation`, `Preview`, `RegisteredMutations`, and `FiringSet`.

Follow the printed next steps: run `go mod tidy` so `github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1`
resolves in your module, then `go test ./certificate/...` to verify the generated package builds and passes before you
start replacing the scaffolded defaults.

## Import handling

The import path in `--type` is everything before the last dot, and all four generated files import it, so `ocf` checks
its shape before it writes anything. It must be slash-separated elements of ASCII letters, digits and `-`, `.`, `_`, `~`
or `+`, with no empty element and no element starting or ending in a dot. Anything else is rejected with
`--type import path "..." is not a valid Go import path` and no files are written. Only the shape is checked, so a
well-formed path to a package that does not exist still passes here.

`--alias` defaults to a derived name when omitted: the sanitized second-to-last import-path segment concatenated with
the last segment, lowercased and with every character that cannot appear in a Go identifier stripped. In the example
above, `github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1` derives `certmanagerv1` from `certmanager` and
`v1`. When the last segment does not look like an API version, the sanitized last segment is used alone. Pass `--alias`
to override the derived name. If no valid Go identifier can be derived at all, `ocf` exits with an error and `--alias`
must be passed explicitly.

`--version` defaults to the import path's last segment only when that segment matches the API-version pattern described
in [Group and version validation](#group-and-version-validation). When the last segment does not match, for example an
import path ending in `/api` or `/types`, `ocf` exits with `--version is required` and you must pass `--version`
explicitly.

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
