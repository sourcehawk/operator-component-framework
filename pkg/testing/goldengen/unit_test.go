package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceAdapter(t *testing.T) {
	res := buildStatefulSetWith(t, statefulset.Mutation{
		Name:   "Always",
		Mutate: func(*statefulset.Mutator) error { return nil },
	})
	u := goldengen.Resource(res, testScheme())

	assert.Equal(t, []string{"Always"}, u.RegisteredMutations())

	firing, err := u.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"Always"}, firing)

	y, err := u.RenderYAML()
	require.NoError(t, err)
	assert.Contains(t, string(y), "kind: StatefulSet")
}

func TestComponentAdapter(t *testing.T) {
	// Build two distinct resources so the rendered YAML is multi-document.
	a := buildStatefulSetWith(t)
	b := namedStatefulSet(t, "db-2")
	c := buildComponentWith(t, a, b)

	u := goldengen.Component(c, testScheme())
	y, err := u.RenderYAML()
	require.NoError(t, err)
	assert.Contains(t, string(y), "---") // multi-doc when >1 resource
}
