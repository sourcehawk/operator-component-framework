//nolint:dupl
package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIntegrationResource(t *testing.T) {
	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
	}
	identityFunc := func(s *corev1.Service) string { return s.Name }
	defaultApp := func(current, desired *corev1.Service) error {
		current.Spec = desired.Spec
		return nil
	}
	newMutator := func(s *corev1.Service) *mockMutator { return &mockMutator{service: s} }

	res := &IntegrationResource[*corev1.Service, *mockMutator]{
		BaseResource: BaseResource[*corev1.Service, *mockMutator]{
			DesiredObject:          obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApp,
			NewMutator:             newMutator,
		},
	}

	t.Run("Identity", func(t *testing.T) {
		assert.Equal(t, "test-svc", res.Identity())
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		require.NoError(t, err)
		assert.Equal(t, "test-svc", got.GetName())
	})

	t.Run("Mutate and Suspend", func(t *testing.T) {
		current := &corev1.Service{}
		mutCalled := false
		res.Mutations = []Mutation[*mockMutator]{
			{
				Name:    "test-mut",
				Feature: alwaysEnabled{},
				Mutate: func(_ *mockMutator) error {
					mutCalled = true
					return nil
				},
			},
		}

		err := res.Mutate(current)
		require.NoError(t, err)
		assert.True(t, mutCalled, "mutation was not called")

		suspendMutCalled := false
		res.SuspendMutationHandler = func(_ *mockMutator) error {
			suspendMutCalled = true
			return nil
		}

		err = res.Suspend()
		require.NoError(t, err)

		current = &corev1.Service{}
		err = res.Mutate(current)
		require.NoError(t, err)

		assert.True(t, suspendMutCalled, "suspend mutation was not called")
		assert.Nil(t, res.Suspender, "suspender should be nil after use")
	})
}
