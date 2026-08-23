// Package observability_test checks the dashboard and alert templates: that
// they render to valid JSON/YAML, keep their uids, leave no placeholder behind
// and reference only metric names that actually exist.
package observability_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
)

const (
	lintNamespace = "lint_operator"
	nsLabel       = "exported_namespace"
)

// render mirrors the Makefile's render_template.
func render(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	s := strings.ReplaceAll(string(b), "{{operator_namespace}}", lintNamespace+"_")
	return strings.ReplaceAll(s, "{{namespace_label}}", nsLabel)
}

// knownMetrics is every metric name the templates may reference: the
// framework's own, the condition gauge, and the controller-runtime, workqueue,
// client-go, leader election, process and Go runtime families.
func knownMetrics(t *testing.T) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	descs := make(chan *prometheus.Desc, 64)
	go func() {
		metrics.NewCollectors().Describe(descs)
		ocm.NewOperatorConditionsGauge(lintNamespace).Describe(descs)
		close(descs)
	}()
	fq := regexp.MustCompile(`fqName: "([^"]+)"`)
	for d := range descs {
		m := fq.FindStringSubmatch(d.String())
		require.Len(t, m, 2, d.String())
		known[m[1]] = true
	}
	for _, n := range []string{
		"controller_runtime_reconcile_total", "controller_runtime_reconcile_errors_total",
		"controller_runtime_reconcile_panics_total", "controller_runtime_reconcile_time_seconds",
		"controller_runtime_max_concurrent_reconciles", "controller_runtime_active_workers",
		"workqueue_depth", "workqueue_adds_total", "workqueue_queue_duration_seconds",
		"workqueue_work_duration_seconds", "workqueue_unfinished_work_seconds",
		"workqueue_longest_running_processor_seconds", "workqueue_retries_total",
		"rest_client_requests_total", "leader_election_master_status",
		"process_cpu_seconds_total", "process_resident_memory_bytes", "go_goroutines",
	} {
		known[n] = true
	}
	return known
}

// metricNameRe pulls every identifier that looks like one of the metric
// families we care about out of a PromQL expression, including the
// `_bucket`/`_sum`/`_count` suffixes of histograms.
var metricNameRe = regexp.MustCompile(`\b((?:ocf|controller_runtime|workqueue|rest_client|leader_election|process|go)_[a-z0-9_]+|` + lintNamespace + `_controller_condition)\b`)

// baseName strips the histogram sample suffixes off a series name so that it
// can be looked up as a metric family.
func baseName(n string) string {
	for _, s := range []string{"_bucket", "_sum", "_count"} {
		n = strings.TrimSuffix(n, s)
	}
	return n
}

// exprsFromJSON collects every PromQL-carrying string (panel targets, variable
// queries and definitions) from a decoded dashboard.
func exprsFromJSON(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if k == "expr" || k == "query" || k == "definition" {
				if s, ok := val.(string); ok {
					*out = append(*out, s)
				}
			}
			exprsFromJSON(val, out)
		}
	case []any:
		for _, val := range x {
			exprsFromJSON(val, out)
		}
	}
}

func TestDashboards(t *testing.T) {
	files, err := filepath.Glob("dashboards/*.tpl.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	known := knownMetrics(t)
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			rendered := render(t, f)
			assert.NotContains(t, rendered, "{{operator_namespace}}")
			assert.NotContains(t, rendered, "{{namespace_label}}")
			var d map[string]any
			require.NoError(t, json.Unmarshal([]byte(rendered), &d), "valid JSON")
			want := strings.TrimSuffix(filepath.Base(f), ".tpl.json")
			assert.Equal(t, lintNamespace+"_"+want, d["uid"], "uid is the file name prefixed with the metric namespace")
			assert.Equal(t, "", d["refresh"], "auto refresh is off by default")
			var exprs []string
			exprsFromJSON(d, &exprs)
			require.NotEmpty(t, exprs)
			for _, e := range exprs {
				for _, m := range metricNameRe.FindAllString(e, -1) {
					assert.True(t, known[baseName(m)], "unknown metric %q in %q", m, e)
				}
			}
		})
	}
}

func TestAlerts(t *testing.T) {
	// The glob matches the shared files and, because `.tpl.yaml` also ends in
	// `.yaml`, the templated ones too; render handles both.
	files, err := filepath.Glob("alerts/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	known := knownMetrics(t)
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			rendered := render(t, f)
			assert.NotContains(t, rendered, "{{operator_namespace}}")
			assert.NotContains(t, rendered, "{{namespace_label}}")
			var doc struct {
				Groups []struct {
					Name  string `json:"name"`
					Rules []struct {
						Alert  string            `json:"alert"`
						Expr   string            `json:"expr"`
						Labels map[string]string `json:"labels"`
					} `json:"rules"`
				} `json:"groups"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &doc))
			require.NotEmpty(t, doc.Groups)
			for _, g := range doc.Groups {
				for _, r := range g.Rules {
					assert.Equal(t, map[string]string{"severity": "warning"}, r.Labels, "%s labels", r.Alert)
					for _, m := range metricNameRe.FindAllString(r.Expr, -1) {
						assert.True(t, known[baseName(m)], "unknown metric %q in %s", m, r.Alert)
					}
				}
			}
			// Every alert in the file has a promtool unit test.
			base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(f), ".yaml"), ".tpl")
			tests, err := os.ReadFile(filepath.Join("alerts", "tests", base+"_test.yaml"))
			require.NoError(t, err, "every rule file has a promtool test file")
			for _, g := range doc.Groups {
				for _, r := range g.Rules {
					assert.Contains(t, string(tests), "alertname: "+r.Alert, "%s has a unit test", r.Alert)
				}
			}
		})
	}
}
