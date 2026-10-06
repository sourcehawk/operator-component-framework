// Command syncplugin copies the framework docs into the Claude Code plugin skills and fails when a relative link in
// the plugin resolves to no file. Run it from the repository root, through make sync-plugin.
package main

import (
	"fmt"
	"os"

	"github.com/sourcehawk/operator-component-framework/internal/pluginsync"
)

func main() {
	if err := pluginsync.Sync("docs", "plugin/skills", pluginsync.SiteURL, pluginsync.Bundled); err != nil {
		fmt.Fprintln(os.Stderr, "sync-plugin:", err)
		os.Exit(1)
	}

	if err := pluginsync.CheckLinks("plugin"); err != nil {
		fmt.Fprintln(os.Stderr, "sync-plugin: broken links in the plugin:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
