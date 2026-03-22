package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestBindingSubjectsEditor_Raw(t *testing.T) {
	t.Run("initialises nil slice", func(t *testing.T) {
		var subjects []rbacv1.Subject
		e := NewBindingSubjectsEditor(&subjects)
		raw := e.Raw()
		require.NotNil(t, raw)
		assert.Empty(t, *raw)
	})

	t.Run("returns existing slice", func(t *testing.T) {
		subjects := []rbacv1.Subject{{Kind: "User", Name: "alice"}}
		e := NewBindingSubjectsEditor(&subjects)
		raw := e.Raw()
		assert.Len(t, *raw, 1)
		assert.Equal(t, "alice", (*raw)[0].Name)
	})

	t.Run("append through pointer propagates", func(t *testing.T) {
		var subjects []rbacv1.Subject
		e := NewBindingSubjectsEditor(&subjects)
		raw := e.Raw()
		*raw = append(*raw, rbacv1.Subject{Kind: "User", Name: "bob"})
		assert.Len(t, subjects, 1)
		assert.Equal(t, "bob", subjects[0].Name)
	})
}

func TestBindingSubjectsEditor_Add(t *testing.T) {
	var subjects []rbacv1.Subject
	e := NewBindingSubjectsEditor(&subjects)
	e.Add(rbacv1.Subject{Kind: "User", Name: "alice"})
	e.Add(rbacv1.Subject{Kind: "Group", Name: "devs"})
	assert.Len(t, subjects, 2)
	assert.Equal(t, "alice", subjects[0].Name)
	assert.Equal(t, "devs", subjects[1].Name)
}

func TestBindingSubjectsEditor_Remove(t *testing.T) {
	t.Run("removes matching subject", func(t *testing.T) {
		subjects := []rbacv1.Subject{
			{Kind: "User", Name: "alice", Namespace: "default"},
			{Kind: "User", Name: "bob", Namespace: "default"},
		}
		e := NewBindingSubjectsEditor(&subjects)
		e.Remove("User", "alice", "default")
		assert.Len(t, subjects, 1)
		assert.Equal(t, "bob", subjects[0].Name)
	})

	t.Run("no-op when not found", func(t *testing.T) {
		subjects := []rbacv1.Subject{
			{Kind: "User", Name: "alice", Namespace: "default"},
		}
		e := NewBindingSubjectsEditor(&subjects)
		e.Remove("User", "nobody", "default")
		assert.Len(t, subjects, 1)
	})

	t.Run("no-op on nil slice", func(t *testing.T) {
		var subjects []rbacv1.Subject
		e := NewBindingSubjectsEditor(&subjects)
		e.Remove("User", "alice", "default")
		assert.Nil(t, subjects)
	})
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
