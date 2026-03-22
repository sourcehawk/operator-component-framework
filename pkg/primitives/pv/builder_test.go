package pv

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pv          *corev1.PersistentVolume
		expectedErr string
	}{
		{
			name:        "nil persistent volume",
			pv:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "namespace set on cluster-scoped resource",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pv", Namespace: "should-not-be-set"},
			},
			expectedErr: "PersistentVolume is cluster-scoped: namespace must not be set",
		},
		{
			name: "valid persistent volume",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.pv).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "v1/PersistentVolume/test-pv", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	res, err := NewBuilder(pv).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomFieldApplicator(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	called := false
	applicator := func(_, _ *corev1.PersistentVolume) error {
		called = true
		return nil
	}
	res, err := NewBuilder(pv).
		WithCustomFieldApplicator(applicator).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.CustomFieldApplicator)
	_ = res.base.CustomFieldApplicator(nil, nil)
	assert.True(t, called)
}

func TestBuilder_WithFieldApplicationFlavor(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	res, err := NewBuilder(pv).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(nil). // nil must be ignored
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 1)
}

func TestBuilder_WithCustomOperationalStatus(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	handler := func(_ concepts.ConvergingOperation, _ *corev1.PersistentVolume) (concepts.OperationalStatusWithReason, error) {
		return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
	}
	res, err := NewBuilder(pv).
		WithCustomOperationalStatus(handler).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.OperationalStatusHandler)
	status, err := res.base.OperationalStatusHandler(concepts.ConvergingOperationNone, nil)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	called := false
	extractor := func(_ corev1.PersistentVolume) error {
		called = true
		return nil
	}
	res, err := NewBuilder(pv).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&corev1.PersistentVolume{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	res, err := NewBuilder(pv).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	res, err := NewBuilder(pv).
		WithDataExtractor(func(_ corev1.PersistentVolume) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&corev1.PersistentVolume{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
