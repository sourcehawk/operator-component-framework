package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sourcehawk/operator-component-framework/internal/scaffold"
	"github.com/spf13/cobra"
)

// newScaffoldCommand builds the scaffold subcommand group.
func newScaffoldCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate framework code from embedded templates",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newScaffoldWrapperCommand())

	return cmd
}

// newScaffoldWrapperCommand builds the wrapper generation subcommand.
func newScaffoldWrapperCommand() *cobra.Command {
	var (
		opts  scaffold.Options
		out   string
		force bool
	)

	variantNames := make([]string, 0, len(scaffold.Variants))
	for _, variant := range scaffold.Variants {
		variantNames = append(variantNames, string(variant))
	}

	cmd := &cobra.Command{
		Use:   "wrapper",
		Short: "Generate a custom-resource wrapper package",
		Long: "Generate a custom-resource wrapper package for a Kubernetes kind the built-in\n" +
			"primitives do not cover. The generated package compiles and its tests pass as\n" +
			"soon as the wrapped type resolves in your module.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.GroupSet = cmd.Flags().Changed("group")

			data, err := opts.Resolve()
			if err != nil {
				return err
			}

			dir := out
			if dir == "" {
				dir = filepath.Join(".", data.Package)
			}

			written, err := scaffold.Generate(data, dir, force)
			if err != nil {
				return err
			}

			return printSummary(cmd, data, dir, written)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.Type, "type", "", "wrapped Go type as <import-path>.<TypeName> (required)")
	flags.StringVar(&opts.Variant, "variant", "",
		fmt.Sprintf("resource category, one of %s (required)", strings.Join(variantNames, ", ")))
	flags.StringVar(&opts.Group, "group", "", `API group, pass "" for core API group types (required)`)
	flags.StringVar(&opts.Version, "version", "", "API version, derived from the import path when it ends in one")
	flags.StringVar(&opts.Kind, "kind", "", "kind used in the identity string (default: the type name)")
	flags.BoolVar(&opts.ClusterScoped, "cluster-scoped", false, "the wrapped kind is cluster-scoped")
	flags.StringVar(&opts.Alias, "alias", "", "import alias for the wrapped type's package (default: derived)")
	flags.StringVar(&opts.Package, "package", "", "generated Go package name (default: the lowercased kind)")
	flags.StringVar(&out, "out", "", "output directory (default: ./<package>)")
	flags.BoolVar(&force, "force", false, "write into a non-empty output directory")

	return cmd
}

// printSummary reports what was generated and what the user has to do next.
func printSummary(cmd *cobra.Command, data scaffold.TemplateData, dir string, written []string) error {
	out := cmd.OutOrStdout()
	display := displayDir(dir)

	if _, err := fmt.Fprintf(out, "Generated %s wrapper package %q in %s:\n", data.Variant, data.Package, display); err != nil {
		return err
	}
	for _, path := range written {
		if _, err := fmt.Fprintf(out, "  %s\n", filepath.Base(path)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(out, "\nNext steps:\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  1. Run go mod tidy so %s resolves in your module.\n", data.ImportPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		out, "  2. Run go test %s/... to verify the generated package.\n", display,
	); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		out, "  3. Replace the scaffolded default handlers in builder.go with %s-specific logic.\n", data.Kind,
	)
	return err
}

// displayDir formats dir for the summary output as a copy-pasteable path
// argument: an absolute dir is printed as-is, and a relative dir keeps or gains a
// leading "./" so it is recognized as a filesystem path rather than a package
// import path.
func displayDir(dir string) string {
	display := filepath.ToSlash(dir)
	if filepath.IsAbs(dir) {
		return display
	}
	if strings.HasPrefix(display, "./") || strings.HasPrefix(display, "../") {
		return display
	}
	return "./" + display
}
