package clusterrole

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
		cr          *rbacv1.ClusterRole
		expectedErr string
	}{
		{
			name:        "nil clusterrole",
			cr:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name:        "empty name",
			cr:          &rbacv1.ClusterRole{},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "valid clusterrole",
			cr: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
			},
		},
		{
			name: "valid clusterrole without namespace",
			cr: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.cr).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/test-cr", res.Identity())
			}
		})
	}
}

func TestBuilder_Build_NoNamespaceRequired(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-scoped"},
	}
	res, err := NewBuilder(cr).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/cluster-scoped", res.Identity())
}

func TestBuilder_Build_NamespaceNotPersisted(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	_, err := NewBuilder(cr).Build()
	require.NoError(t, err)
	assert.Empty(t, cr.Namespace, "namespace should remain empty after Build")
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomFieldApplicator(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	called := false
	applicator := func(_, _ *rbacv1.ClusterRole) error {
		called = true
		return nil
	}
	res, err := NewBuilder(cr).
		WithCustomFieldApplicator(applicator).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.CustomFieldApplicator)
	_ = res.base.CustomFieldApplicator(nil, nil)
	assert.True(t, called)
}

func TestBuilder_WithFieldApplicationFlavor(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithFieldApplicationFlavor(PreserveExternalRules).
		WithFieldApplicationFlavor(nil). // nil must be ignored
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 1)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	called := false
	extractor := func(_ rbacv1.ClusterRole) error {
		called = true
		return nil
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&rbacv1.ClusterRole{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(func(_ rbacv1.ClusterRole) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&rbacv1.ClusterRole{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
