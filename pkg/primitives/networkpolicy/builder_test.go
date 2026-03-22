package networkpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		np          *networkingv1.NetworkPolicy
		expectedErr string
	}{
		{
			name:        "nil networkpolicy",
			np:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			np: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			np: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-np"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid networkpolicy",
			np: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.np).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "networking.k8s.io/v1/NetworkPolicy/test-ns/test-np", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	res, err := NewBuilder(np).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomFieldApplicator(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	called := false
	applicator := func(_, _ *networkingv1.NetworkPolicy) error {
		called = true
		return nil
	}
	res, err := NewBuilder(np).
		WithCustomFieldApplicator(applicator).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.CustomFieldApplicator)
	_ = res.base.CustomFieldApplicator(nil, nil)
	assert.True(t, called)
}

func TestBuilder_WithFieldApplicationFlavor(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	res, err := NewBuilder(np).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(nil). // nil must be ignored
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 1)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	called := false
	extractor := func(_ networkingv1.NetworkPolicy) error {
		called = true
		return nil
	}
	res, err := NewBuilder(np).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&networkingv1.NetworkPolicy{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	res, err := NewBuilder(np).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-np", Namespace: "test-ns"},
	}
	res, err := NewBuilder(np).
		WithDataExtractor(func(_ networkingv1.NetworkPolicy) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&networkingv1.NetworkPolicy{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
