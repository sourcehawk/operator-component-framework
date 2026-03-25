package pdb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pdb         *policyv1.PodDisruptionBudget
		expectedErr string
	}{
		{
			name:        "nil pdb",
			pdb:         nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pdb"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid pdb",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.pdb).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "policy/v1/PodDisruptionBudget/test-ns/test-pdb", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	p := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(p).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	p := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
	}
	called := false
	extractor := func(_ policyv1.PodDisruptionBudget) error {
		called = true
		return nil
	}
	res, err := NewBuilder(p).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&policyv1.PodDisruptionBudget{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	p := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(p).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	p := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(p).
		WithDataExtractor(func(_ policyv1.PodDisruptionBudget) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&policyv1.PodDisruptionBudget{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
