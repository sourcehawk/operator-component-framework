package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestConfigValidate(t *testing.T) {
	valid := goldengen.Config[*corev1.ConfigMap]{
		Dir:      "td",
		Versions: []string{"1.0.0", "2.0.0"},
		Scheme:   testScheme(),
		Fixtures: []goldengen.Fixture[*corev1.ConfigMap]{{
			Name: "default", Spec: &corev1.ConfigMap{},
			Requires: []goldengen.Expect{{Name: "A"}, {Name: "B", For: "2.0.0"}},
		}},
		Build: func(string, *corev1.ConfigMap) (goldengen.Unit, error) { return nil, nil },
	}
	require.NoError(t, valid.Validate())

	t.Run("for not in versions", func(t *testing.T) {
		bad := valid
		bad.Fixtures = []goldengen.Fixture[*corev1.ConfigMap]{{
			Name: "default", Spec: &corev1.ConfigMap{},
			Requires: []goldengen.Expect{{Name: "B", For: "9.9.9"}},
		}}
		err := bad.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "9.9.9")
	})

	t.Run("empty versions", func(t *testing.T) {
		bad := valid
		bad.Versions = nil
		require.Error(t, bad.Validate())
	})

	t.Run("nil build", func(t *testing.T) {
		bad := valid
		bad.Build = nil
		require.Error(t, bad.Validate())
	})

	t.Run("empty dir", func(t *testing.T) {
		bad := valid
		bad.Dir = ""
		require.Error(t, bad.Validate())
	})

	t.Run("duplicate fixture name", func(t *testing.T) {
		bad := valid
		bad.Fixtures = append([]goldengen.Fixture[*corev1.ConfigMap]{}, valid.Fixtures...)
		bad.Fixtures = append(bad.Fixtures, bad.Fixtures[0])
		require.Error(t, bad.Validate())
	})

	t.Run("empty fixture name", func(t *testing.T) {
		bad := valid
		bad.Fixtures = []goldengen.Fixture[*corev1.ConfigMap]{{Name: "", Spec: &corev1.ConfigMap{}}}
		require.Error(t, bad.Validate())
	})

	t.Run("empty expectation name", func(t *testing.T) {
		bad := valid
		bad.Fixtures = []goldengen.Fixture[*corev1.ConfigMap]{{
			Name: "default", Spec: &corev1.ConfigMap{},
			Forbids: []goldengen.Expect{{Name: ""}},
		}}
		require.Error(t, bad.Validate())
	})
}
