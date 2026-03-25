//nolint:dupl
package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTaskResource(t *testing.T) {
	obj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	identityFunc := func(j *batchv1.Job) string { return j.Name }
	newMutator := func(j *batchv1.Job) *mockMutator { return &mockMutator{job: j} }

	res := &TaskResource[*batchv1.Job, *mockMutator]{
		BaseResource: BaseResource[*batchv1.Job, *mockMutator]{
			DesiredObject: obj,
			IdentityFunc:  identityFunc,
			NewMutator:    newMutator,
		},
	}

	t.Run("Identity", func(t *testing.T) {
		assert.Equal(t, "test-job", res.Identity())
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		require.NoError(t, err)
		assert.Equal(t, "test-job", got.GetName())
	})

	t.Run("Mutate and Suspend", func(t *testing.T) {
		current := obj.DeepCopy()
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

		current = obj.DeepCopy()
		err = res.Mutate(current)
		require.NoError(t, err)

		assert.True(t, suspendMutCalled, "suspend mutation was not called")
		assert.Nil(t, res.Suspender, "suspender should be nil after use")
	})
}
