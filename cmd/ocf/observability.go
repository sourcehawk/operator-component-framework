package main

import (
	"fmt"
	"path/filepath"

	"github.com/sourcehawk/operator-component-framework/internal/observability"
	"github.com/spf13/cobra"
)

// newObservabilityCommand builds the observability subcommand group.
func newObservabilityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observability",
		Short: "Render the Grafana dashboards and Prometheus alert rules",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newObservabilityRenderCommand())

	return cmd
}

// newObservabilityRenderCommand builds the render subcommand.
func newObservabilityRenderCommand() *cobra.Command {
	var (
		opts        observability.Options
		alertFormat string
		labels      string
		out         string
	)

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render the dashboards and alert rules for an operator",
		Long: "Render the Grafana dashboards and Prometheus alert rules the framework ships,\n" +
			"keyed on the metric namespace of your operator (the argument you passed to\n" +
			"ocm.NewOperatorConditionsGauge), into <out>/dashboards and <out>/alerts.\n\n" +
			"Rendered files are regenerated output: every .json in <out>/dashboards and\n" +
			"every .yaml in <out>/alerts is replaced on each run.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.AlertFormat = observability.AlertFormat(alertFormat)

			parsed, err := observability.ParseLabels(labels)
			if err != nil {
				return err
			}
			opts.PrometheusRuleLabels = parsed

			written, err := observability.Render(opts, out)
			if err != nil {
				return err
			}

			return printRenderSummary(cmd, opts, out, written)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.MetricNamespace, "metric-namespace", "",
		"metric namespace of the operator, the argument to ocm.NewOperatorConditionsGauge (required)")
	flags.StringVar(&opts.NamespaceLabel, "namespace-label", observability.DefaultNamespaceLabel,
		"label carrying the owner's namespace on condition series")
	flags.StringVar(&alertFormat, "alert-format", string(observability.AlertFormatPrometheusRule),
		fmt.Sprintf("shape of the alert files, %s or %s",
			observability.AlertFormatPrometheusRule, observability.AlertFormatRules))
	flags.StringVar(&opts.PrometheusRuleNamespace, "prometheusrule-namespace", "",
		"metadata.namespace of the PrometheusRule objects")
	flags.StringVar(&labels, "prometheusrule-labels", "",
		"comma-separated key=value labels for metadata.labels of the PrometheusRule objects")
	flags.StringVar(&out, "out", "./observability", "output directory")

	return cmd
}

// printRenderSummary reports what was rendered and where.
func printRenderSummary(cmd *cobra.Command, opts observability.Options, dir string, written []string) error {
	w := cmd.OutOrStdout()

	if _, err := fmt.Fprintf(w, "Rendered observability artifacts for metric namespace %q in %s:\n",
		opts.MetricNamespace, displayDir(dir)); err != nil {
		return err
	}
	for _, path := range written {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		if _, err := fmt.Fprintf(w, "  %s\n", filepath.ToSlash(rel)); err != nil {
			return err
		}
	}
	return nil
}
