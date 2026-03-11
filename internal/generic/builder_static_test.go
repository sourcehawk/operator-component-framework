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

	t.Run("successful build", func(t *testing.T) {
		builder := NewStaticBuilder(obj, identityFunc, defaultApp)
		res, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if res.Object != obj {
			t.Errorf("expected object %v, got %v", obj, res.Object)
		}
	})

	t.Run("with custom applicator", func(t *testing.T) {
		customApp := func(_, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp).WithCustomFieldApplicator(customApp)
		res, _ := builder.Build()
		if reflectValueOf(res.CustomFieldApplicator).Pointer() != reflectValueOf(customApp).Pointer() {
			t.Errorf("custom applicator not set correctly")
		}
	})

	t.Run("with field application flavor", func(t *testing.T) {
		flavor := func(_, _, _ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp).WithFieldApplicationFlavor(flavor)
		res, _ := builder.Build()
		if len(res.FieldFlavors) != 1 {
			t.Errorf("expected 1 flavor, got %d", len(res.FieldFlavors))
		}
	})

	t.Run("with data extractor", func(t *testing.T) {
		extractor := func(_ *corev1.ConfigMap) error { return nil }
		builder := NewStaticBuilder(obj, identityFunc, defaultApp).WithDataExtractor(extractor)
		res, _ := builder.Build()
		if len(res.DataExtractors) != 1 {
			t.Errorf("expected 1 extractor, got %d", len(res.DataExtractors))
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		tests := []struct {
			name    string
			obj     *corev1.ConfigMap
			idFunc  func(*corev1.ConfigMap) string
			defApp  FieldApplicator[*corev1.ConfigMap]
			wantErr string
		}{
			{"nil object", nil, identityFunc, defaultApp, "object cannot be nil"},
			{"empty name", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}, identityFunc, defaultApp, "object name cannot be empty"},
			{"empty namespace", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, identityFunc, defaultApp, "object namespace cannot be empty"},
			{"nil identity", obj, nil, defaultApp, "identity function cannot be nil"},
			{"nil applicator", obj, identityFunc, nil, "default field applicator cannot be nil"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := NewStaticBuilder(tt.obj, tt.idFunc, tt.defApp).Build()
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
			})
		}
	})
}
