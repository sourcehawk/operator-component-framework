# Observability

The local Prometheus and Grafana stack for the dashboards and alert rules the framework ships, and the notes for their
maintainers. The templates themselves live in `internal/observability/templates/` and are embedded in the `ocf` CLI;
render them for your operator with `ocf observability render --metric-namespace <namespace>`, where the namespace is the
argument you gave `ocm.NewOperatorConditionsGauge`. Full documentation:
[docs/observability.md](../docs/observability.md).

From a clone, `make dashboards METRIC_NAMESPACE=myoperator` or `make alerts METRIC_NAMESPACE=myoperator` runs the same
command through `go run ./cmd/ocf` into `generated/`. `generated/alerts/` contains the per-operator condition rules,
named after the metric namespace, plus the shared `ocf-*` rules for controller-runtime and the managed resource
counters, which are installed once per cluster. Add `NAMESPACE_LABEL=namespace` if your scrape keeps the exported
`namespace` label, and `ALERT_FORMAT=rules` for plain rule files instead of `PrometheusRule` objects.
`PROMETHEUSRULE_NAMESPACE` and `PROMETHEUSRULE_LABELS` set the metadata of the `PrometheusRule` objects; for
kube-prometheus-stack pass `PROMETHEUSRULE_LABELS=release=<name>`.

The rest of this file is for maintainers of the templates.

## Local stack

`dev/` holds a docker compose file for Prometheus and Grafana and a Go simulator that plays a scripted operator. The
simulator records the framework's own series through the real `metrics.Recorder`; the controller-runtime, workqueue,
REST client and leader election series are lookalikes with the same names, labels and buckets, guarded by the parity
test below. You need docker with the compose plugin and Go.

    make observability-up

The target renders both dashboards and plain rule files for the metric namespace `demo` (`OBS_DEV_NAMESPACE`) into
`generated/dev/`, rewrites every `for:` in the rendered rules to `2m` so the alerts fire within minutes (this rewrite
exists only in the dev render), starts the containers, waits for Prometheus to become ready, and runs the simulator in
the foreground with `go run`. Prometheus listens on `127.0.0.1:9090` and Grafana on `127.0.0.1:3000` with anonymous
admin access and both dashboards provisioned into the OCF folder. Prometheus scrapes the simulator every five seconds
with the static target labels `job="demo-operator"`, `namespace="operators"` and `pod="demo-operator-0"`, so the
namespace the condition gauge exports collides into `exported_namespace` exactly as it does in a cluster. Stop the
simulator with Ctrl-C, then `make observability-down` removes the containers.

`SIMULATOR_ARGS` passes extra flags to the simulator; `make observability-up SIMULATOR_ARGS="-leader=false"` reports the
replica as a standby so `OperatorLeaderMissing` fires.

The world has two controllers, `webapp` (owner kind `WebApp`, components `server` and `ingress`) and `database` (owner
kind `Database`, components `storage` and `backup`). Each scenario targets one alert or one dashboard feature:

