package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// newCM returns a fresh empty ConfigMap for LoadMatrix to unmarshal each fixture
// spec into.
func newCM() *corev1.ConfigMap { return &corev1.ConfigMap{} }

// cmBuildFn materializes a ConfigMap fixture spec into a Unit at a version. The
// version does not influence the build here; it exists to satisfy the Build
// signature and to prove the produced Config is runnable.
func cmBuildFn(_ string, spec *corev1.ConfigMap) (goldengen.Unit, error) {
	res, err := configmap.NewBuilder(spec).Build()
	if err != nil {
		return nil, err
	}
	return goldengen.Resource(res, testScheme()), nil
}

func TestLoadMatrixInline(t *testing.T) {
	cfg, err := goldengen.LoadMatrix("testdata/matrix_inline.yaml", newCM, cmBuildFn, testScheme())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	assert.Equal(t, "testdata/goldens", cfg.Dir)
	assert.Equal(t, []string{"1.0.0", "2.0.0"}, cfg.Versions)
	require.Len(t, cfg.Fixtures, 1)

	f := cfg.Fixtures[0]
	assert.Equal(t, "default", f.Name)
	// The inline spec must round-trip into the typed T, including nested fields.
	require.NotNil(t, f.Spec)
	assert.Equal(t, "from-inline", f.Spec.Name)
	assert.Equal(t, "default", f.Spec.Namespace)
	assert.Equal(t, "value", f.Spec.Data["key"])

	require.Len(t, f.Requires, 1)
	assert.Equal(t, "A", f.Requires[0].Name)
	assert.Equal(t, "1.0.0", f.Requires[0].For)
	require.Len(t, f.Forbids, 1)
	assert.Equal(t, "B", f.Forbids[0].Name)
}

func TestLoadMatrixSpecFile(t *testing.T) {
	cfg, err := goldengen.LoadMatrix("testdata/matrix_specfile.yaml", newCM, cmBuildFn, testScheme())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	require.Len(t, cfg.Fixtures, 1)
	// specFile is resolved relative to the matrix file's directory.
	assert.Equal(t, "from-file", cfg.Fixtures[0].Spec.Name)
	assert.Equal(t, "default", cfg.Fixtures[0].Spec.Namespace)
}

func TestLoadMatrixRunnable(t *testing.T) {
	cfg, err := goldengen.LoadMatrix("testdata/matrix_inline.yaml", newCM, cmBuildFn, testScheme())
	require.NoError(t, err)

	// The produced Config must be runnable: Build yields a Unit that renders.
	unit, err := cfg.Build("1.0.0", cfg.Fixtures[0].Spec)
	require.NoError(t, err)
	out, err := unit.RenderYAML()
	require.NoError(t, err)
	assert.Contains(t, string(out), "from-inline")
}

func TestLoadMatrixErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "both spec and specFile set", path: "testdata/matrix_both.yaml"},
		{name: "neither spec nor specFile set", path: "testdata/matrix_neither.yaml"},
		{name: "For not in versions", path: "testdata/matrix_badfor.yaml"},
		{name: "missing matrix file", path: "testdata/does_not_exist.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := goldengen.LoadMatrix(tc.path, newCM, cmBuildFn, testScheme())
			require.Error(t, err)
		})
	}
}
