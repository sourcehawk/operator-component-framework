package generic

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
)

// reflectValueOf is a helper for testing function equality.
func reflectValueOf(i any) reflect.Value {
	return reflect.ValueOf(i)
}

type mockMutator struct {
	deployment *appsv1.Deployment
	applied    bool
}

func (m *mockMutator) Apply() error {
	m.applied = true
	return nil
}
