//go:build scaffold

package scaffold_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcehawk/operator-component-framework/internal/scaffold"
	"github.com/stretchr/testify/require"
)

// gateCases scaffolds one wrapper per variant against a real Kubernetes type.
func gateCases() []scaffold.Options {
	return []scaffold.Options{
		{Type: "k8s.io/api/core/v1.ConfigMap", Variant: "static", Group: "", GroupSet: true},
		{Type: "k8s.io/api/apps/v1.Deployment", Variant: "workload", Group: "apps", GroupSet: true},
		{Type: "k8s.io/api/batch/v1.Job", Variant: "task", Group: "batch", GroupSet: true},
		{
			Type: "k8s.io/api/networking/v1.Ingress", Variant: "integration",
			Group: "networking.k8s.io", GroupSet: true,
		},
		{
			Type: "k8s.io/api/rbac/v1.ClusterRole", Variant: "static",
			Group: "rbac.authorization.k8s.io", GroupSet: true, ClusterScoped: true,
		},
	}
}

// TestScaffoldedWrappersCompileAndPass generates every variant into a temporary
// module that replaces the framework with this checkout, then runs the generated
// tests inside it.
func TestScaffoldedWrappersCompileAndPass(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	moduleDir := t.TempDir()
	writeGateModule(t, repoRoot, moduleDir)

	for _, opts := range gateCases() {
		data, err := opts.Resolve()
		require.NoError(t, err)

		_, err = scaffold.Generate(data, filepath.Join(moduleDir, data.Package), false)
		require.NoError(t, err, "generate %s", data.Package)
	}

	out, err := runGo(t, moduleDir, "test", "./...")
	require.NoError(t, err, "go test in scaffolded module failed:\n%s", out)
}

// writeGateModule writes a go.mod that replaces the framework with the local
// checkout, pinning every direct dependency to the version this module uses.
func writeGateModule(t *testing.T, repoRoot, moduleDir string) {
	t.Helper()

	goVersion := strings.TrimSpace(mustRunGo(t, repoRoot, "list", "-m", "-f", "{{.GoVersion}}"))

	var requires strings.Builder
	for _, path := range []string{
		"k8s.io/api",
		"k8s.io/apimachinery",
		"sigs.k8s.io/controller-runtime",
		"github.com/stretchr/testify",
	} {
		version := strings.TrimSpace(mustRunGo(t, repoRoot, "list", "-m", "-f", "{{.Version}}", path))
		requires.WriteString("\t" + path + " " + version + "\n")
	}

	goMod := "module ocfscaffoldgate\n\n" +
		"go " + goVersion + "\n\n" +
		"require (\n" +
		"\tgithub.com/sourcehawk/operator-component-framework v0.0.0\n" +
		requires.String() +
		")\n\n" +
		"replace github.com/sourcehawk/operator-component-framework => " + repoRoot + "\n"

	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o600))

	goSum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "go.sum"), goSum, 0o600))
}

// runGo runs the go tool in dir and returns its combined output.
func runGo(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")

	out, err := cmd.CombinedOutput()

	return string(out), err
}

// mustRunGo runs the go tool in dir and fails the test if it errors.
func mustRunGo(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := runGo(t, dir, args...)
	require.NoError(t, err, "go %s failed:\n%s", strings.Join(args, " "), out)

	return out
}
