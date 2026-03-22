package clusterrolebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultFieldApplicator(t *testing.T) {
	t.Run("create: applies desired roleRef when current has no ResourceVersion", func(t *testing.T) {
		current := &rbacv1.ClusterRoleBinding{}
		desired := &rbacv1.ClusterRoleBinding{
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "desired-role",
			},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "alice"},
			},
		}

		err := DefaultFieldApplicator(current, desired)
		require.NoError(t, err)
		assert.Equal(t, "desired-role", current.RoleRef.Name)
		assert.Len(t, current.Subjects, 1)
	})

	t.Run("update: preserves current roleRef when ResourceVersion is set", func(t *testing.T) {
		current := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "12345",
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "original-role",
			},
		}
		desired := &rbacv1.ClusterRoleBinding{
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "new-role",
			},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "bob"},
			},
		}

		err := DefaultFieldApplicator(current, desired)
		require.NoError(t, err)
		assert.Equal(t, "original-role", current.RoleRef.Name, "roleRef must be preserved on update")
		assert.Equal(t, "12345", current.ResourceVersion, "ResourceVersion must be preserved on update")
		assert.Len(t, current.Subjects, 1)
		assert.Equal(t, "bob", current.Subjects[0].Name)
	})

	t.Run("update: preserves server-managed and shared-controller fields", func(t *testing.T) {
		current := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "12345",
				UID:             "abc-def",
				Generation:      3,
				OwnerReferences: []metav1.OwnerReference{
					{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
				},
				Finalizers: []string{"finalizer.example.com"},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "original-role",
			},
		}
		desired := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "test"},
			},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "alice"},
			},
		}

		err := DefaultFieldApplicator(current, desired)
		require.NoError(t, err)

		// Desired fields are applied
		assert.Equal(t, "test", current.Labels["app"])
		assert.Len(t, current.Subjects, 1)

		// Server-managed fields are preserved
		assert.Equal(t, "12345", current.ResourceVersion)
		assert.Equal(t, "abc-def", string(current.UID))
		assert.Equal(t, int64(3), current.Generation)

		// Shared-controller fields are preserved
		assert.Len(t, current.OwnerReferences, 1)
		assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
		assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)

		// roleRef is preserved on update
		assert.Equal(t, "original-role", current.RoleRef.Name)
	})

	t.Run("does not mutate desired", func(t *testing.T) {
		current := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			RoleRef: rbacv1.RoleRef{Name: "current-role"},
		}
		desired := &rbacv1.ClusterRoleBinding{
			RoleRef: rbacv1.RoleRef{Name: "desired-role"},
		}

		err := DefaultFieldApplicator(current, desired)
		require.NoError(t, err)
		assert.Equal(t, "desired-role", desired.RoleRef.Name, "desired must not be modified")
	})
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "12345",
			UID:             "abc-def",
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "original-role",
		},
	}
	desired := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app": "test"},
		},
		Subjects: []rbacv1.Subject{
			{Kind: "User", Name: "alice"},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "test", current.Labels["app"])
	assert.Len(t, current.Subjects, 1)

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)

	// roleRef is preserved on update (immutable in RBAC API)
	assert.Equal(t, "original-role", current.RoleRef.Name)
}
