package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestYAML(t *testing.T) {
	m := goldengen.Manifest{
		Fixtures: []goldengen.FixtureManifest{{
			Name: "default",
			Regimes: []goldengen.RegimeManifest{
				{Representative: "8.8.0", Versions: []string{"8.8.0"}, Firing: []string{"Always", "Pre89"}},
				{Representative: "8.9.0", Versions: []string{"8.9.0"}, Firing: []string{"Always", "Unified89"}},
			},
		}},
	}
	out, err := m.YAML()
	require.NoError(t, err)
	assert.Contains(t, string(out), "name: default")
	assert.Contains(t, string(out), "representative: 8.8.0")
	assert.Contains(t, string(out), "Unified89")
}
