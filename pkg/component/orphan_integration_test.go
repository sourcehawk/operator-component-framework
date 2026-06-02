package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// orphanTestResource returns a MockResource stubbed to act as a ConfigMap named
// name in test-namespace. Object and Identity are the only methods the orphan
// path invokes, so only those are registered.
func orphanTestResource(name string) *MockResource {
	res := &MockResource{}
	res.On("Object").Return(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-namespace"},
	}, nil)
	res.On("Identity").Return("v1/ConfigMap/" + name)
	return res
}

func TestBuild_OrphanWhenPartition(t *testing.T) {
	t.Run("OrphanWhen(true) partitions into orphanResources", func(t *testing.T) {
		c, err := NewComponentBuilder().
			WithName("orphan-comp").
			WithConditionType("Ready").
			WithResource(orphanTestResource("orphaned-cm"), OrphanWhen(true)).
			Build()
		require.NoError(t, err)

		assert.Len(t, c.orphanResources, 1)
		assert.Empty(t, c.deleteResources)
		assert.Empty(t, c.allManagedResources())
	})

	t.Run("OrphanWhen(false) is managed normally", func(t *testing.T) {
		managed, err := NewComponentBuilder().
			WithName("managed-comp").
			WithConditionType("Ready").
			WithResource(orphanTestResource("managed-cm"), OrphanWhen(false)).
			Build()
		require.NoError(t, err)

		assert.Empty(t, managed.orphanResources)
		assert.Len(t, managed.reconcileResources, 1)
	})
}
