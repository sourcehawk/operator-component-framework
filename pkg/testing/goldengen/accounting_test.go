package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
)

// configWithRegistered builds a Config whose Build registers a StatefulSet with the
// given mutation names, attaching the given Requires to a single fixture and the
// given Exclude to the config.
func configWithRegistered(t *testing.T, registered, requires, exclude []string) goldengen.Config[*struct{}] {
	t.Helper()
	reqs := make([]goldengen.Expect, 0, len(requires))
	for _, name := range requires {
		reqs = append(reqs, goldengen.Expect{Name: name})
	}
	return goldengen.Config[*struct{}]{
		Dir:      "testdata/accounting",
		Versions: []string{"1.0.0"},
		Exclude:  exclude,
		Fixtures: []goldengen.Fixture[*struct{}]{{
			Name:     "default",
			Spec:     &struct{}{},
			Requires: reqs,
		}},
		Build: func(_ string, _ *struct{}) (goldengen.Unit, error) {
			muts := make([]statefulset.Mutation, 0, len(registered))
			for _, name := range registered {
				muts = append(muts, statefulset.Mutation(feature.Mutation[*statefulset.Mutator]{
					Name:   name,
					Mutate: func(*statefulset.Mutator) error { return nil },
				}))
			}
			res, err := statefulset.NewBuilder(baseStatefulSet()).
				WithMutation(muts...).
				Build()
			if err != nil {
				return nil, err
			}
			return goldengen.Resource(res, testScheme()), nil
		},
	}
}

func TestAssertComplete(t *testing.T) {
	t.Run("complete passes", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A", "B"}, []string{"A"}, []string{"B"}))
		assert.Equal(t, 0, gen.AssertComplete(0))
	})

	t.Run("unaccounted fails", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A", "B"}, []string{"A"}, nil))
		assert.NotEqual(t, 0, gen.AssertComplete(0)) // B is neither required nor excluded
	})

	t.Run("stale exclude fails", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A"}, []string{"A"}, []string{"Ghost"}))
		assert.NotEqual(t, 0, gen.AssertComplete(0)) // Ghost is excluded but never registered
	})

	t.Run("stale requires fails", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A"}, []string{"A", "Ghost"}, nil))
		assert.NotEqual(t, 0, gen.AssertComplete(0)) // Ghost is required but never registered
	})

	t.Run("passes through nonzero code", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A"}, []string{"A"}, nil))
		assert.Equal(t, 7, gen.AssertComplete(7)) // accounting holds, but tests already failed
	})

	t.Run("nonzero code wins over accounting failure", func(t *testing.T) {
		gen := goldengen.New(configWithRegistered(t, []string{"A", "B"}, []string{"A"}, nil))
		assert.Equal(t, 7, gen.AssertComplete(7)) // incoming nonzero is returned unchanged
	})

	t.Run("invalid config fails", func(t *testing.T) {
		cfg := configWithRegistered(t, []string{"A"}, []string{"A"}, nil)
		cfg.Versions = nil
		gen := goldengen.New(cfg)
		assert.NotEqual(t, 0, gen.AssertComplete(0))
	})
}
