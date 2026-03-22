package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestBindingSubjectsEditor_EnsureSubject_Append(t *testing.T) {
	var subjects []rbacv1.Subject
	e := NewBindingSubjectsEditor(&subjects)

	e.EnsureSubject(rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      "my-sa",
		Namespace: "default",
	})

	require.Len(t, subjects, 1)
	assert.Equal(t, "ServiceAccount", subjects[0].Kind)
	assert.Equal(t, "my-sa", subjects[0].Name)
	assert.Equal(t, "default", subjects[0].Namespace)
}

func TestBindingSubjectsEditor_EnsureSubject_Upsert(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default", APIGroup: ""},
	}
	e := NewBindingSubjectsEditor(&subjects)

	e.EnsureSubject(rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      "my-sa",
		Namespace: "default",
		APIGroup:  "rbac.authorization.k8s.io",
	})

	require.Len(t, subjects, 1)
	assert.Equal(t, "rbac.authorization.k8s.io", subjects[0].APIGroup)
}

func TestBindingSubjectsEditor_EnsureSubject_MultipleSubjects(t *testing.T) {
	var subjects []rbacv1.Subject
	e := NewBindingSubjectsEditor(&subjects)

	e.EnsureSubject(rbacv1.Subject{Kind: "ServiceAccount", Name: "sa-1", Namespace: "ns-1"})
	e.EnsureSubject(rbacv1.Subject{Kind: "User", Name: "admin", Namespace: ""})

	require.Len(t, subjects, 2)
	assert.Equal(t, "sa-1", subjects[0].Name)
	assert.Equal(t, "admin", subjects[1].Name)
}

func TestBindingSubjectsEditor_RemoveSubject(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "keep", Namespace: "default"},
		{Kind: "ServiceAccount", Name: "remove", Namespace: "default"},
		{Kind: "User", Name: "admin", Namespace: ""},
	}
	e := NewBindingSubjectsEditor(&subjects)

	e.RemoveSubject("ServiceAccount", "remove", "default")

	require.Len(t, subjects, 2)
	assert.Equal(t, "keep", subjects[0].Name)
	assert.Equal(t, "admin", subjects[1].Name)
}

func TestBindingSubjectsEditor_RemoveSubject_NotPresent(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "keep", Namespace: "default"},
	}
	e := NewBindingSubjectsEditor(&subjects)

	e.RemoveSubject("ServiceAccount", "missing", "default")

	require.Len(t, subjects, 1)
	assert.Equal(t, "keep", subjects[0].Name)
}

func TestBindingSubjectsEditor_RemoveSubject_EmptySlice(t *testing.T) {
	var subjects []rbacv1.Subject
	e := NewBindingSubjectsEditor(&subjects)

	e.RemoveSubject("ServiceAccount", "missing", "default")

	assert.Empty(t, subjects)
}

func TestBindingSubjectsEditor_Raw(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
	}
	e := NewBindingSubjectsEditor(&subjects)

	raw := e.Raw()
	require.NotNil(t, raw)
	assert.Same(t, &subjects, raw)
}

func TestBindingSubjectsEditor_Raw_NilSlice(t *testing.T) {
	var subjects []rbacv1.Subject
	e := NewBindingSubjectsEditor(&subjects)

	raw := e.Raw()
	require.NotNil(t, raw)
}
