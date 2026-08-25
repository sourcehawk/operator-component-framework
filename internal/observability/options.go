// Package observability renders the Grafana dashboards and Prometheus alert
// rules the framework ships for operators built on it. The templates are
// embedded, so a render always matches the framework version the binary was
// built from.
package observability

import (
	"fmt"
	"regexp"
	"strings"
)

// AlertFormat selects the shape of the rendered alert files.
type AlertFormat string

const (
	// AlertFormatPrometheusRule wraps each rule file in a
	// monitoring.coreos.com/v1 PrometheusRule object for the Prometheus
	// Operator.
	AlertFormatPrometheusRule AlertFormat = "prometheusrule"
	// AlertFormatRules writes the plain groups: file that Prometheus loads
	// through rule_files.
	AlertFormatRules AlertFormat = "rules"

	// DefaultNamespaceLabel is the label a scrape through a ServiceMonitor or
	// PodMonitor leaves the owner's namespace under: the target's own namespace
	// label collides with the exported one, which Prometheus renames.
	DefaultNamespaceLabel = "exported_namespace"

	// MaxMetricNamespaceLen caps the metric namespace. It prefixes the
	// dashboard uids, which Grafana limits to maxGrafanaUIDLen characters, and
	// the longest dashboard file name, crd_conditions_browser, is 22
	// characters plus the joining underscore. templates_test.go pins that
	// arithmetic to the dashboard file names.
	MaxMetricNamespaceLen = 17

	// maxGrafanaUIDLen is Grafana's limit on a dashboard uid.
	maxGrafanaUIDLen = 40
)

var (
	// metricNamespacePattern restricts the metric namespace to metric name
	// characters, starting with a letter because it also names the
	// PrometheusRule objects (with _ mapped to -), and to MaxMetricNamespaceLen
	// characters.
	metricNamespacePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,16}$`)
	// labelNamePattern is the Prometheus label name grammar.
	labelNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Options are the inputs of a render, the values of the "ocf observability
// render" flags.
type Options struct {
	// MetricNamespace is the argument the operator passed to
	// ocm.NewOperatorConditionsGauge. It names the condition gauge, the
	// dashboard uids and the condition PrometheusRule. Required.
	MetricNamespace string
	// NamespaceLabel is the label carrying the owner's namespace on condition
	// series, substituted into PromQL as a label name. Use
	// DefaultNamespaceLabel unless the scrape keeps the exported label.
	NamespaceLabel string
	// AlertFormat is the shape of the rendered alert files.
	AlertFormat AlertFormat
	// PrometheusRuleNamespace is metadata.namespace of the PrometheusRule
	// objects. Empty leaves it out.
	PrometheusRuleNamespace string
	// PrometheusRuleLabels are written to metadata.labels of the
	// PrometheusRule objects, in key order.
	PrometheusRuleLabels map[string]string
}

// Validate reports the first option that cannot render. Messages name the
// "ocf observability render" flag the value came from.
func (o Options) Validate() error {
	if o.MetricNamespace == "" {
		return fmt.Errorf("--metric-namespace is required")
	}
	if !metricNamespacePattern.MatchString(o.MetricNamespace) {
		return fmt.Errorf(
			"--metric-namespace %q is not renderable: it must start with a letter, "+
				"contain only letters, digits and underscores, and be at most %d characters, "+
				"because it names the PrometheusRule <namespace>-crd-conditions and the dashboard uid "+
				"<namespace>_crd_conditions_browser must fit Grafana's %d character limit",
			o.MetricNamespace, MaxMetricNamespaceLen, maxGrafanaUIDLen,
		)
	}
	if !labelNamePattern.MatchString(o.NamespaceLabel) {
		return fmt.Errorf(
			"--namespace-label %q is not a Prometheus label name: it must match ^[A-Za-z_][A-Za-z0-9_]*$, "+
				"for example exported_namespace or namespace",
			o.NamespaceLabel,
		)
	}
	switch o.AlertFormat {
	case AlertFormatPrometheusRule, AlertFormatRules:
	default:
		return fmt.Errorf("--alert-format must be %s or %s, got %q",
			AlertFormatPrometheusRule, AlertFormatRules, o.AlertFormat)
	}
	for key := range o.PrometheusRuleLabels {
		if key == "" {
			return fmt.Errorf("--prometheusrule-labels contains an entry with an empty key")
		}
	}
	return nil
}

// ParseLabels parses the comma-separated key=value list of
// --prometheusrule-labels. An empty string yields nil. An entry without a
// key or without "=" is an error naming the entry.
func ParseLabels(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}
	labels := map[string]string{}
	for entry := range strings.SplitSeq(value, ",") {
		key, val, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("--prometheusrule-labels entry %q is not key=value", entry)
		}
		labels[key] = val
	}
	return labels, nil
}
