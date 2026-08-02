package main

import "github.com/spf13/cobra"

// newRootCommand builds the ocf command tree.
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "ocf",
		Short: "Code generation for the operator component framework",
		Long: "ocf generates code for operators built on the operator component framework.\n\n" +
			"Templates are embedded in the binary, so generated code always matches the\n" +
			"framework version this CLI was built from.",
		SilenceUsage: true,
	}

	root.AddCommand(newScaffoldCommand(), newVersionCommand())

	return root
}
