package clusterrolebinding

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidCRB() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-crb",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "User", Name: "alice"},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidCRB()).Build()
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRoleBinding/test-crb", res.Identity())
}

func TestResource_Object(t *testing.T) {
	crb := newValidCRB()
	res, err := NewBuilder(crb).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*rbacv1.ClusterRoleBinding)
	require.True(t, ok)
	assert.Equal(t, crb.Name, got.Name)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-crb", crb.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidCRB()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got, ok := obj.(*rbacv1.ClusterRoleBinding)
	require.True(t, ok)
	assert.Equal(t, "test-role", got.RoleRef.Name)
	assert.Len(t, got.Subjects, 1)
	assert.Equal(t, "alice", got.Subjects[0].Name)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidCRB()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name: "add-subject",
			Mutate: func(m *Mutator) error {
				m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
					e.EnsureServiceAccount("extra-sa", "default")
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

	got, ok := obj.(*rbacv1.ClusterRoleBinding)
	require.True(t, ok)
	assert.Len(t, got.Subjects, 2)
}

func TestMutator_EditSubjects_WithoutBeginFeature(t *testing.T) {
	crb := newValidCRB()
	m := NewMutator(crb)

	// EditSubjects should not panic even without a prior BeginFeature call.
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("lazy-sa", "default")
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Len(t, crb.Subjects, 2)
	assert.Equal(t, "lazy-sa", crb.Subjects[1].Name)
}
