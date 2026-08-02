package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStaticBuilder(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
	}
	identityFunc := func(cm *corev1.ConfigMap) string { return cm.Name }
	newMutator := func(_ *corev1.ConfigMap) *mockMutator { return &mockMutator{} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewStaticBuilder(obj, identityFunc, newMutator)
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, obj, res.DesiredObject)
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: alwaysEnabled{},
			Mutate:  func(_ *mockMutator) error { return nil },
		}
		builder := NewStaticBuilder(obj, identityFunc, newMutator).
			WithMutation(mut)
		res, _ := builder.Build()
		assert.Len(t, res.Mutations, 1)
	})

	t.Run("registers multiple mutations in order", func(t *testing.T) {
		first := Mutation[*mockMutator]{
			Name:    "first",
			Feature: alwaysEnabled{},
			Mutate:  func(_ *mockMutator) error { return nil },
		}
		second := Mutation[*mockMutator]{
			Name:    "second",
			Feature: alwaysEnabled{},
			Mutate:  func(_ *mockMutator) error { return nil },
		}

		builder := NewStaticBuilder(obj, identityFunc, newMutator)
		result := builder.WithMutation(first, second)
		require.Same(t, builder, result)

		res, err := result.Build()
		require.NoError(t, err)
		require.Len(t, res.Mutations, 2)
		assert.Equal(t, "first", res.Mutations[0].Name)
		assert.Equal(t, "second", res.Mutations[1].Name)
	})

	t.Run("spread slice registers all mutations in order", func(t *testing.T) {
		muts := []Mutation[*mockMutator]{
			{Name: "a", Feature: alwaysEnabled{}, Mutate: func(_ *mockMutator) error { return nil }},
			{Name: "b", Feature: alwaysEnabled{}, Mutate: func(_ *mockMutator) error { return nil }},
			{Name: "c", Feature: alwaysEnabled{}, Mutate: func(_ *mockMutator) error { return nil }},
		}

		res, err := NewStaticBuilder(obj, identityFunc, newMutator).
			WithMutation(muts...).
			Build()
		require.NoError(t, err)
		require.Len(t, res.Mutations, 3)
		assert.Equal(t, "a", res.Mutations[0].Name)
		assert.Equal(t, "b", res.Mutations[1].Name)
		assert.Equal(t, "c", res.Mutations[2].Name)
	})

	t.Run("zero mutations is a no-op", func(t *testing.T) {
		builder := NewStaticBuilder(obj, identityFunc, newMutator)
		result := builder.WithMutation()
		require.Same(t, builder, result)

		res, err := result.Build()
		require.NoError(t, err)
		assert.Empty(t, res.Mutations)
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewStaticBuilder(clusterObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, clusterObj, res.DesiredObject)
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewStaticBuilder(nsObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		require.EqualError(t, err, errClusterScopedNamespace)
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests(
			t,
			obj,
			identityFunc,
			newMutator,
			func(
				o *corev1.ConfigMap,
				id func(*corev1.ConfigMap) string,
				mut func(*corev1.ConfigMap) *mockMutator,
			) genericBuilder[*StaticResource[*corev1.ConfigMap, *mockMutator]] {
				return NewStaticBuilder(o, id, mut)
			},
		)
	})
}
