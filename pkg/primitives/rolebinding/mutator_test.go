package rolebinding

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestRB() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rb",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "my-role",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", rb.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditSubjects ---

func TestMutator_EditSubjects_EnsureSubject(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureSubject(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "my-sa",
			Namespace: "default",
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "my-sa", rb.Subjects[0].Name)
}

func TestMutator_EditSubjects_RemoveSubject(t *testing.T) {
	rb := newTestRB()
	rb.Subjects = []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "keep", Namespace: "default"},
		{Kind: "ServiceAccount", Name: "remove", Namespace: "default"},
	}
	m := NewMutator(rb)
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.RemoveSubject("ServiceAccount", "remove", "default")
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "keep", rb.Subjects[0].Name)
}

func TestMutator_EditSubjects_RawAccess(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		raw := e.Raw()
		*raw = append(*raw, rbacv1.Subject{
			Kind: "User",
			Name: "admin",
		})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "admin", rb.Subjects[0].Name)
}

func TestMutator_EditSubjects_Nil(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditSubjects(nil)
	assert.NoError(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before subject edits.
	rb := newTestRB()
	m := NewMutator(rb)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureSubject(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "my-sa",
			Namespace: "default",
		})
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", rb.Labels["order"])
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "my-sa", rb.Subjects[0].Name)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureSubject(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "sa-1",
			Namespace: "default",
		})
		return nil
	})
	m.beginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureSubject(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "sa-2",
			Namespace: "default",
		})
		return nil
	})
	require.NoError(t, m.Apply())

	require.Len(t, rb.Subjects, 2)
	assert.Equal(t, "sa-1", rb.Subjects[0].Name)
	assert.Equal(t, "sa-2", rb.Subjects[1].Name)
}

func TestMutator_EditSubjects_ErrorPropagated(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditSubjects(func(_ *editors.BindingSubjectsEditor) error {
		return assert.AnError
	})
	err := m.Apply()
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestMutator_EditObjectMetadata_ErrorPropagated(t *testing.T) {
	rb := newTestRB()
	m := NewMutator(rb)
	m.EditObjectMetadata(func(_ *editors.ObjectMetaEditor) error {
		return assert.AnError
	})
	err := m.Apply()
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
