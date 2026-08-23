# Observability

Grafana dashboards and Prometheus alert rules for operators built on the framework, plus a local stack to look at them.
Full documentation: [docs/observability.md](../docs/observability.md).

Render for your operator, where `METRIC_NAMESPACE` is the argument you gave `ocm.NewOperatorConditionsGauge`:

    make dashboards METRIC_NAMESPACE=myoperator
    make alerts METRIC_NAMESPACE=myoperator

Output lands in `generated/`. `generated/alerts/` contains the per-operator condition rules, named after the metric
namespace, plus the shared `ocf-*` rules for controller-runtime and the managed resource counters, which are installed
once per cluster. Add `NAMESPACE_LABEL=namespace` if your scrape keeps the exported `namespace` label, and
`ALERT_FORMAT=rules` for plain rule files instead of `PrometheusRule` objects. `PROMETHEUSRULE_NAMESPACE` and
`PROMETHEUSRULE_LABELS` set the metadata of the `PrometheusRule` objects; for kube-prometheus-stack pass
`PROMETHEUSRULE_LABELS=release=<name>`.

Run the alert unit tests with `make test-alerts` (needs `promtool`), and bring up Prometheus and Grafana with the
simulator behind them with `make observability-up`.
