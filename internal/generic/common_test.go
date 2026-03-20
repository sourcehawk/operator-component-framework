package generic

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// reflectValueOf is a helper for testing function equality.
func reflectValueOf(i any) reflect.Value {
	return reflect.ValueOf(i)
}

type mockMutator struct {
	deployment *appsv1.Deployment
	service    *corev1.Service
	applied    bool
}

func (m *mockMutator) Apply() error {
	m.applied = true
	return nil
}
