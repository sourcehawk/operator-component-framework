package statefulset

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("Build validation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			statefulset *appsv1.StatefulSet
			expectedErr string
		}{
			{
				name:        "nil statefulset",
				statefulset: nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				statefulset: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				statefulset: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sts",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid statefulset",
				statefulset: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sts",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.statefulset).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "apps/v1/StatefulSet/test-ns/test-sts", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		m := Mutation{
			Name:   "test-mutation",
			Mutate: func(_ *Mutator) error { return nil },
		}
		res, err := NewBuilder(sts).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithMutation variadic registers in order", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		first := Mutation{Name: "first", Mutate: func(_ *Mutator) error { return nil }}
		second := Mutation{Name: "second", Mutate: func(_ *Mutator) error { return nil }}

		builder := NewBuilder(sts)
		require.Same(t, builder, builder.WithMutation(first, second))

		res, err := builder.Build()
		require.NoError(t, err)
		require.Len(t, res.base.Mutations, 2)
		assert.Equal(t, "first", res.base.Mutations[0].Name)
		assert.Equal(t, "second", res.base.Mutations[1].Name)
	})

	t.Run("WithMutation spread and zero args", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		muts := []Mutation{
			{Name: "a", Mutate: func(_ *Mutator) error { return nil }},
			{Name: "b", Mutate: func(_ *Mutator) error { return nil }},
		}

		builder := NewBuilder(sts)
		require.Same(t, builder, builder.WithMutation(muts...))
		require.Same(t, builder, builder.WithMutation())

		res, err := builder.Build()
		require.NoError(t, err)
		require.Len(t, res.base.Mutations, 2)
		assert.Equal(t, "a", res.base.Mutations[0].Name)
		assert.Equal(t, "b", res.base.Mutations[1].Name)
	})

	t.Run("WithCustomConvergeStatus", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *appsv1.StatefulSet) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusUpdating}, nil
		}
		res, err := NewBuilder(sts).
			WithCustomConvergeStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.ConvergingStatusHandler)
		status, err := res.base.ConvergingStatusHandler(concepts.ConvergingOperationUpdated, nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusUpdating, status.Status)
	})

	t.Run("WithCustomGraceStatus", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
		}
		res, err := NewBuilder(sts).
			WithCustomGraceStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.GraceStatusHandler)
		status, err := res.base.GraceStatusHandler(nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
	})

	t.Run("WithCustomSuspendStatus", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.StatefulSet) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(sts).
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
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(sts).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.StatefulSet) bool {
			return true
		}
		res, err := NewBuilder(sts).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.True(t, res.base.DeleteOnSuspendHandler(nil))
	})

	t.Run("WithDataExtractor", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		called := false
		extractor := func(_ appsv1.StatefulSet) error {
			called = true
			return nil
		}
		res, err := NewBuilder(sts).
			WithDataExtractor(extractor).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 1)
		err = res.base.DataExtractors[0](&appsv1.StatefulSet{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("WithDataExtractor nil", func(t *testing.T) {
		t.Parallel()
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(sts).
			WithDataExtractor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 0)
	})
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default", Labels: map[string]string{"team": "platform"}},
	})
	ExtractInto(builder, cell, func(o appsv1.StatefulSet) (string, error) {
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
	builder := NewBuilder(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default"},
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
