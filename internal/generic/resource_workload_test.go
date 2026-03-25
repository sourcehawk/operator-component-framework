package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadResource(t *testing.T) {
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
		},
	}
	identityFunc := func(d *appsv1.Deployment) string { return d.Name }
	newMutator := func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} }

	res := &WorkloadResource[*appsv1.Deployment, *mockMutator]{
		BaseResource: BaseResource[*appsv1.Deployment, *mockMutator]{
			DesiredObject: obj,
			IdentityFunc:  identityFunc,
			NewMutator:    newMutator,
		},
	}

	t.Run("Identity", func(t *testing.T) {
		assert.Equal(t, "test-deploy", res.Identity())
	})

	t.Run("Object", func(t *testing.T) {
		got, err := res.Object()
		require.NoError(t, err)
		assert.Equal(t, "test-deploy", got.GetName())
	})

	t.Run("Mutate", func(t *testing.T) {
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

		obj, err := res.Object()
		require.NoError(t, err)
		require.NoError(t, res.Mutate(obj))
		assert.True(t, mutCalled, "mutation was not called")
	})

	t.Run("Suspend", func(t *testing.T) {
		suspendMutCalled := false
		res.SuspendMutationHandler = func(_ *mockMutator) error {
			suspendMutCalled = true
			return nil
		}

		err := res.Suspend()
		require.NoError(t, err)

		obj, err := res.Object()
		require.NoError(t, err)
		err = res.Mutate(obj)
		require.NoError(t, err)

		assert.True(t, suspendMutCalled, "suspend mutation was not called")
		assert.Nil(t, res.Suspender, "suspender should be nil after use")
	})

	t.Run("Status handlers", func(t *testing.T) {
		res.ConvergingStatusHandler = func(_ concepts.ConvergingOperation, _ *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}, nil
		}
		res.GraceStatusHandler = func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
		}
		res.SuspendStatusHandler = func(_ *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res.DeleteOnSuspendHandler = func(_ *appsv1.Deployment) bool {
			return true
		}

		cs, _ := res.ConvergingStatus(concepts.ConvergingOperationCreated)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, cs.Status)

		gs, _ := res.GraceStatus()
		assert.Equal(t, concepts.GraceStatusHealthy, gs.Status)

		ss, _ := res.SuspensionStatus()
		assert.Equal(t, concepts.SuspensionStatusSuspended, ss.Status)

		assert.True(t, res.DeleteOnSuspend())
	})
}
