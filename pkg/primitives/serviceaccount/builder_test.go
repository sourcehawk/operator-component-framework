package serviceaccount

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sa          *corev1.ServiceAccount
		expectedErr string
	}{
		{
			name:        "nil serviceaccount",
			sa:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sa"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid serviceaccount",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.sa).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "v1/ServiceAccount/test-ns/test-sa", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	res, err := NewBuilder(sa).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomFieldApplicator(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	called := false
	applicator := func(_, _ *corev1.ServiceAccount) error {
		called = true
		return nil
	}
	res, err := NewBuilder(sa).
		WithCustomFieldApplicator(applicator).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.CustomFieldApplicator)
	_ = res.base.CustomFieldApplicator(nil, nil)
	assert.True(t, called)
}

func TestBuilder_WithFieldApplicationFlavor(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	res, err := NewBuilder(sa).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(nil). // nil must be ignored
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 1)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	called := false
	extractor := func(_ corev1.ServiceAccount) error {
		called = true
		return nil
	}
	res, err := NewBuilder(sa).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&corev1.ServiceAccount{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	res, err := NewBuilder(sa).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "test-ns"},
	}
	res, err := NewBuilder(sa).
		WithDataExtractor(func(_ corev1.ServiceAccount) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&corev1.ServiceAccount{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
