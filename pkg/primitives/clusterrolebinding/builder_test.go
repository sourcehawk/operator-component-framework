package clusterrolebinding

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		crb         *rbacv1.ClusterRoleBinding
		expectedErr string
	}{
		{
			name:        "nil clusterrolebinding",
			crb:         nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			crb: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "non-empty namespace",
			crb: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-crb", Namespace: "default"},
			},
			expectedErr: "cluster-scoped object must not have a namespace",
		},
		{
			name: "valid clusterrolebinding",
			crb: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.crb).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRoleBinding/test-crb", res.Identity())
			}
		})
	}
}

func TestBuilder_Build_NoNamespaceRequired(t *testing.T) {
	t.Parallel()
	// ClusterRoleBinding is cluster-scoped; no namespace is required.
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
	}
	res, err := NewBuilder(crb).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
	}
	res, err := NewBuilder(crb).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
	}
	called := false
	extractor := func(_ rbacv1.ClusterRoleBinding) error {
		called = true
		return nil
	}
	res, err := NewBuilder(crb).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&rbacv1.ClusterRoleBinding{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
	}
	res, err := NewBuilder(crb).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-crb"},
	}
	res, err := NewBuilder(crb).
		WithDataExtractor(func(_ rbacv1.ClusterRoleBinding) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&rbacv1.ClusterRoleBinding{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
