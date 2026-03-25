package hpa

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("Build validation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			hpa         *autoscalingv2.HorizontalPodAutoscaler
			expectedErr string
		}{
			{
				name:        "nil HPA",
				hpa:         nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				hpa: &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				hpa: &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-hpa",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid HPA",
				hpa: &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hpa",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.hpa).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "autoscaling/v2/HorizontalPodAutoscaler/test-ns/test-hpa", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		m := Mutation{
			Name: "test-mutation",
		}
		res, err := NewBuilder(hpa).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithCustomOperationalStatus", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *autoscalingv2.HorizontalPodAutoscaler) (concepts.OperationalStatusWithReason, error) {
			return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
		}
		res, err := NewBuilder(hpa).
			WithCustomOperationalStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.OperationalStatusHandler)
		status, err := res.base.OperationalStatusHandler(concepts.ConvergingOperationNone, nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
	})

	t.Run("WithCustomSuspendStatus", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *autoscalingv2.HorizontalPodAutoscaler) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(hpa).
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
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(hpa).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *autoscalingv2.HorizontalPodAutoscaler) bool {
			return false
		}
		res, err := NewBuilder(hpa).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.False(t, res.base.DeleteOnSuspendHandler(nil))
	})

	t.Run("WithDataExtractor", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		called := false
		extractor := func(_ autoscalingv2.HorizontalPodAutoscaler) error {
			called = true
			return nil
		}
		res, err := NewBuilder(hpa).
			WithDataExtractor(extractor).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 1)
		err = res.base.DataExtractors[0](&autoscalingv2.HorizontalPodAutoscaler{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("WithDataExtractor nil", func(t *testing.T) {
		t.Parallel()
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hpa",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(hpa).
			WithDataExtractor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 0)
	})
}
