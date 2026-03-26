package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadBuilder(t *testing.T) {
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
		},
	}
	identityFunc := func(d *appsv1.Deployment) string { return d.Name }
	newMutator := func(d *appsv1.Deployment) *mockMutator { return &mockMutator{deployment: d} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewWorkloadBuilder(obj, identityFunc, newMutator)
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, obj, res.DesiredObject)
	})

	t.Run("with mutation", func(t *testing.T) {
		mut := Mutation[*mockMutator]{
			Name:    "test-mutation",
			Feature: alwaysEnabled{},
			Mutate: func(_ *mockMutator) error {
				return nil
			},
		}
		builder := NewWorkloadBuilder(obj, identityFunc, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		assert.Len(t, res.Mutations, 1)
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewWorkloadBuilder(obj, identityFunc, newMutator).
			WithCustomConvergeStatus(func(_ concepts.ConvergingOperation, _ *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
				return concepts.AliveStatusWithReason{}, nil
			}).
			WithCustomGraceStatus(func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
				return concepts.GraceStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(_ *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
				return concepts.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(_ *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(_ *appsv1.Deployment) bool {
				return true
			})

		res, _ := builder.Build()
		assert.NotNil(t, res.ConvergingStatusHandler, "ConvergingStatusHandler not set")
		assert.NotNil(t, res.GraceStatusHandler, "GraceStatusHandler not set")
		assert.NotNil(t, res.SuspendStatusHandler, "SuspendStatusHandler not set")
		assert.NotNil(t, res.SuspendMutationHandler, "SuspendMutationHandler not set")
		assert.NotNil(t, res.DeleteOnSuspendHandler, "DeleteOnSuspendHandler not set")
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewWorkloadBuilder(clusterObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, clusterObj, res.DesiredObject)
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewWorkloadBuilder(nsObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		require.EqualError(t, err, errClusterScopedNamespace)
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*WorkloadResource[*appsv1.Deployment, *mockMutator]](
			t, obj, identityFunc, newMutator,
			func(o *appsv1.Deployment, id func(*appsv1.Deployment) string, mut func(*appsv1.Deployment) *mockMutator) genericBuilder[*WorkloadResource[*appsv1.Deployment, *mockMutator]] {
				return NewWorkloadBuilder(o, id, mut)
			},
		)
	})
}
