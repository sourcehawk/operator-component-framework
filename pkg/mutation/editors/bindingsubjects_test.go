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

func TestBindingSubjectsEditor_EnsureServiceAccount(t *testing.T) {
	t.Run("adds new service account", func(t *testing.T) {
		var subjects []rbacv1.Subject
		e := NewBindingSubjectsEditor(&subjects)
		e.EnsureServiceAccount("my-sa", "default")
		require.Len(t, subjects, 1)
		assert.Equal(t, "ServiceAccount", subjects[0].Kind)
		assert.Equal(t, "my-sa", subjects[0].Name)
		assert.Equal(t, "default", subjects[0].Namespace)
	})

	t.Run("no-op when already present", func(t *testing.T) {
		subjects := []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
		}
		e := NewBindingSubjectsEditor(&subjects)
		e.EnsureServiceAccount("my-sa", "default")
		assert.Len(t, subjects, 1)
	})
}

func TestBindingSubjectsEditor_RemoveServiceAccount(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
		{Kind: "User", Name: "alice"},
	}
	e := NewBindingSubjectsEditor(&subjects)
	e.RemoveServiceAccount("my-sa", "default")
	assert.Len(t, subjects, 1)
	assert.Equal(t, "alice", subjects[0].Name)
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

func TestBindingSubjectsEditor_NilPointer(t *testing.T) {
	e := NewBindingSubjectsEditor(nil)

	// Should not panic; operations work on the internal slice.
	e.EnsureSubject(rbacv1.Subject{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"})
	e.RemoveSubject("ServiceAccount", "sa", "ns")

	raw := e.Raw()
	require.NotNil(t, raw)
	assert.Empty(t, *raw)
}
