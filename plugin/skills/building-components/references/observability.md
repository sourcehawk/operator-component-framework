# Observability

The framework ships Grafana dashboards and Prometheus alert rules for the metrics an operator built on it exposes: the
[condition and resource apply metrics](component.md#metrics) recorded through `pkg/metrics`, and the reconcile,
workqueue, REST client, leader election and process series every controller-runtime operator exports. They live under
`observability/` in the repository as templates, keyed on the metric namespace of your operator, and render with `make`.
A local Prometheus and Grafana stack fed by a simulator lets you look at every panel and every alert without a cluster.

## What ships

| Artifact                  | File                                         | Scope                                                                                                                                                                                                                                     |
| ------------------------- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OCF Operator dashboard    | `dashboards/ocf_operator.tpl.json`           | Per operator. Rendered uid `<metric_namespace>_ocf_operator`. Operator health end to end: reconciliation, workqueue, managed resource applies, condition summary, API client, process.                                                    |
| CRD Conditions Browser    | `dashboards/crd_conditions_browser.tpl.json` | Per operator. Rendered uid `<metric_namespace>_crd_conditions_browser`. Per-owner condition drill-down; the target of the condition alerts' `dashboard_url` links.                                                                        |
| Condition alerts          | `alerts/crd_conditions.tpl.yaml`             | Per operator, rendered per metric namespace as the `PrometheusRule` `<metric-namespace>-crd-conditions`. `CustomResourceNotReady`, `CustomResourceConditionUnknown`, `CustomResourceConditionStuck`.                                      |
| Managed resource alerts   | `alerts/managed_resources.yaml`              | Shared, cluster-wide. Installed once as the `PrometheusRule` `ocf-managed-resources`, whichever operator rendered it. `ManagedResourceNotConverging`, `ManagedResourceApplyFailing`.                                                      |
| Controller-runtime alerts | `alerts/controller_runtime.yaml`             | Shared, cluster-wide. Installed once as the `PrometheusRule` `ocf-controller-runtime`. `ControllerReconcileErrors`, `ControllerReconcilePanics`, `ControllerWorkqueueBacklog`, `ControllerReconcileLatencyHigh`, `OperatorLeaderMissing`. |

The split follows the metrics. The condition gauge is named after the metric namespace
(`<metric_namespace>_controller_condition`), so its rules and both dashboards carry a placeholder and render per
operator. The apply counters (`ocf_resource_apply_total`, `ocf_resource_apply_errors_total`) and the controller-runtime
families have fixed names shared by every operator in the cluster, so their rules contain no placeholder, tell operators
apart by label, and are installed once. `make alerts` writes the shared files alongside the per-operator one on every
render; apply them from whichever operator's render you like, the content is identical.

## Rendering

The render pipeline is `make` and `sed`; it needs no Go toolchain. Three different things are called a namespace on this
page, so to be precise: the **metric namespace** is the string your operator passed to `ocm.NewOperatorConditionsGauge`
(the prefix of the condition gauge's name), the **owner namespace** is the Kubernetes namespace of a custom resource,
carried as a label on its condition series, and the **operator namespace** is the Kubernetes namespace the operator pod
runs in, stamped on every series by the scrape job.

Clone the repository and, from its root, render with your metric namespace:

```bash
make dashboards METRIC_NAMESPACE=myoperator
make alerts METRIC_NAMESPACE=myoperator
```

Output lands in `observability/generated/`, which is gitignored:

```
observability/generated/
├── alerts/
│   ├── controller_runtime.yaml   PrometheusRule ocf-controller-runtime (shared)
│   ├── crd_conditions.yaml       PrometheusRule myoperator-crd-conditions
│   └── managed_resources.yaml    PrometheusRule ocf-managed-resources (shared)
└── dashboards/
    ├── crd_conditions_browser.json
    └── ocf_operator.json
```

Two placeholders are substituted: `{{operator_namespace}}` becomes `<METRIC_NAMESPACE>_`, so every reference to the
condition gauge reads `myoperator_controller_condition` (the placeholder is named for the metric namespace, not a
Kubernetes one), and `{{namespace_label}}` becomes the value of `NAMESPACE_LABEL`. Pass the same variables to both
`make dashboards` and `make alerts`. Both placeholders and every variable below are the same as in
[go-crd-condition-metrics](https://github.com/sourcehawk/go-crd-condition-metrics), so a build that already renders that
repository's artifacts needs no change.

| Variable                   | Default                   | Effect                                                                                                                                                                                                  |
| -------------------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `METRIC_NAMESPACE`         | required                  | The argument to `ocm.NewOperatorConditionsGauge`. Names the condition gauge, the dashboard uids and the condition `PrometheusRule`.                                                                     |
| `NAMESPACE_LABEL`          | `exported_namespace`      | The label carrying the owner's namespace on condition series, see below.                                                                                                                                |
| `ALERT_FORMAT`             | `prometheusrule`          | `prometheusrule` wraps each rule file in a `monitoring.coreos.com/v1` `PrometheusRule` for the Prometheus Operator; `rules` writes the plain `groups:` file that Prometheus loads through `rule_files`. |
| `PROMETHEUSRULE_NAMESPACE` | unset                     | `metadata.namespace` of the `PrometheusRule` objects. Unset leaves it to `kubectl apply -n`.                                                                                                            |
| `PROMETHEUSRULE_LABELS`    | unset                     | Comma-separated `key=value` pairs written to `metadata.labels`. A kube-prometheus-stack install selects rules by its release label, so pass `PROMETHEUSRULE_LABELS=release=<release name>`.             |
| `OBS_OUT`                  | `observability/generated` | Render output directory.                                                                                                                                                                                |

`PrometheusRule` names are the metric namespace or `ocf` prefix joined to the file name, lower-cased, with `_` and `:`
folded to `-`, so a namespace of `My_Operator` renders as `my-operator-crd-conditions`.

### The namespace label

The condition gauge exports the owner's namespace as a `namespace` label. When a ServiceMonitor or PodMonitor scrapes
the operator, Prometheus stamps the scrape target's own labels on every series, and the target's `namespace` label (the
operator pod's namespace) collides with the exported one. Prometheus resolves the collision by renaming the exported
label to `exported_namespace`, which is why that is the default. Pass `NAMESPACE_LABEL=namespace` when your scrape sets
`honorLabels: true`, or does not stamp a `namespace` target label at all, so the exported label arrives unchanged.

The setting affects the condition rules and both dashboards. Nothing else is namespace-scoped by owner: the apply
counters carry no owner namespace by design, and the `namespace` the controller-runtime and managed-resource rules
aggregate by is the operator's own, stamped by the scrape job, which is what lets two installs of one operator in a
cluster alert separately. Outside a cluster that label is simply absent, which is harmless.

### Installing

With the default `ALERT_FORMAT`, apply the rendered rules into the namespace your Prometheus Operator watches:

```bash
make alerts METRIC_NAMESPACE=myoperator PROMETHEUSRULE_NAMESPACE=monitoring PROMETHEUSRULE_LABELS=release=kube-prometheus-stack
kubectl apply -f observability/generated/alerts/
```

With `ALERT_FORMAT=rules`, add the three files to your Prometheus `rule_files`.

For the dashboards, either import the two JSON files through Grafana's UI or API, or, with the Grafana sidecar that
kube-prometheus-stack deploys, ship them as a ConfigMap carrying the sidecar's label (`grafana_dashboard` by default):

```bash
kubectl create configmap myoperator-dashboards -n monitoring --from-file=observability/generated/dashboards/
kubectl label configmap myoperator-dashboards -n monitoring grafana_dashboard=1
```

Check the rendered files into the repository that deploys your operator, and re-render when you upgrade the framework.
Tune thresholds and `for:` durations in the rendered files, or with a kustomize patch over them; the sections below say
what each threshold means so the change is deliberate. Give the two shared `PrometheusRule` objects one owner in the
cluster: two teams applying differently tuned copies under the same name overwrite each other, and applying them into
two namespaces installs every shared alert twice.

## Naming the controller

The OCF Operator dashboard filters four metric families with one `controller` variable: controller-runtime's reconcile
series, the workqueue series, the framework's apply counters and the condition gauge. controller-runtime labels the
first two with the name of the controller, which is the lower-cased kind passed to `For` unless `Named` overrides it.
The framework labels the last two with the name you pass to `metrics.NewRecorder`. For the dashboard to correlate them,
the two names must be the same:

```go
ctrl.NewControllerManagedBy(mgr).
    For(&v1.WebApp{}).
    Named("webapp"). // optional, "webapp" is the default for kind WebApp
    Complete(r)

recCtx := component.ReconcileContext{
    // ...
    Metrics: metrics.NewRecorder("webapp", conditions, collectors),
}
```

The alerts key on the same label, so the `controller` in a `ManagedResourceNotConverging` notification and the
`controller` in a `ControllerReconcileErrors` notification then name the same thing. `OperatorLeaderMissing` is the one
exception: leader election is per operator, not per controller, and its `name` label is the lease name.

## Alerts

Every rule ships with `severity: warning` and no routing labels; severity, thresholds and routing are yours to tune. No
rule on the apply counters or the controller-runtime metrics creates a series per owner: they aggregate by the
operator's static topology, so the same rules hold whether the operator manages three owners or three thousand. The
per-owner signal comes from the condition rules.

### Managed resources

Shared, installed once as `ocf-managed-resources`. Both rules key on
`(namespace, controller, owner_kind, component, resource, kind)`: the labels of `ocf_resource_apply_total` plus the
scrape namespace. They fire per resource type, not per owner, because the counters carry no owner identity.

| Alert                          | Fires when                                           | Threshold                                                                     | `for` |
| ------------------------------ | ---------------------------------------------------- | ----------------------------------------------------------------------------- | ----- |
| `ManagedResourceNotConverging` | most applies of one resource type rewrite the object | `updated / all applies` over 15m `> 0.5`, and more than 15 updates in 15m     | 15m   |
| `ManagedResourceApplyFailing`  | most apply attempts of one resource type fail        | `errors / (errors + applies)` over 15m `> 0.5`, and more than 5 errors in 15m | 15m   |

`ManagedResourceNotConverging` is the alert the apply counters were built for: a managed resource rewritten on every
reconcile. [Resource metrics](component.md#resource-metrics) explains what an `updated` apply on every reconcile means
and why events do not catch it.

The rule measures the share of a resource's own applies that rewrote it, not the absolute `updated` rate. A bare
`rate(updated) > 0` is wrong at scale: legitimate spec changes across many owners keep the aggregate `updated` rate
above zero indefinitely. Legitimate churn is followed by a `none` apply on the next reconcile, which keeps its ratio at
or below one half; a hot loop pushes the ratio to one. The floor of 15 updates in 15 minutes keeps a single edit on an
otherwise idle resource from producing a ratio of one over a handful of samples. Raise the floor if your operator's
resources are edited in bursts; lower the ratio only if you are sure your reconcile cadence never produces a `none`
between two legitimate updates.

`ManagedResourceApplyFailing` counts every failure of an attempt: mutating the desired object, the server-side apply
patch, and the classification after it. Transient conflicts among successful applies stay under the ratio, and a
resource failing for one owner among many stays under the floor; that owner's `Ready` condition goes `False` and the
condition rules catch it. The `or` in the denominator keeps the ratio defined for a resource that has never applied
successfully, where no success series exists to add. The framework records no event for a failed apply, so the `kubectl`
command in the notification's description lists the owners' `Ready` condition reason and message, which is where the
failure lives.

### Controller-runtime

Shared, installed once as `ocf-controller-runtime`. All but the last rule aggregate by `(namespace, controller)`.

| Alert                            | Fires when                                     | Threshold                                                                        | `for` |
| -------------------------------- | ---------------------------------------------- | -------------------------------------------------------------------------------- | ----- |
| `ControllerReconcileErrors`      | a controller's reconciles mostly return errors | `result="error"` share of `controller_runtime_reconcile_total` over 10m `> 0.25` | 15m   |
| `ControllerReconcilePanics`      | a reconcile panicked                           | `increase(controller_runtime_reconcile_panics_total[10m]) > 0`                   | none  |
| `ControllerWorkqueueBacklog`     | items wait too long for a worker               | p99 of `workqueue_queue_duration_seconds` over 10m `> 100` seconds               | 15m   |
| `ControllerReconcileLatencyHigh` | reconciles are slow                            | p99 of `controller_runtime_reconcile_time_seconds` over 10m `> 30` seconds       | 15m   |
| `OperatorLeaderMissing`          | no replica holds the leader lease              | `max by (namespace, name) (leader_election_master_status) == 0`                  | 5m    |

The thresholds are ratios and quantiles rather than absolute rates for the same reason as above: they hold at any scale.
The backlog rule uses queue wait time rather than queue depth, because no depth is right for every operator, whereas
items waiting minutes for a worker is wrong at any scale.

Both quantile thresholds sit on histogram bucket bounds so that they mean what they say. controller-runtime's reconcile
time histogram has 60 seconds as its largest finite bucket, and `histogram_quantile` never returns more than the last
finite bound, so a threshold of 60 or above could never fire; 30 is the highest bound that leaves room above it. The
workqueue histogram has one bucket per decade, so `> 100` means more than one percent of items waited longer than 100
seconds, and the p99 value reported in the notification is interpolated within that bucket. Move these thresholds only
to another bucket bound: the reconcile histogram's bounds from ten seconds up are 10, 15, 20, 25, 30, 40, 50 and 60, and
the workqueue histogram's are 1, 10, 100 and 1000. A threshold between two bounds, such as 45, fires exactly like the
bound below it and only reads as if it were stricter.

`OperatorLeaderMissing` is silent in two cases: when leader election is off, because the gauge is not exported at all,
and when no replica is alive to export it, for example a crash loop before the elector starts. Pair it with your
platform's target-down alert for the operator's scrape job.

### Conditions

Per operator, rendered as `<metric-namespace>-crd-conditions`. The metric value of
`<metric_namespace>_controller_condition` is the condition's `lastTransitionTime`, which the rules rely on.

| Alert                            | Fires when                                                     | Threshold                                                                                   | `for` |
| -------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | ----- |
| `CustomResourceNotReady`         | an owner's `Ready` condition is `False`                        | `max by (controller, kind, name, <namespace label>)` of `condition="Ready", status="False"` | 30m   |
| `CustomResourceConditionUnknown` | any condition of an owner is `Unknown`                         | `max by (controller, kind, name, condition, <namespace label>)` of `status="Unknown"`       | 30m   |
| `CustomResourceConditionStuck`   | an owner's `Ready` condition has not been `True` for six hours | `time() - max(...)` of `condition="Ready", status!="True"` `> 21600`                        | none  |

Every rule aggregates with `max()` instead of matching series directly, and that is load bearing twice over. The
aggregation drops the `reason`, `status` and `id` labels, so a controller that keeps changing the reason while an owner
stays unhealthy does not restart the `for:` clock every time. And `max()` keeps the freshest `lastTransitionTime`, so a
former leader pod still exporting a stale series cannot skew the value the way `sum()` would.

`CustomResourceNotReady` and `CustomResourceConditionStuck` are scoped to `Ready` on purpose. Matching `status="False"`
across every condition type would fire forever on negative-polarity conditions such as `Degraded`, where `False` is the
healthy state. To cover your own positive-polarity conditions, widen the `condition` matcher in the rendered file, for
example `condition=~"Ready|CertificateReady"`. The stuck rule keeps `condition` in its `by` clause, so that edit alone
gives one alert per owner and condition. `CustomResourceNotReady` aggregates without `condition`, so add it to the `by`
clause as well if you want a separate alert per condition rather than one per owner. `CustomResourceConditionUnknown` is
not scoped, because `Unknown` is bad whatever the polarity.

`CustomResourceConditionStuck` has no `for:` clause because its expression is itself a duration comparison. A `for:`
clause measures how long the alert has been true, which is bounded by how long the series has been continuously present;
a scrape gap, an operator restart or a ruler restart silently restarts that clock. The stuck rule measures how long the
owner has been in its state according to its own status, so it survives all three and reports the real age. It also
covers `Unknown`, which `CustomResourceNotReady` does not. Tune the `21600` (six hours) to the longest time an owner of
yours can legitimately take to become ready.

Each condition alert carries a `dashboard_url` annotation that deep-links into the CRD Conditions Browser rendered for
the same metric namespace, narrowed to the one owner. The `id` label is `<namespace>/<name>`, or `/<name>` for a
cluster-scoped owner, so the link works without a conditional.

## Dashboards

Both dashboards are Grafana JSON at schema version 41 with a `datasource` variable, auto-refresh off, and the `ocf` tag.
A dashboard link at the top of each lists every dashboard carrying that tag, which is how they cross-reference each
other regardless of the folder or sub-path Grafana serves them from.

The uids are templated with the metric namespace because Grafana upserts dashboards by uid: with fixed uids, importing a
second operator's render into a shared Grafana would overwrite the first. Two operators in one Grafana therefore get
`alpha_ocf_operator` and `beta_ocf_operator`, and each operator's condition alerts link to its own browser.

### OCF Operator

Variables: `job` (from `controller_runtime_reconcile_total`) and `controller` (multi, All), see
[Naming the controller](#naming-the-controller).

| Row               | What it answers                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview          | Is the operator healthy right now? Stat tiles for reconciles per second, reconcile error ratio, p99 reconcile time, p99 queue wait, active against max workers, leader status, and owner counts: Ready, not Ready for more than two minutes (debounced on `lastTransitionTime`, so a rollout in progress does not count), and Unknown. All three owner counts are scoped to the `Ready` condition; unlike `CustomResourceConditionUnknown`, the Unknown tile does not cover other conditions.                                          |
| Reconciliation    | Where does reconcile time go? Reconcile rate by result, error ratio over time, latency at p50, p90 and p99, panics, the age of the longest in-progress reconcile (`workqueue_longest_running_processor_seconds`), and workers.                                                                                                                                                                                                                                                                                                         |
| Workqueue         | Is the operator keeping up? Depth, adds per second, queue wait p99, work duration p99, retries per second, unfinished work.                                                                                                                                                                                                                                                                                                                                                                                                            |
| Managed resources | Is anything being rewritten or failing? Apply rate by operation with the error rate on the same panel, a table of the `updated` rate per resource sorted descending, the not-converging ratio per resource, and the apply error ratio per resource. The not-converging panel plots the `ManagedResourceNotConverging` expression without its floor, so a resource heading for the alert is visible before it fires; the error ratio's denominator falls back to the error rate alone when no success series exists, as the alert does. |
| Conditions        | Which owners are unhealthy? Owners by condition type and status, and an Owners not Ready table (kind, namespace, name, reason, since, status) whose rows link into the CRD Conditions Browser filtered to that owner.                                                                                                                                                                                                                                                                                                                  |
| API client        | Is the API server pushing back? `rest_client_requests_total` rate by method and non-2xx responses by code.                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Process           | Collapsed. CPU, resident memory and goroutines for the `job`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

Reason strings are operator-specific, so colour is driven by `status` (`True` green, `False` red, `Unknown` yellow) and
reasons appear as text.

### CRD Conditions Browser

The browser answers "which owners of this kind are in this state, and since when". Its variables narrow each other from
left to right: `kind` (single choice), then `condition`, `status`, `reason`, `namespace` and `resource_id`, all
multi-select with All, plus ad hoc filters. The Operator Conditions row shows the count of matching conditions and a
table of them by name, namespace, condition, status and reason with a since column; the collapsed Status Counts row
breaks the count down by `False`, `Unknown` and `True`.

`resource_id` is what the alerts drill into: a `CustomResourceNotReady` notification opens the browser with `kind`,
`condition`, `status` and `resource_id` preset, and the Owners not Ready table on the operator dashboard does the same
for the row you click.

Every multi variable answers All with the regular expression `.*` rather than a list of every value. That keeps the
query size constant however many owners exist, and it keeps cluster-scoped owners visible: their condition series carry
no namespace label, and a `=~".*"` matcher on an absent label matches, whereas a list of observed namespaces would not.

### Stale series

The condition gauge is exported by whichever pod recorded it. After a leader change the former leader keeps exporting
its last values until it restarts, so for a while two series describe one owner, and a plain `count()` double counts.
Every condition query in both dashboards joins on the freshest series per owner:

```promql
<selector> and topk by (kind, id) (1, <selector without status and reason matchers>)
```

The metric value is the `lastTransitionTime`, so `topk` keeps the most recently transitioned series for each owner and
drops the stale duplicate. The join carries `kind` because `id` is only `<namespace>/<name>`, and two owners of
different kinds can share it. Queries that span more than one condition type, such as the Owners by condition and status
panel, join on `topk by (kind, id, condition)` instead, so each owner keeps one freshest series per condition rather
than one overall. The browser pins `kind` through its variable, so its queries join on `topk by (id, condition)`. The
alerts get the same protection from `max()`.

## Previewing locally

A clone of the repository can bring up Prometheus and Grafana with simulated operator data behind them, so you can look
at every panel and every alert before installing anything in a cluster. You need docker with the compose plugin and Go.

```bash
make observability-up
```

Grafana serves on `http://localhost:3000` with anonymous admin access and both dashboards provisioned; Prometheus serves
on `http://localhost:9090`, with the alerts on `http://localhost:9090/alerts`. The simulator plays a scripted world in
which every panel is populated and every alert fires within a few minutes (`OperatorLeaderMissing` only with
`make observability-up SIMULATOR_ARGS="-leader=false"`). Stop the simulator with Ctrl-C, then remove the containers:

```bash
make observability-down
```

Maintainers find the stack's internals, the scripted world and the tests that guard the templates in
`observability/README.md` in the repository.
