package ingress

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("Build validation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			ingress     *networkingv1.Ingress
			expectedErr string
		}{
			{
				name:        "nil ingress",
				ingress:     nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ing",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid ingress",
				ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ing",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.ingress).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "networking.k8s.io/v1/Ingress/test-ns/test-ing", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		m := Mutation{
			Name: "test-mutation",
		}
		res, err := NewBuilder(ing).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithCustomFieldApplicator", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		applied := false
		applicator := func(_, _ *networkingv1.Ingress) error {
			applied = true
			return nil
		}
		res, err := NewBuilder(ing).
			WithCustomFieldApplicator(applicator).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.CustomFieldApplicator)
		_ = res.base.CustomFieldApplicator(nil, nil)
		assert.True(t, applied)
	})

	t.Run("WithFieldApplicationFlavor", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(ing).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			WithFieldApplicationFlavor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.FieldFlavors, 1)
	})

	t.Run("WithCustomOperationalStatus", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *networkingv1.Ingress) (concepts.OperationalStatusWithReason, error) {
			return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusFailing}, nil
		}
		res, err := NewBuilder(ing).
			WithCustomOperationalStatus(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.OperationalStatusHandler)
		status, err := res.base.OperationalStatusHandler(concepts.ConvergingOperationNone, nil)
		require.NoError(t, err)
		assert.Equal(t, concepts.OperationalStatusFailing, status.Status)
	})

	t.Run("WithCustomSuspendStatus", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *networkingv1.Ingress) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(ing).
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
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(ing).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *networkingv1.Ingress) bool {
			return true
		}
		res, err := NewBuilder(ing).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.True(t, res.base.DeleteOnSuspendHandler(nil))
	})

	t.Run("WithDataExtractor", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		called := false
		extractor := func(_ networkingv1.Ingress) error {
			called = true
			return nil
		}
		res, err := NewBuilder(ing).
			WithDataExtractor(extractor).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 1)
		err = res.base.DataExtractors[0](&networkingv1.Ingress{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("WithDataExtractor nil", func(t *testing.T) {
		t.Parallel()
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ing",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(ing).
			WithDataExtractor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 0)
	})
}
