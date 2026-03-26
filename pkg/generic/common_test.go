package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const errClusterScopedNamespace = "cluster-scoped object must not have a namespace"

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

func (m *mockMutator) NextFeature() {}

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

func runBuilderValidationTests[T any, O client.Object, M FeatureMutator](
	t *testing.T,
	obj O,
	identityFunc func(O) string,
	newMutator func(O) M,
	newBuilder func(O, func(O) string, func(O) M) genericBuilder[T],
) {
	t.Helper()
	tests := []struct {
		name    string
		obj     O
		idFunc  func(O) string
		newMut  func(O) M
		wantErr string
	}{
		{"nil object", *new(O), identityFunc, newMutator, "object cannot be nil"},
		{"empty name", func() O {
			o := obj.DeepCopyObject().(O)
			o.SetName("")
			o.SetNamespace("default")
			return o
		}(), identityFunc, newMutator, "object name cannot be empty"},
		{"empty namespace", func() O {
			o := obj.DeepCopyObject().(O)
			o.SetName("test")
			o.SetNamespace("")
			return o
		}(), identityFunc, newMutator, "object namespace cannot be empty"},
		{"nil identity", obj, nil, newMutator, "identity function cannot be nil"},
		{"nil mutator factory", obj, identityFunc, nil, "mutator factory cannot be nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newBuilder(tt.obj, tt.idFunc, tt.newMut).Build()
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
