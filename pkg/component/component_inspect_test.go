package component

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// inspectableResource is a minimal Resource that also implements
// concepts.MutationInspector, for exercising the component's aggregation.
type inspectableResource struct {
	identity   string
	registered []string
	firing     []string
	firingErr  error
}

func (r *inspectableResource) Identity() string { return r.identity }

func (r *inspectableResource) Object() (client.Object, error) {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: r.identity}}, nil
}

func (r *inspectableResource) Mutate(_ client.Object) error  { return nil }
func (r *inspectableResource) RegisteredMutations() []string { return r.registered }
func (r *inspectableResource) FiringSet() ([]string, error)  { return r.firing, r.firingErr }

func buildInspectComponent(t *testing.T, regs ...func(*Builder)) *Component {
	t.Helper()
	b := NewComponentBuilder().WithName("test").WithConditionType("Ready")
	for _, reg := range regs {
		reg(b)
	}
	c, err := b.Build()
	require.NoError(t, err)
	return c
}

func TestComponentRegisteredMutationsUnion(t *testing.T) {
	res1 := &inspectableResource{identity: "res1", registered: []string{"A", "B"}}
	res2 := &inspectableResource{identity: "res2", registered: []string{"B", "C"}}
	res3 := &inspectableResource{identity: "res3", registered: []string{"X"}}

	c := buildInspectComponent(t,
		func(b *Builder) { b.WithResource(res1) },
		func(b *Builder) { b.WithResource(res2) },
		func(b *Builder) { b.WithResource(res3, ReadOnly()) },
	)

	// Read-only res3 is excluded; union preserves registration order.
	assert.Equal(t, []string{"A", "B", "C"}, c.RegisteredMutations())
}

func TestComponentFiringSetUnion(t *testing.T) {
	res1 := &inspectableResource{identity: "res1", registered: []string{"A"}, firing: []string{"A"}}
	res2 := &inspectableResource{identity: "res2", registered: []string{"C"}, firing: []string{"C"}}
	res3 := &inspectableResource{identity: "res3", registered: []string{"X"}, firing: []string{"X"}}

	c := buildInspectComponent(t,
		func(b *Builder) { b.WithResource(res1) },
		func(b *Builder) { b.WithResource(res2) },
		func(b *Builder) { b.WithResource(res3, ReadOnly()) },
	)

	firing, err := c.FiringSet()
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "C"}, firing)
}

func TestComponentFiringSetPropagatesError(t *testing.T) {
	res := &inspectableResource{identity: "res", registered: []string{"A"}, firingErr: errors.New("boom")}

	c := buildInspectComponent(t, func(b *Builder) { b.WithResource(res) })

	_, err := c.FiringSet()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "res")
}
