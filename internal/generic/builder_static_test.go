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
	defaultApp := func(_, _ *corev1.ConfigMap) error { return nil }
	newMutator := func(_ *corev1.ConfigMap) *mockMutator { return &mockMutator{} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator)
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, obj, res.DesiredObject)
	})

	t.Run("with custom applicator", func(t *testing.T) {
		customApp := func(_, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithCustomFieldApplicator(customApp)
		res, _ := builder.Build()
		assert.Equal(t, reflectValueOf(customApp).Pointer(), reflectValueOf(res.CustomFieldApplicator).Pointer(), "custom applicator not set correctly")
	})

	t.Run("with field application flavor", func(t *testing.T) {
		flavor := func(_, _, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithFieldApplicationFlavor(flavor)
		res, _ := builder.Build()
		assert.Len(t, res.FieldFlavors, 1)
	})

	t.Run("with data extractor", func(t *testing.T) {
		extractor := func(_ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithDataExtractor(extractor)
		res, _ := builder.Build()
		assert.Len(t, res.DataExtractors, 1)
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: alwaysEnabled{},
			Mutate:  func(_ *mockMutator) error { return nil },
		}
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithMutation(mut)
		res, _ := builder.Build()
		assert.Len(t, res.Mutations, 1)
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewStaticBuilder(clusterObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, clusterObj, res.DesiredObject)
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewStaticBuilder(nsObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		require.EqualError(t, err, errClusterScopedNamespace)
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests(
			t,
			obj,
			identityFunc,
			defaultApp,
			newMutator,
			func(
				o *corev1.ConfigMap,
				id func(*corev1.ConfigMap) string,
				app FieldApplicator[*corev1.ConfigMap],
				mut func(*corev1.ConfigMap) *mockMutator,
			) genericBuilder[*StaticResource[*corev1.ConfigMap, *mockMutator]] {
				return NewStaticBuilder(o, id, app, mut)
			},
		)
	})
}
