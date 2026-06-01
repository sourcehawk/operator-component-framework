package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckGating(t *testing.T) {
	firing := map[string][]string{ // version -> firing-set
		"8.8.0": {"Always", "Pre89"},
		"8.9.0": {"Always", "Unified89"},
	}
	f := goldengen.Fixture[int]{
		Name: "default",
		Requires: []goldengen.Expect{
			{Name: "Always"},                  // existential: fires somewhere
			{Name: "Unified89", For: "8.9.0"}, // pinned
			{Name: "Pre89", For: "8.8.0"},
		},
		Forbids: []goldengen.Expect{
			{Name: "Unified89", For: "8.8.0"}, // not before boundary
			{Name: "Pre89", For: "8.9.0"},     // not after boundary
		},
	}
	require.NoError(t, goldengen.CheckGating(f, []string{"8.8.0", "8.9.0"}, firing))
}

func TestCheckGatingFailures(t *testing.T) {
	firing := map[string][]string{"8.9.0": {"Always"}}
	versions := []string{"8.9.0"}

	t.Run("required existential missing", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Requires: []goldengen.Expect{{Name: "Ghost"}}}
		err := goldengen.CheckGating(f, versions, firing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Ghost")
	})
	t.Run("required pinned missing", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Requires: []goldengen.Expect{{Name: "Ghost", For: "8.9.0"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
	t.Run("forbidden existential fires", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Forbids: []goldengen.Expect{{Name: "Always"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
	t.Run("forbidden pinned fires", func(t *testing.T) {
		f := goldengen.Fixture[int]{Name: "f", Forbids: []goldengen.Expect{{Name: "Always", For: "8.9.0"}}}
		require.Error(t, goldengen.CheckGating(f, versions, firing))
	})
}