| Scenario                                                                                                            | Exercises                                                                                      |
| ------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Fifty healthy `WebApp` owners across five namespaces, reconciling on a jittered interval and converging with `none` | No alert fires for them; the baseline the ratio thresholds are tested against                  |
| `webapp-01`'s `server/configmap` applied as `updated` four times a second while its other resources converge        | `ManagedResourceNotConverging`, the Updated rate and Not-converging ratio panels               |
| `Database` `shop/orders-db` `Ready=False` with a `lastTransitionTime` eight hours in the past                       | `CustomResourceConditionStuck` at once, `CustomResourceNotReady` after the shortened `for:`    |
| `Database` `shop/users-db` `Ready=Unknown` and `StorageReady=Unknown`                                               | `CustomResourceConditionUnknown`, the Owners Unknown tile                                      |
| `Database` `analytics/reports-db` `Ready=False` with its reason flipping every three minutes                        | `CustomResourceNotReady` keeps firing: a reason change does not restart the `for:` clock       |
| A cluster-scoped `Database` `shared-gateway` with no namespace label                                                | The CRD Conditions Browser's wildcard All keeps it visible; the alerts' cluster-scoped wording |
| `storage/pvc` applies failing three times out of four                                                               | `ManagedResourceApplyFailing`, the Apply error ratio panel                                     |
| Thirty percent of `database` reconciles returning an error                                                          | `ControllerReconcileErrors`                                                                    |
| `database` reconciles taking five seconds to two minutes                                                            | `ControllerReconcileLatencyHigh`                                                               |
| `database` queue wait observations of several minutes with a deep queue and both workers busy                       | `ControllerWorkqueueBacklog`, the Workqueue row                                                |
| A `database` panic one minute in and every twenty minutes after                                                     | `ControllerReconcilePanics`                                                                    |
| `leader_election_master_status{name="demo-operator"}` at 1, or 0 with `-leader=false`                               | The Leader tile, `OperatorLeaderMissing`                                                       |

Within a few minutes of starting, every alert in the table except `OperatorLeaderMissing`, which needs
`SIMULATOR_ARGS="-leader=false"`, is visible as firing on `http://localhost:9090/alerts`.

## Testing

Three checks guard the artifacts. CI runs `make test-alerts` and `make lint-dashboards` in the "Alerts and dashboards"
job; the simulator parity test is an ordinary Go test and runs under `make test` in the unit-test job.

`make test-alerts` needs `promtool`, which ships with Prometheus and is deliberately left out of `make all` so
contributors without it are unaffected. It renders the rules in the plain `rules` format with the metric namespace
`test_operator` into two directories, one with `exported_namespace` and one with `namespace` as the namespace label,
lints every file with `promtool check rules --lint=all --lint-fatal`, and runs the unit tests with
`promtool test rules --diff`. Each rule file has a test file of the same name under
`internal/observability/templates/alerts/tests/` (`crd_conditions_test.yaml` for `crd_conditions.tpl.yaml`), whose
`rule_files` point one directory up at the rendered file, and every alert has a firing case plus the negative cases that
justify its design, among them legitimate churn at scale and a single edit on an idle resource for
`ManagedResourceNotConverging`, sporadic conflicts among successful applies for `ManagedResourceApplyFailing`, and a
reason change mid-window and a cluster-scoped owner for the condition rules. Annotations are compared exactly, so a
wording change shows up as a diff. After editing a rule, run `make test-alerts` and `make lint-dashboards`; the latter
also asserts that every alert still has a unit test.

`make lint-dashboards` runs `go test ./internal/observability/`, which covers `Options.Validate` and `Render` (both
alert formats, the `PrometheusRule` metadata, the output file set and the removal of a previous render) and lints the
embedded templates: it renders every template with a fixed namespace and checks that each dashboard is valid JSON, keeps
its uid (`<namespace>_<file name>`) and its disabled auto-refresh, and that no `{{placeholder}}` survived rendering. It
then pulls every metric name out of every panel query, variable query and alert expression and asserts each one exists:
the framework's own families and the condition gauge are gathered from the real collectors, the controller-runtime,
workqueue, client-go, leader election and process families from a fixed list. For the alerts it also asserts that every
rule carries only the `severity` label and has a promtool unit test. The same test pins the 17 character cap on the
metric namespace to the longest dashboard file name, so a renamed dashboard that no longer fits Grafana's 40 character
uid limit fails here.

`go test ./observability/dev/simulator/` includes the parity test that keeps the simulator honest: it starts a real,
unmanaged controller-runtime controller, drives one request through its workqueue so controller-runtime initialises its
reconcile and workqueue series, then asserts that every lookalike family the simulator defines exists in
controller-runtime's registry with the same type, label names and histogram buckets. A controller-runtime upgrade that
renames a label the dashboards depend on fails this test rather than emptying a panel.
