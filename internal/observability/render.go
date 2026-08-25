package observability

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/dashboards/*.tpl.json templates/alerts/*.yaml templates/alerts/tests/*.yaml
var templates embed.FS

const (
	dashboardsDir = "templates/dashboards"
	alertsDir     = "templates/alerts"
	alertTestsDir = "templates/alerts/tests"

	// The two placeholders shared with go-crd-condition-metrics. Substitution
	// is literal: the templates also carry Grafana and Alertmanager {{ }}
	// templating and YAML comments that must survive verbatim.
	operatorNamespacePlaceholder = "{{operator_namespace}}"
	namespaceLabelPlaceholder    = "{{namespace_label}}"

	// sharedRulePrefix names the PrometheusRule objects rendered from the rule
	// files without a placeholder, which every operator renders identically
	// and a cluster installs once.
	sharedRulePrefix = "ocf"
)

// Render writes the dashboards and alert rules for opts into outDir as
// <outDir>/dashboards/<name>.json and <outDir>/alerts/<name>.yaml, and returns
// the written paths. Rendered files are regenerated output: every .json in
// the dashboards directory and every .yaml in the alerts directory is removed
// first, so a file a framework upgrade renamed or dropped does not linger.
// opts is validated before anything is written.
func Render(opts Options, outDir string) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	dashboards, err := renderDashboards(opts)
	if err != nil {
		return nil, err
	}
	alerts, err := renderAlerts(opts)
	if err != nil {
		return nil, err
	}

	var written []string
	for _, dir := range []struct {
		name  string
		ext   string
		files map[string][]byte
	}{
		{name: "dashboards", ext: ".json", files: dashboards},
		{name: "alerts", ext: ".yaml", files: alerts},
	} {
		paths, err := writeDir(filepath.Join(outDir, dir.name), dir.ext, dir.files)
		if err != nil {
			return nil, err
		}
		written = append(written, paths...)
	}
	return written, nil
}

// writeDir replaces every *<ext> file in dir with files, keyed by base name.
func writeDir(dir, ext string, files map[string][]byte) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	stale, err := filepath.Glob(filepath.Join(dir, "*"+ext))
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	for _, p := range stale {
		if err := os.Remove(p); err != nil {
			return nil, fmt.Errorf("remove stale %s: %w", p, err)
		}
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	written := make([]string, 0, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", p, err)
		}
		written = append(written, p)
	}
	return written, nil
}

// substitute replaces the two placeholders in a template.
func substitute(tmpl []byte, opts Options) []byte {
	out := bytes.ReplaceAll(tmpl, []byte(operatorNamespacePlaceholder), []byte(opts.MetricNamespace+"_"))
	return bytes.ReplaceAll(out, []byte(namespaceLabelPlaceholder), []byte(opts.NamespaceLabel))
}

// renderDashboards renders every dashboard template, keyed by output file name.
func renderDashboards(opts Options) (map[string][]byte, error) {
	files, err := fs.Glob(templates, path.Join(dashboardsDir, "*.tpl.json"))
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(files))
	for _, file := range files {
		tmpl, err := templates.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", file, err)
		}
		name := strings.TrimSuffix(path.Base(file), ".tpl.json") + ".json"
		out[name] = substitute(tmpl, opts)
	}
	return out, nil
}

// renderAlerts renders every rule file, keyed by output file name. A
// .tpl.yaml file is per operator and its PrometheusRule is named after the
// metric namespace; a plain .yaml file is shared and named with the ocf
// prefix.
func renderAlerts(opts Options) (map[string][]byte, error) {
	files, err := fs.Glob(templates, path.Join(alertsDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(files))
	for _, file := range files {
		tmpl, err := templates.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", file, err)
		}
		base := path.Base(file)
		var name, prefix string
		if strings.HasSuffix(base, ".tpl.yaml") {
			name = strings.TrimSuffix(base, ".tpl.yaml")
			prefix = opts.MetricNamespace
		} else {
			name = strings.TrimSuffix(base, ".yaml")
			prefix = sharedRulePrefix
		}
		body := substitute(tmpl, opts)
		if opts.AlertFormat == AlertFormatPrometheusRule {
			body = wrapPrometheusRule(ruleName(prefix, name), opts, body)
		}
		out[name+".yaml"] = body
	}
	return out, nil
}

// ruleName joins prefix and file name into a PrometheusRule name: lower-cased,
// with _ and : folded to -, so it is a valid DNS subdomain name.
func ruleName(prefix, name string) string {
	return strings.NewReplacer("_", "-", ":", "-").Replace(strings.ToLower(prefix + "-" + name))
}

// wrapPrometheusRule emits a monitoring.coreos.com/v1 PrometheusRule whose
// spec is the rule file indented by two spaces, with trailing whitespace
// trimmed on every body line so blank lines stay blank.
func wrapPrometheusRule(name string, opts Options, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("apiVersion: monitoring.coreos.com/v1\n")
	buf.WriteString("kind: PrometheusRule\n")
	buf.WriteString("metadata:\n")
	buf.WriteString("  name: " + name + "\n")
	if opts.PrometheusRuleNamespace != "" {
		buf.WriteString("  namespace: " + opts.PrometheusRuleNamespace + "\n")
	}
	if len(opts.PrometheusRuleLabels) > 0 {
		keys := make([]string, 0, len(opts.PrometheusRuleLabels))
		for k := range opts.PrometheusRuleLabels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteString("  labels:\n")
		for _, k := range keys {
			buf.WriteString("    " + k + ": " + strconv.Quote(opts.PrometheusRuleLabels[k]) + "\n")
		}
	}
	buf.WriteString("spec:\n")
	for _, line := range bytes.SplitAfter(body, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		trimmed := bytes.TrimRight(line, " \t\r\n")
		if len(trimmed) > 0 {
			buf.WriteString("  ")
			buf.Write(trimmed)
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}
