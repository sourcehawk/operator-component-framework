package generic

import (
	"testing"

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
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.DesiredObject != obj {
			t.Errorf("expected object %v, got %v", obj, res.DesiredObject)
		}
	})

	t.Run("with custom applicator", func(t *testing.T) {
		customApp := func(_, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithCustomFieldApplicator(customApp)
		res, _ := builder.Build()
		if reflectValueOf(res.CustomFieldApplicator).Pointer() != reflectValueOf(customApp).Pointer() {
			t.Errorf("custom applicator not set correctly")
		}
	})

	t.Run("with field application flavor", func(t *testing.T) {
		flavor := func(_, _, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithFieldApplicationFlavor(flavor)
		res, _ := builder.Build()
		if len(res.FieldFlavors) != 1 {
			t.Errorf("expected 1 flavor, got %d", len(res.FieldFlavors))
		}
	})

	t.Run("with data extractor", func(t *testing.T) {
		extractor := func(_ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp, newMutator).
			WithDataExtractor(extractor)
		res, _ := builder.Build()
		if len(res.DataExtractors) != 1 {
			t.Errorf("expected 1 extractor, got %d", len(res.DataExtractors))
		}
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
		if len(res.Mutations) != 1 {
			t.Errorf("expected 1 mutation, got %d", len(res.Mutations))
		}
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewStaticBuilder(clusterObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.DesiredObject != clusterObj {
			t.Errorf("expected object %v, got %v", clusterObj, res.DesiredObject)
		}
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewStaticBuilder(nsObj, identityFunc, defaultApp, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		if err == nil || err.Error() != errClusterScopedNamespace {
			t.Errorf("expected cluster-scoped namespace error, got %v", err)
		}
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
