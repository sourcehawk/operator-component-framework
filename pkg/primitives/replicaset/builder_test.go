package replicaset

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
			replicaset  *appsv1.ReplicaSet
			expectedErr string
		}{
			{
				name:        "nil replicaset",
				replicaset:  nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				replicaset: &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				replicaset: &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-rs",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid replicaset",
				replicaset: &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rs",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.replicaset).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "apps/v1/ReplicaSet/test-ns/test-rs", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		m := Mutation{
			Name: "test-mutation",
		}
		res, err := NewBuilder(rs).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithCustomFieldApplicator", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		applied := false
		applicator := func(_ *appsv1.ReplicaSet, _ *appsv1.ReplicaSet) error {
			applied = true
			return nil
		}
		res, err := NewBuilder(rs).
			WithCustomFieldApplicator(applicator).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.CustomFieldApplicator)
		_ = res.base.CustomFieldApplicator(nil, nil)
		assert.True(t, applied)
	})

	t.Run("WithFieldApplicationFlavor", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(rs).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			WithFieldApplicationFlavor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.FieldFlavors, 1)
	})

	t.Run("WithCustomConvergeStatus", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *appsv1.ReplicaSet) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusUpdating}, nil
		}
		res, err := NewBuilder(rs).
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
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.ReplicaSet) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
		}
		res, err := NewBuilder(rs).
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
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.ReplicaSet) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(rs).
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
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(rs).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.ReplicaSet) bool {
			return true
		}
		res, err := NewBuilder(rs).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.True(t, res.base.DeleteOnSuspendHandler(nil))
	})

	t.Run("WithDataExtractor", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		called := false
		extractor := func(_ appsv1.ReplicaSet) error {
			called = true
			return nil
		}
		res, err := NewBuilder(rs).
			WithDataExtractor(extractor).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 1)
		err = res.base.DataExtractors[0](&appsv1.ReplicaSet{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("WithDataExtractor nil", func(t *testing.T) {
		t.Parallel()
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(rs).
			WithDataExtractor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 0)
	})
}
