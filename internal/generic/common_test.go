package generic

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const errClusterScopedNamespace = "cluster-scoped object must not have a namespace"

// reflectValueOf is a helper for testing function equality.
func reflectValueOf(i any) reflect.Value {
	return reflect.ValueOf(i)
}

type mockMutator struct {
	deployment *appsv1.Deployment
	service    *corev1.Service
	job        *batchv1.Job
	applied    bool
}

func (m *mockMutator) Apply() error {
	m.applied = true
	return nil
}

// alwaysEnabled is a MutationFeature that always reports enabled.
type alwaysEnabled struct{}

func (alwaysEnabled) Enabled() (bool, error) { return true, nil }

// mockMutation constructs a Mutation for *mockMutator that is always applied.
func mockMutation(fn func(*mockMutator) error) Mutation[*mockMutator] {
	return Mutation[*mockMutator]{
		Name:    "mock-mutation",
		Feature: alwaysEnabled{},
		Mutate:  fn,
	}
}

type genericBuilder[T any] interface {
	Build() (T, error)
}

func runBuilderValidationTests[T any, O client.Object, M MutatorApplier](
	t *testing.T,
	obj O,
	identityFunc func(O) string,
	defaultApp FieldApplicator[O],
	newMutator func(O) M,
	newBuilder func(O, func(O) string, FieldApplicator[O], func(O) M) genericBuilder[T],
) {
	t.Helper()
	tests := []struct {
		name    string
		obj     O
		idFunc  func(O) string
		defApp  FieldApplicator[O]
		newMut  func(O) M
		wantErr string
	}{
		{"nil object", *new(O), identityFunc, defaultApp, newMutator, "object cannot be nil"},
		{"empty name", func() O {
			o := obj.DeepCopyObject().(O)
			o.SetName("")
			o.SetNamespace("default")
			return o
		}(), identityFunc, defaultApp, newMutator, "object name cannot be empty"},
		{"empty namespace", func() O {
			o := obj.DeepCopyObject().(O)
			o.SetName("test")
			o.SetNamespace("")
			return o
		}(), identityFunc, defaultApp, newMutator, "object namespace cannot be empty"},
		{"nil identity", obj, nil, defaultApp, newMutator, "identity function cannot be nil"},
		{"nil applicator", obj, identityFunc, nil, newMutator, "default field applicator cannot be nil"},
		{"nil mutator factory", obj, identityFunc, defaultApp, nil, "mutator factory cannot be nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newBuilder(tt.obj, tt.idFunc, tt.defApp, tt.newMut).Build()
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}
