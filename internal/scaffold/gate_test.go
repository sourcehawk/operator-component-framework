//go:build scaffold

package scaffold_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcehawk/operator-component-framework/internal/scaffold"
	"github.com/stretchr/testify/assert"
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

// gateModuleName is the module declared in the temporary go.mod written by
// writeGateModule. Generated package import paths are this name joined with
// the package's directory relative to the module root.
const gateModuleName = "ocfscaffoldgate"

// TestScaffoldedWrappersCompileAndPass generates every variant into a temporary
// module that replaces the framework with this checkout, then runs the generated
// tests inside it.
//
// Passing the temp module's overall exit code is not enough: a Go test binary
// exits 0 both for a package with zero test files ("[no test files]") and for a
// test file with zero Test functions ("[no tests to run]"). Either would let a
// template regression that guts or omits builder_test.go sail through silently.
// So this test also asserts, per generated package, that at least one test
// actually reported a pass, using `go test -json` output rather than scraping
// text markers.
func TestScaffoldedWrappersCompileAndPass(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	moduleDir := t.TempDir()
	writeGateModule(t, repoRoot, moduleDir)

	expectedPackages := make([]string, 0, len(gateCases()))
	for _, opts := range gateCases() {
		data, err := opts.Resolve()
		require.NoError(t, err)

		outDir := filepath.Join(moduleDir, data.Package)
		written, err := scaffold.Generate(data, outDir, false)
		require.NoError(t, err, "generate %s", data.Package)

		expectedFiles := make([]string, 0, len(scaffold.GeneratedFiles))
		for _, name := range scaffold.GeneratedFiles {
			expectedFiles = append(expectedFiles, filepath.Join(outDir, name))
		}
		assert.Equal(t, expectedFiles, written, "generate %s did not produce the expected file set", data.Package)

		expectedPackages = append(expectedPackages, gateModuleName+"/"+data.Package)
	}

	events, out, err := runGoTestJSON(t, moduleDir)
	require.NoError(t, err, "go test in scaffolded module failed:\n%s", out)

	assertEachPackageRanTests(t, events, expectedPackages)
}

// testEvent is the subset of a `go test -json` event this gate needs. The full
// event carries Time and Elapsed fields too, which are unused here.
type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

// assertEachPackageRanTests fails the test if any expected package reported
// zero passing tests. A package with no test files, or a test file with no
// Test functions, both produce zero "pass" events for that package, so this
// catches template regressions the exit code alone would miss.
func assertEachPackageRanTests(t *testing.T, events []testEvent, expectedPackages []string) {
	t.Helper()

	passed := make(map[string]int)
	for _, ev := range events {
		if ev.Action == "pass" && ev.Test != "" {
			passed[ev.Package]++
		}
	}

	for _, pkg := range expectedPackages {
		assert.Positive(t, passed[pkg], "package %q reported no passing tests; its test file may be missing or empty", pkg)
	}
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

	goMod := "module " + gateModuleName + "\n\n" +
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

	cmd := exec.CommandContext(t.Context(), "go", args...)
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

// runGoTestJSON runs `go test -json ./...` in dir. It returns the decoded test
// events from stdout, plus the combined stdout and stderr for use in failure
// messages, so that a failing run still has readable diagnostics.
//
// Stdout and stderr are captured separately, not combined, because `go test
// -json` writes newline-delimited JSON only to stdout; interleaving it with
// stderr on the same buffer could split a JSON line mid-write and break decoding.
func runGoTestJSON(t *testing.T, dir string) ([]testEvent, string, error) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "test", "-json", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	rawStdout := stdout.String()

	var events []testEvent
	decoder := json.NewDecoder(strings.NewReader(rawStdout))
	for {
		var ev testEvent
		if decodeErr := decoder.Decode(&ev); decodeErr != nil {
			break
		}
		events = append(events, ev)
	}

	return events, rawStdout + stderr.String(), runErr
}
