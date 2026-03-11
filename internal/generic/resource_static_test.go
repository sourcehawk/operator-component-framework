package generic

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStaticResource(t *testing.T) {
	const testVal = "bar"
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"foo": testVal},
	}
	identityFunc := func(cm *corev1.ConfigMap) string { return cm.Name }
	defaultApp := func(current, desired *corev1.ConfigMap) error {
		current.Data = desired.Data
		return nil
	}

	res := &StaticResource[*corev1.ConfigMap]{
		Object:                 obj,
		IdentityFunc:           identityFunc,
		DefaultFieldApplicator: defaultApp,
	}

	t.Run("Identity", func(t *testing.T) {
		if res.Identity() != "test-cm" {
			t.Errorf("expected identity test-cm, got %s", res.Identity())
		}
	})

	t.Run("GetObject", func(t *testing.T) {
		got, err := res.GetObject()
		if err != nil {
			t.Fatalf("GetObject() error = %v", err)
		}
		if got.GetName() != "test-cm" {
			t.Errorf("expected name test-cm, got %s", got.GetName())
		}
		if got == res.Object {
			t.Errorf("GetObject should return a copy, not the same object")
		}
	})

	t.Run("Mutate", func(t *testing.T) {
		current := &corev1.ConfigMap{}
		err := res.Mutate(current)
		if err != nil {
			t.Fatalf("Mutate() error = %v", err)
		}
		if current.Data["foo"] != testVal {
			t.Errorf("expected foo=%s, got %v", testVal, current.Data["foo"])
		}
	})

	t.Run("ExtractData", func(t *testing.T) {
		extracted := false
		res.DataExtractors = []func(*corev1.ConfigMap) error{
			func(cm *corev1.ConfigMap) error {
				extracted = true
				if cm.Data["foo"] != testVal {
					t.Errorf("expected foo=%s in extractor", testVal)
				}
				return nil
			},
		}
		err := res.ExtractData()
		if err != nil {
			t.Fatalf("ExtractData() error = %v", err)
		}
		if !extracted {
			t.Errorf("extractor was not called")
		}
	})

	t.Run("ExtractData error", func(t *testing.T) {
		res.DataExtractors = []func(*corev1.ConfigMap) error{
			func(_ *corev1.ConfigMap) error {
				return errors.New("extract error")
			},
		}
		err := res.ExtractData()
		if err == nil || err.Error() != "extract error" {
			t.Errorf("expected extract error, got %v", err)
		}
	})
}
