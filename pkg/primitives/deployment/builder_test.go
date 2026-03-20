package deployment

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
			deployment  *appsv1.Deployment
			expectedErr string
		}{
			{
				name:        "nil deployment",
				deployment:  nil,
				expectedErr: "object cannot be nil",
			},
			{
				name: "empty name",
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ns",
					},
				},
				expectedErr: "object name cannot be empty",
			},
			{
				name: "empty namespace",
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-deploy",
					},
				},
				expectedErr: "object namespace cannot be empty",
			},
			{
				name: "valid deployment",
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
					},
				},
				expectedErr: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				res, err := NewBuilder(tt.deployment).Build()
				if tt.expectedErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedErr)
					assert.Nil(t, res)
				} else {
					require.NoError(t, err)
					require.NotNil(t, res)
					assert.Equal(t, "apps/v1/Deployment/test-ns/test-deploy", res.Identity())
				}
			})
		}
	})

	t.Run("WithMutation", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		m := feature.Mutation[*Mutator]{
			Name: "test-mutation",
		}
		res, err := NewBuilder(deploy).
			WithMutation(m).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.Mutations, 1)
		assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
	})

	t.Run("WithCustomFieldApplicator", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		applied := false
		applicator := func(_ *appsv1.Deployment, _ *appsv1.Deployment) error {
			applied = true
			return nil
		}
		res, err := NewBuilder(deploy).
			WithCustomFieldApplicator(applicator).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.CustomFieldApplicator)
		_ = res.base.CustomFieldApplicator(nil, nil)
		assert.True(t, applied)
	})

	t.Run("WithFieldApplicationFlavor", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(deploy).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			WithFieldApplicationFlavor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.FieldFlavors, 1)
	})

	t.Run("WithCustomConvergeStatus", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		handler := func(_ concepts.ConvergingOperation, _ *appsv1.Deployment) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusUpdating}, nil
		}
		res, err := NewBuilder(deploy).
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
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy}, nil
		}
		res, err := NewBuilder(deploy).
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
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}
		res, err := NewBuilder(deploy).
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
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *Mutator) error {
			return errors.New("suspend error")
		}
		res, err := NewBuilder(deploy).
			WithCustomSuspendMutation(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.SuspendMutationHandler)
		err = res.base.SuspendMutationHandler(nil)
		assert.EqualError(t, err, "suspend error")
	})

	t.Run("WithCustomSuspendDeletionDecision", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		handler := func(_ *appsv1.Deployment) bool {
			return true
		}
		res, err := NewBuilder(deploy).
			WithCustomSuspendDeletionDecision(handler).
			Build()
		require.NoError(t, err)
		require.NotNil(t, res.base.DeleteOnSuspendHandler)
		assert.True(t, res.base.DeleteOnSuspendHandler(nil))
	})

	t.Run("WithDataExtractor", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		called := false
		extractor := func(_ appsv1.Deployment) error {
			called = true
			return nil
		}
		res, err := NewBuilder(deploy).
			WithDataExtractor(extractor).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 1)
		err = res.base.DataExtractors[0](&appsv1.Deployment{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("WithDataExtractor nil", func(t *testing.T) {
		t.Parallel()
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "test-ns",
			},
		}
		res, err := NewBuilder(deploy).
			WithDataExtractor(nil).
			Build()
		require.NoError(t, err)
		assert.Len(t, res.base.DataExtractors, 0)
	})
}
