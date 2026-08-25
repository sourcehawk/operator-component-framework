package observability_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/sourcehawk/operator-component-framework/internal/observability"
)

var (
	expectedDashboards = []string{"crd_conditions_browser.json", "ocf_operator.json"}
	expectedAlerts     = []string{"controller_runtime.yaml", "crd_conditions.yaml", "managed_resources.yaml"}
)

// prometheusRule is the part of a rendered PrometheusRule the tests look at.
type prometheusRule struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Groups []struct {
			Name  string `json:"name"`
			Rules []struct {
				Alert string `json:"alert"`
			} `json:"rules"`
		} `json:"groups"`
	} `json:"spec"`
}

func readRule(t *testing.T, path string) (string, prometheusRule) {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var rule prometheusRule
	require.NoError(t, yaml.Unmarshal(b, &rule), "%s is valid YAML", path)
	return string(b), rule
}

func assertNoPlaceholders(t *testing.T, content, path string) {
	t.Helper()
	assert.NotContains(t, content, "{{operator_namespace}}", path)
	assert.NotContains(t, content, "{{namespace_label}}", path)
}

func TestRenderWritesDashboardsAndPrometheusRules(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	opts := validOptions()
	opts.PrometheusRuleNamespace = "monitoring"
	opts.PrometheusRuleLabels = map[string]string{"team": "platform", "release": "kps"}

	written, err := observability.Render(opts, out)
	require.NoError(t, err)

	var expected []string
	for _, name := range expectedDashboards {
		expected = append(expected, filepath.Join(out, "dashboards", name))
	}
	for _, name := range expectedAlerts {
		expected = append(expected, filepath.Join(out, "alerts", name))
	}
	assert.Equal(t, expected, written, "dashboards then alerts, each sorted by name")

	for _, name := range expectedDashboards {
		b, err := os.ReadFile(filepath.Join(out, "dashboards", name))
		require.NoError(t, err)
		assertNoPlaceholders(t, string(b), name)
		var dashboard map[string]any
		require.NoError(t, json.Unmarshal(b, &dashboard), "%s is valid JSON", name)
		assert.Equal(t, "demo_"+strings.TrimSuffix(name, ".json"), dashboard["uid"])
	}

	names := map[string]string{
		"controller_runtime.yaml": "ocf-controller-runtime",
		"crd_conditions.yaml":     "demo-crd-conditions",
		"managed_resources.yaml":  "ocf-managed-resources",
	}
	for file, ruleName := range names {
		path := filepath.Join(out, "alerts", file)
		content, rule := readRule(t, path)
		assertNoPlaceholders(t, content, file)
		assert.Equal(t, "monitoring.coreos.com/v1", rule.APIVersion)
		assert.Equal(t, "PrometheusRule", rule.Kind)
		assert.Equal(t, ruleName, rule.Metadata.Name)
		assert.Equal(t, "monitoring", rule.Metadata.Namespace)
		assert.Equal(t, map[string]string{"release": "kps", "team": "platform"}, rule.Metadata.Labels)
		require.NotEmpty(t, rule.Spec.Groups, "%s carries the rule groups under spec", file)
		assert.NotEmpty(t, rule.Spec.Groups[0].Rules)
		assert.True(t, strings.HasPrefix(content,
			"apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: "+ruleName+
				"\n  namespace: monitoring\n  labels:\n    release: \"kps\"\n    team: \"platform\"\nspec:\n"),
			"%s header is written in a fixed order, labels sorted by key", file)
		assert.NotRegexp(t, `(?m)[ \t]+$`, content, "%s has no trailing whitespace", file)
	}

	// The condition gauge is named after the metric namespace and the
	// namespace label is substituted into the PromQL.
	content, _ := readRule(t, filepath.Join(out, "alerts", "crd_conditions.yaml"))
	assert.Contains(t, content, "demo_controller_condition{")
	assert.Contains(t, content, "exported_namespace")
}

func TestRenderPrometheusRuleOmitsUnsetMetadata(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	_, err := observability.Render(validOptions(), out)
	require.NoError(t, err)

	content, rule := readRule(t, filepath.Join(out, "alerts", "crd_conditions.yaml"))
	assert.Empty(t, rule.Metadata.Namespace)
	assert.Empty(t, rule.Metadata.Labels)
	assert.True(t, strings.HasPrefix(content,
		"apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: demo-crd-conditions\nspec:\n"))
}

func TestRenderRuleNameFoldsCaseAndSeparators(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	opts := validOptions()
	opts.MetricNamespace = "My_Operator"
	_, err := observability.Render(opts, out)
	require.NoError(t, err)

	_, rule := readRule(t, filepath.Join(out, "alerts", "crd_conditions.yaml"))
	assert.Equal(t, "my-operator-crd-conditions", rule.Metadata.Name)
}

func TestRenderRulesFormatWritesPlainGroups(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	opts := validOptions()
	opts.AlertFormat = observability.AlertFormatRules
	opts.NamespaceLabel = "namespace"
	_, err := observability.Render(opts, out)
	require.NoError(t, err)

	for _, name := range expectedAlerts {
		b, err := os.ReadFile(filepath.Join(out, "alerts", name))
		require.NoError(t, err)
		content := string(b)
		assertNoPlaceholders(t, content, name)
		assert.NotContains(t, content, "kind: PrometheusRule")
		assert.True(t, strings.HasPrefix(content, "#") || strings.HasPrefix(content, "groups:"),
			"%s starts with the rule file itself", name)
		var doc struct {
			Groups []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		require.NoError(t, yaml.Unmarshal(b, &doc))
		assert.NotEmpty(t, doc.Groups, "%s is a plain groups: file", name)
	}
	content, err := os.ReadFile(filepath.Join(out, "alerts", "crd_conditions.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "by (job, controller, kind, name, namespace)")
	assert.NotContains(t, string(content), "exported_namespace")
}

func TestRenderReplacesPreviousRender(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(out, "dashboards"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(out, "alerts"), 0o755))
	stale := []string{
		filepath.Join(out, "dashboards", "renamed.json"),
		filepath.Join(out, "alerts", "dropped.yaml"),
	}
	for _, p := range stale {
		require.NoError(t, os.WriteFile(p, []byte("stale"), 0o644))
	}
	keep := filepath.Join(out, "alerts", "notes.txt")
	require.NoError(t, os.WriteFile(keep, []byte("mine"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(out, "dashboards", "ocf_operator.json"), []byte("old"), 0o644))

	_, err := observability.Render(validOptions(), out)
	require.NoError(t, err)

	for _, p := range stale {
		assert.NoFileExists(t, p, "a file from an earlier render that no longer exists is removed")
	}
	assert.FileExists(t, keep, "files of other types are left alone")
	b, err := os.ReadFile(filepath.Join(out, "dashboards", "ocf_operator.json"))
	require.NoError(t, err)
	assert.NotEqual(t, "old", string(b), "an existing render is overwritten")
}

func TestRenderRejectsInvalidOptionsBeforeWriting(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "observability")
	opts := validOptions()
	opts.MetricNamespace = "my-operator"

	written, err := observability.Render(opts, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--metric-namespace")
	assert.Nil(t, written)
	assert.NoDirExists(t, out)
}
