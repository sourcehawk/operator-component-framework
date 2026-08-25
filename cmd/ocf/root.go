package main

import "github.com/spf13/cobra"

// newRootCommand builds the ocf command tree.
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "ocf",
		Short: "Code generation and observability artifacts for the operator component framework",
		Long: "ocf generates code and renders the Grafana dashboards and Prometheus alert rules\n" +
			"for operators built on the operator component framework.\n\n" +
			"Templates are embedded in the binary, so generated code and rendered artifacts\n" +
			"always match the framework version this CLI was built from.",
		SilenceUsage: true,
	}

	root.AddCommand(newScaffoldCommand(), newObservabilityCommand(), newVersionCommand())

	return root
}
