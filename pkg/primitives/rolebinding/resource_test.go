package rolebinding

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidRB() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "test-ns",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "my-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "sa", Namespace: "test-ns"},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidRB()).Build()
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/RoleBinding/test-ns/test-rb", res.Identity())
}

func TestResource_Object(t *testing.T) {
	rb := newValidRB()
	res, err := NewBuilder(rb).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*rbacv1.RoleBinding)
	require.True(t, ok)
	assert.Equal(t, rb.Name, got.Name)
	assert.Equal(t, rb.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-rb", rb.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidRB()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.RoleBinding)
	assert.Equal(t, "sa", got.Subjects[0].Name)
	assert.Equal(t, "my-role", got.RoleRef.Name)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidRB()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-subject",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
					e.EnsureSubject(rbacv1.Subject{
						Kind:      "ServiceAccount",
						Name:      "from-mutation",
						Namespace: "test-ns",
					})
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.RoleBinding)
	assert.Equal(t, "sa", got.Subjects[0].Name)
	assert.Equal(t, "from-mutation", got.Subjects[1].Name)
}

func TestResource_ExtractData(t *testing.T) {
	rb := newValidRB()

	var extracted string
	res, err := NewBuilder(rb).
		WithDataExtractor(func(r rbacv1.RoleBinding) error {
			extracted = r.Subjects[0].Name
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "sa", extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidRB()).
		WithDataExtractor(func(_ rbacv1.RoleBinding) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}
