package job

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("Build validation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			job         *batchv1.Job
			expectedErr string
		}{
			{
				name:        "nil job",
				job:         nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				job: &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				job: &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-job",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid job",
				job: &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.job).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "batch/v1/Job/test-ns/test-job", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}
		m := Mutation{
			Name: "test-mutation",
		}
		res, err := NewBuilder(job).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithCustomConvergeStatus", func(t *testing.T) {
		t.Parallel()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *batchv1.Job) (concepts.CompletionStatusWithReason, error) {
			return concepts.CompletionStatusWithReason{Status: concepts.CompletionStatusCompleted}, nil
		}
		res, err := NewBuilder(job).
			WithCustomConvergeStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.ConvergingStatusHandler)
		status, err := res.base.ConvergingStatusHandler(concepts.ConvergingOperationUpdated, nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.CompletionStatusCompleted, status.Status)
	})

	t.Run("WithCustomSuspendStatus", func(t *testing.T) {
		t.Parallel()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *batchv1.Job) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(job).
			WithCustomSuspendStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendStatusHandler)
		status, err := res.base.SuspendStatusHandler(nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.SuspensionStatusSuspended, status.Status)
	})

	t.Run("WithCustomSuspendMutation", func(t *testing.T) {
		t.Parallel()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(job).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *batchv1.Job) bool {
			return false
		}
		res, err := NewBuilder(job).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.False(t, res.base.DeleteOnSuspendHandler(nil))
	})
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "default", Labels: map[string]string{"team": "platform"}},
	})
	ExtractInto(builder, cell, func(o batchv1.Job) (string, error) {
		return o.Labels["team"], nil
	})

	res, err := builder.Build()
	require.NoError(t, err)

	produced := res.ProducedData()
	require.Len(t, produced, 1)
	assert.Equal(t, "team-label", produced[0].Name())

	require.NoError(t, res.ExtractData())
	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "platform", v)
}

func TestWithDataGuardAndOptionalDataDeclarations(t *testing.T) {
	t.Parallel()
	guarded := concepts.NewData[string]("db-host")
	optional := concepts.NewData[string]("db-port")
	builder := NewBuilder(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "default"},
	}).WithDataGuard(guarded).WithOptionalData(optional)

	res, err := builder.Build()
	require.NoError(t, err)

	consumed := res.ConsumedData()
	require.Len(t, consumed, 2)
	assert.Equal(t, "db-host", consumed[0].Cell.Name())
	assert.False(t, consumed[0].Optional)
	assert.Equal(t, "db-port", consumed[1].Cell.Name())
	assert.True(t, consumed[1].Optional)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.Equal(t, `waiting for data "db-host"`, status.Reason)

	guarded.Set("postgres.default.svc")
	status, err = res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
}
