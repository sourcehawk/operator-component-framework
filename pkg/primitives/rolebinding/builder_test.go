package rolebinding

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
		rb          *rbacv1.RoleBinding
		expectedErr string
	}{
		{
			name:        "nil rolebinding",
			rb:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid rolebinding",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "my-role",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.rb).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "rbac.authorization.k8s.io/v1/RoleBinding/test-ns/test-rb", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(rb).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomFieldApplicator(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	called := false
	applicator := func(_, _ *rbacv1.RoleBinding) error {
		called = true
		return nil
	}
	res, err := NewBuilder(rb).
		WithCustomFieldApplicator(applicator).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.CustomFieldApplicator)
	_ = res.base.CustomFieldApplicator(nil, nil)
	assert.True(t, called)
}

func TestBuilder_WithFieldApplicationFlavor(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(rb).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(nil). // nil must be ignored
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.FieldFlavors, 1)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	called := false
	extractor := func(_ rbacv1.RoleBinding) error {
		called = true
		return nil
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&rbacv1.RoleBinding{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(func(_ rbacv1.RoleBinding) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&rbacv1.RoleBinding{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}
