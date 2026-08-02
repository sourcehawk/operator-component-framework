// Command ocf generates code for operators built on the operator component framework.
package main

import "os"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
