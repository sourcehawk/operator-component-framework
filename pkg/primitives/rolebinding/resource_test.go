package rolebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultFieldApplicator_Create(t *testing.T) {
	// When current has no ResourceVersion (new object), desired roleRef is applied.
	current := &rbacv1.RoleBinding{}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "test-ns",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "desired-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "sa", Namespace: "test-ns"},
		},
	}

	require.NoError(t, DefaultFieldApplicator(current, desired))

	assert.Equal(t, "test-rb", current.Name)
	assert.Equal(t, "desired-role", current.RoleRef.Name)
	assert.Equal(t, "Role", current.RoleRef.Kind)
	assert.Len(t, current.Subjects, 1)
}

func TestDefaultFieldApplicator_Update_PreservesLiveRoleRef(t *testing.T) {
	// When current has a ResourceVersion (existing object), the live roleRef
	// is preserved regardless of what desired declares.
	current := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rb",
			Namespace:       "test-ns",
			ResourceVersion: "12345",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "live-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "old-sa", Namespace: "test-ns"},
		},
	}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "test-ns",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "desired-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "new-sa", Namespace: "test-ns"},
		},
	}

	require.NoError(t, DefaultFieldApplicator(current, desired))

	// roleRef must be the live one, not the desired one.
	assert.Equal(t, "live-role", current.RoleRef.Name)
	assert.Equal(t, "ClusterRole", current.RoleRef.Kind)
	// Other fields should come from desired.
	assert.Len(t, current.Subjects, 1)
	assert.Equal(t, "new-sa", current.Subjects[0].Name)
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rb",
			Namespace:       "test-ns",
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
			Name:     "live-role",
		},
	}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "desired-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "sa", Namespace: "test-ns"},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "test", current.Labels["app"])
	assert.Len(t, current.Subjects, 1)
	assert.Equal(t, "sa", current.Subjects[0].Name)

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)

	// roleRef is preserved from live object
	assert.Equal(t, "live-role", current.RoleRef.Name)
	assert.Equal(t, "ClusterRole", current.RoleRef.Kind)
}

func TestDefaultFieldApplicator_DeepCopiesDesired(t *testing.T) {
	// Mutations to current after application must not affect desired.
	current := &rbacv1.RoleBinding{}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "test-ns",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "sa", Namespace: "test-ns"},
		},
	}

	require.NoError(t, DefaultFieldApplicator(current, desired))

	current.Subjects[0].Name = "mutated"
	assert.Equal(t, "sa", desired.Subjects[0].Name)
}
