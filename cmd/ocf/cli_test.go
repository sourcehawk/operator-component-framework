package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCommand executes the root command with args, capturing stdout and stderr.
func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

func TestRootCommandListsSubcommands(t *testing.T) {
	t.Parallel()

	out, err := runCommand(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "scaffold")
	assert.Contains(t, out, "version")
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	out, err := runCommand(t, "version")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestScaffoldWrapperGeneratesPackage(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "certificate")

	out, err := runCommand(t,
		"scaffold", "wrapper",
		"--type", "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1.Certificate",
		"--variant", "integration",
		"--group", "cert-manager.io",
		"--out", dir,
	)
	require.NoError(t, err)

	for _, name := range []string{"builder.go", "builder_test.go", "mutator.go", "resource.go"} {
		assert.FileExists(t, filepath.Join(dir, name))
	}

	assert.Contains(t, out, dir)
	assert.Contains(t, out, "go mod tidy")
	assert.Contains(t, out, "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1")

	// dir is absolute (rooted at t.TempDir()), so the printed go test invocation
	// must use it as-is, not glued onto a "./" prefix.
	assert.Contains(t, out, "  2. Run go test "+dir+"/... to verify the generated package.\n")
}

func TestScaffoldWrapperDefaultsOutToPackageDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runCommand(t,
		"scaffold", "wrapper",
		"--type", "k8s.io/api/core/v1.ConfigMap",
		"--variant", "static",
		"--group", "",
	)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "configmap", "builder.go"))

	// The default output directory is relative, so the printed go test
	// invocation must be a copy-pasteable relative path, prefixed with "./".
	assert.Contains(t, out, "  2. Run go test ./configmap/... to verify the generated package.\n")
}

func TestScaffoldWrapperRefusesNonEmptyDirectoryWithoutForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package keep\n"), 0o644))

	_, err := runCommand(t,
		"scaffold", "wrapper",
		"--type", "k8s.io/api/core/v1.ConfigMap",
		"--variant", "static",
		"--group", "",
		"--out", dir,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not empty")
}

func TestScaffoldWrapperForceWritesIntoNonEmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package keep\n"), 0o644))

	_, err := runCommand(t,
		"scaffold", "wrapper",
		"--type", "k8s.io/api/core/v1.ConfigMap",
		"--variant", "static",
		"--group", "",
		"--out", dir,
		"--force",
	)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "builder.go"))
	assert.FileExists(t, filepath.Join(dir, "keep.go"))
}

func TestScaffoldWrapperFlagErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{
			name:        "missing type",
			args:        []string{"scaffold", "wrapper", "--variant", "static", "--group", ""},
			expectedErr: "--type is required",
		},
		{
			name:        "missing variant",
			args:        []string{"scaffold", "wrapper", "--type", "k8s.io/api/core/v1.ConfigMap", "--group", ""},
			expectedErr: "--variant is required",
		},
		{
			name: "unknown variant",
			args: []string{
				"scaffold", "wrapper",
				"--type", "k8s.io/api/core/v1.ConfigMap", "--variant", "daemon", "--group", "",
			},
			expectedErr: "--variant must be one of static, workload, task, integration",
		},
		{
			name:        "group not provided",
			args:        []string{"scaffold", "wrapper", "--type", "k8s.io/api/core/v1.ConfigMap", "--variant", "static"},
			expectedErr: "--group is required",
		},
		{
			name: "version not derivable",
			args: []string{
				"scaffold", "wrapper",
				"--type", "example.io/apis/messaging.Queue", "--variant", "static", "--group", "messaging.example.io",
			},
			expectedErr: "--version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := make([]string, 0, len(tt.args)+2)
			args = append(args, tt.args...)
			args = append(args, "--out", filepath.Join(t.TempDir(), "pkg"))
			_, err := runCommand(t, args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestVersionFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     *debug.BuildInfo
		ok       bool
		expected string
	}{
		{
			name:     "build info unavailable",
			info:     nil,
			ok:       false,
			expected: "unknown",
		},
		{
			name:     "empty main version",
			info:     &debug.BuildInfo{Main: debug.Module{Version: ""}},
			ok:       true,
			expected: "unknown",
		},
		{
			name:     "tagged version",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}},
			ok:       true,
			expected: "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, versionFrom(tt.info, tt.ok))
		})
	}
}
