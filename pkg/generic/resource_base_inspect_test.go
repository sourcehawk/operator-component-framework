package generic_test

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// staticGate is a feature.Gate with a fixed outcome for tests.
type staticGate struct {
	enabled bool
	err     error
}

func (g staticGate) Enabled() (bool, error) { return g.enabled, g.err }

// noopMutator is a minimal generic.FeatureMutator for exercising BaseResource.
type noopMutator struct{}

func (noopMutator) NextFeature() {}
func (noopMutator) Apply() error { return nil }

func newBase(mutations ...generic.Mutation[noopMutator]) *generic.BaseResource[*corev1.ConfigMap, noopMutator] {
	return &generic.BaseResource[*corev1.ConfigMap, noopMutator]{
		DesiredObject: &corev1.ConfigMap{},
		NewMutator:    func(*corev1.ConfigMap) noopMutator { return noopMutator{} },
		IdentityFunc:  func(*corev1.ConfigMap) string { return "cm" },
		Mutations:     mutations,
	}
}

func TestBaseResourceRegisteredMutations(t *testing.T) {
	base := newBase(
		generic.Mutation[noopMutator]{Name: "A"},
		generic.Mutation[noopMutator]{Name: "B"},
		generic.Mutation[noopMutator]{Name: "A"}, // duplicate
	)
	assert.Equal(t, []string{"A", "B"}, base.RegisteredMutations())
}

func TestBaseResourceFiringSet(t *testing.T) {
	base := newBase(
		generic.Mutation[noopMutator]{Name: "Always"},                                  // nil gate -> fires
		generic.Mutation[noopMutator]{Name: "On", Feature: staticGate{enabled: true}},   // fires
		generic.Mutation[noopMutator]{Name: "Off", Feature: staticGate{enabled: false}}, // does not fire
	)
	firing, err := base.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always", "On"}, firing)
}

func TestBaseResourceFiringSetGateError(t *testing.T) {
	base := newBase(
		generic.Mutation[noopMutator]{Name: "Bad", Feature: staticGate{err: errors.New("boom")}},
	)
	_, err := base.FiringSet()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bad")
}
