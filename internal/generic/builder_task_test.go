//nolint:dupl
package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTaskBuilder(t *testing.T) {
	obj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	identityFunc := func(j *batchv1.Job) string { return j.Name }
	newMutator := func(j *batchv1.Job) *mockMutator { return &mockMutator{job: j} }

	t.Run("successful build", func(t *testing.T) {
		builder := NewTaskBuilder(obj, identityFunc, newMutator)
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
		builder := NewTaskBuilder(obj, identityFunc, newMutator).WithMutation(mut)
		res, _ := builder.Build()
		assert.Len(t, res.Mutations, 1)
	})

	t.Run("with handlers", func(t *testing.T) {
		builder := NewTaskBuilder(obj, identityFunc, newMutator).
			WithCustomConvergeStatus(func(_ concepts.ConvergingOperation, _ *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
				return concepts.CompletionStatusWithReason{}, nil
			}).
			WithCustomSuspendStatus(func(_ *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
				return concepts.SuspensionStatusWithReason{}, nil
			}).
			WithCustomSuspendMutation(func(_ *mockMutator) error {
				return nil
			}).
			WithCustomSuspendDeletionDecision(func(_ *batchv1.Job) bool {
				return true
			})

		res, _ := builder.Build()
		assert.NotNil(t, res.ConvergingStatusHandler, "ConvergingStatusHandler not set")
		assert.NotNil(t, res.SuspendStatusHandler, "SuspendStatusHandler not set")
		assert.NotNil(t, res.SuspendMutationHandler, "SuspendMutationHandler not set")
		assert.NotNil(t, res.DeleteOnSuspendHandler, "DeleteOnSuspendHandler not set")
	})

	t.Run("cluster-scoped build succeeds without namespace", func(t *testing.T) {
		clusterObj := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj"},
		}
		builder := NewTaskBuilder(clusterObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		res, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, clusterObj, res.DesiredObject)
	})

	t.Run("cluster-scoped build rejects non-empty namespace", func(t *testing.T) {
		nsObj := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obj", Namespace: "oops"},
		}
		builder := NewTaskBuilder(nsObj, identityFunc, newMutator)
		builder.MarkClusterScoped()
		_, err := builder.Build()
		require.EqualError(t, err, errClusterScopedNamespace)
	})

	t.Run("validation errors", func(t *testing.T) {
		runBuilderValidationTests[*TaskResource[*batchv1.Job, *mockMutator]](
			t, obj, identityFunc, newMutator,
			func(o *batchv1.Job, id func(*batchv1.Job) string, mut func(*batchv1.Job) *mockMutator) genericBuilder[*TaskResource[*batchv1.Job, *mockMutator]] {
				return NewTaskBuilder(o, id, mut)
			},
		)
	})
}
