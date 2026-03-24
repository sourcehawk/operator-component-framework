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

	current := &rbacv1.RoleBinding{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "sa", current.Subjects[0].Name)
	assert.Equal(t, "my-role", current.RoleRef.Name)
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

	current := &rbacv1.RoleBinding{}
	require.NoError(t, res.Mutate(current))

	assert.Equal(t, "sa", current.Subjects[0].Name)
	assert.Equal(t, "from-mutation", current.Subjects[1].Name)
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidRB()

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *rbacv1.RoleBinding) error {
			applicatorCalled = true
			current.Subjects = d.Subjects
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &rbacv1.RoleBinding{}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	assert.Len(t, current.Subjects, 1)
	assert.Equal(t, "sa", current.Subjects[0].Name)
}

func TestResource_Mutate_CustomFieldApplicator_Error(t *testing.T) {
	res, err := NewBuilder(newValidRB()).
		WithCustomFieldApplicator(func(_, _ *rbacv1.RoleBinding) error {
			return errors.New("applicator error")
		}).
		Build()
	require.NoError(t, err)

	err = res.Mutate(&rbacv1.RoleBinding{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applicator error")
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
