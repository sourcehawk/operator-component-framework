package clusterrolebinding

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestCRB() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-crb",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", crb.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditSubjects ---

func TestMutator_EditSubjects(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("my-sa", "default")
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", crb.Subjects[0].Kind)
	assert.Equal(t, "my-sa", crb.Subjects[0].Name)
	assert.Equal(t, "default", crb.Subjects[0].Namespace)
}

func TestMutator_EditSubjects_Nil(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(nil)
	assert.NoError(t, m.Apply())
}

func TestMutator_EditSubjects_Add(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.Add(rbacv1.Subject{Kind: "User", Name: "alice", APIGroup: "rbac.authorization.k8s.io"})
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "alice", crb.Subjects[0].Name)
}

func TestMutator_EditSubjects_Remove(t *testing.T) {
	crb := newTestCRB()
	crb.Subjects = []rbacv1.Subject{
		{Kind: "User", Name: "alice"},
		{Kind: "User", Name: "bob"},
	}
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.Remove("User", "alice", "")
		return nil
	})
	require.NoError(t, m.Apply())
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "bob", crb.Subjects[0].Name)
}

func TestMutator_EditSubjects_Error(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(_ *editors.BindingSubjectsEditor) error {
		return assert.AnError
	})
	assert.Error(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before subject edits.
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa1", "ns1")
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", crb.Labels["order"])
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "sa1", crb.Subjects[0].Name)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa1", "ns1")
		return nil
	})
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa2", "ns2")
		return nil
	})
	require.NoError(t, m.Apply())

	require.Len(t, crb.Subjects, 2)
	assert.Equal(t, "sa1", crb.Subjects[0].Name)
	assert.Equal(t, "sa2", crb.Subjects[1].Name)
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesNoPlan(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)

	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)

	// Record a mutation in the first feature plan
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa0", "ns0")
		return nil
	})

	// Start a new feature and record a different mutation
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa1", "ns1")
		return nil
	})

	require.Len(t, m.plans, 2)
	assert.Len(t, m.plans[0].subjectEdits, 1, "first plan must have one subject edit")
	assert.Len(t, m.plans[1].subjectEdits, 1, "second plan must have one subject edit")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	crb := newTestCRB()
	m := NewMutator(crb)
	m.BeginFeature()
	m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
		e.EnsureServiceAccount("sa1", "ns1")
		return nil
	})

	require.NoError(t, m.Apply())
	require.Len(t, m.plans, 1, "only one plan must exist when no additional BeginFeature is called")
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
