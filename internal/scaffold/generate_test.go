package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func staticData() TemplateData {
	return TemplateData{
		Package: "configmap", ImportPath: "k8s.io/api/core/v1", ImportAlias: "corev1",
		TypeName: "ConfigMap", Version: "v1", Kind: "ConfigMap", Variant: VariantStatic,
	}
}

func TestGenerateWritesAllFiles(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "configmap")

	written, err := Generate(staticData(), dir, false)
	require.NoError(t, err)
	require.Len(t, written, len(GeneratedFiles))

	for _, name := range GeneratedFiles {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	}
}

func TestGenerateRefusesNonEmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.go"), []byte("package x\n"), 0o644))

	_, err := Generate(staticData(), dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not empty")
	assert.Contains(t, err.Error(), "--force")
}

func TestGenerateAllowsEmptyExistingDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := Generate(staticData(), dir, false)
	require.NoError(t, err)
}

func TestGenerateForceOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	builderPath := filepath.Join(dir, "builder.go")
	require.NoError(t, os.WriteFile(builderPath, []byte("package stale\n"), 0o644))

	_, err := Generate(staticData(), dir, true)
	require.NoError(t, err)

	content, err := os.ReadFile(builderPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "package configmap")
}

func TestGenerateRejectsFilePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	_, err := Generate(staticData(), path, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a directory")
}
