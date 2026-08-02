package main

import "github.com/spf13/cobra"

// newScaffoldCommand builds the scaffold subcommand group.
func newScaffoldCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scaffold",
		Short: "Generate framework code from embedded templates",
		Args:  cobra.NoArgs,
	}
}
