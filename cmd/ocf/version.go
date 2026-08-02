package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// newVersionCommand builds the version subcommand.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the framework version this CLI was built from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), moduleVersion())
			return err
		},
	}
}

// moduleVersion returns the module version the binary was built from.
func moduleVersion() string {
	return versionFrom(debug.ReadBuildInfo())
}

// versionFrom reports the main module version recorded in build info, or
// "unknown" when the binary carries no version stamp.
func versionFrom(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil || info.Main.Version == "" {
		return "unknown"
	}
	return info.Main.Version
}
