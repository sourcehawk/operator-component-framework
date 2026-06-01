package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRegimes(t *testing.T) {
	versions := []string{"1.0.0", "1.1.0", "2.0.0"}
	firing := map[string][]string{
		"1.0.0": {"A"},
		"1.1.0": {"A"},      // same regime as 1.0.0
		"2.0.0": {"A", "B"}, // new regime
	}
	regimes := goldengen.ClassifyRegimes(versions, firing)
	require.Len(t, regimes, 2)
	assert.Equal(t, "1.0.0", regimes[0].Representative)
	assert.Equal(t, []string{"1.0.0", "1.1.0"}, regimes[0].Versions)
	assert.Equal(t, []string{"A"}, regimes[0].Firing)
	assert.Equal(t, "2.0.0", regimes[1].Representative)
	assert.Equal(t, []string{"2.0.0"}, regimes[1].Versions)
	assert.Equal(t, []string{"A", "B"}, regimes[1].Firing)
}

func TestClassifyOrderIndependentSignature(t *testing.T) {
	// firing-set order must not split a regime.
	versions := []string{"1.0.0", "2.0.0"}
	firing := map[string][]string{"1.0.0": {"A", "B"}, "2.0.0": {"B", "A"}}
	regimes := goldengen.ClassifyRegimes(versions, firing)
	require.Len(t, regimes, 1)
	assert.Equal(t, []string{"A", "B"}, regimes[0].Firing)
}

func TestClassifyEmptyFiringSet(t *testing.T) {
	versions := []string{"1.0.0", "2.0.0"}
	firing := map[string][]string{"1.0.0": {}, "2.0.0": nil}
	regimes := goldengen.ClassifyRegimes(versions, firing)
	require.Len(t, regimes, 1)
	assert.Equal(t, "1.0.0", regimes[0].Representative)
	assert.Empty(t, regimes[0].Firing)
}
