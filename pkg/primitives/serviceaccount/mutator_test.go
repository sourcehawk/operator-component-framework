package serviceaccount

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestSA() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "default",
		},
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", sa.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EnsureImagePullSecret ---

func TestMutator_EnsureImagePullSecret(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("my-registry")
	require.NoError(t, m.Apply())
	require.Len(t, sa.ImagePullSecrets, 1)
	assert.Equal(t, "my-registry", sa.ImagePullSecrets[0].Name)
}

func TestMutator_EnsureImagePullSecret_Idempotent(t *testing.T) {
	sa := newTestSA()
	sa.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "my-registry"}}
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("my-registry")
	require.NoError(t, m.Apply())
	assert.Len(t, sa.ImagePullSecrets, 1)
}

func TestMutator_EnsureImagePullSecret_Multiple(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("registry-a")
	m.EnsureImagePullSecret("registry-b")
	require.NoError(t, m.Apply())
	require.Len(t, sa.ImagePullSecrets, 2)
	assert.Equal(t, "registry-a", sa.ImagePullSecrets[0].Name)
	assert.Equal(t, "registry-b", sa.ImagePullSecrets[1].Name)
}

// --- RemoveImagePullSecret ---

func TestMutator_RemoveImagePullSecret(t *testing.T) {
	sa := newTestSA()
	sa.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "keep"},
		{Name: "remove"},
	}
	m := NewMutator(sa)
	m.BeginFeature()
	m.RemoveImagePullSecret("remove")
	require.NoError(t, m.Apply())
	require.Len(t, sa.ImagePullSecrets, 1)
	assert.Equal(t, "keep", sa.ImagePullSecrets[0].Name)
}

func TestMutator_RemoveImagePullSecret_NotPresent(t *testing.T) {
	sa := newTestSA()
	sa.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "keep"}}
	m := NewMutator(sa)
	m.BeginFeature()
	m.RemoveImagePullSecret("missing")
	require.NoError(t, m.Apply())
	assert.Len(t, sa.ImagePullSecrets, 1)
}

func TestMutator_RemoveImagePullSecret_Empty(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.RemoveImagePullSecret("missing")
	require.NoError(t, m.Apply())
	assert.Empty(t, sa.ImagePullSecrets)
}

// --- SetAutomountServiceAccountToken ---

func TestMutator_SetAutomountServiceAccountToken(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	v := true
	m.SetAutomountServiceAccountToken(&v)
	require.NoError(t, m.Apply())
	require.NotNil(t, sa.AutomountServiceAccountToken)
	assert.True(t, *sa.AutomountServiceAccountToken)
}

func TestMutator_SetAutomountServiceAccountToken_False(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	v := false
	m.SetAutomountServiceAccountToken(&v)
	require.NoError(t, m.Apply())
	require.NotNil(t, sa.AutomountServiceAccountToken)
	assert.False(t, *sa.AutomountServiceAccountToken)
}

func TestMutator_SetAutomountServiceAccountToken_Nil(t *testing.T) {
	v := true
	sa := newTestSA()
	sa.AutomountServiceAccountToken = &v
	m := NewMutator(sa)
	m.BeginFeature()
	m.SetAutomountServiceAccountToken(nil)
	require.NoError(t, m.Apply())
	assert.Nil(t, sa.AutomountServiceAccountToken)
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata → image pull secrets → automount.
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	v := false
	m.SetAutomountServiceAccountToken(&v)
	m.EnsureImagePullSecret("my-registry")
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", sa.Labels["order"])
	require.Len(t, sa.ImagePullSecrets, 1)
	assert.Equal(t, "my-registry", sa.ImagePullSecrets[0].Name)
	require.NotNil(t, sa.AutomountServiceAccountToken)
	assert.False(t, *sa.AutomountServiceAccountToken)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("feature1-registry")
	m.BeginFeature()
	m.EnsureImagePullSecret("feature2-registry")
	require.NoError(t, m.Apply())

	require.Len(t, sa.ImagePullSecrets, 2)
	assert.Equal(t, "feature1-registry", sa.ImagePullSecrets[0].Name)
	assert.Equal(t, "feature2-registry", sa.ImagePullSecrets[1].Name)
}

func TestMutator_MultipleFeatures_LaterObservesPrior(t *testing.T) {
	// Feature 2 removes a secret added by feature 1.
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("temp-registry")
	m.BeginFeature()
	m.RemoveImagePullSecret("temp-registry")
	require.NoError(t, m.Apply())

	assert.Empty(t, sa.ImagePullSecrets)
}

// --- Constructor and feature plan invariants ---

func TestNewMutator_InitializesNoPlan(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)

	assert.Empty(t, m.plans, "NewMutator must not create any plans")
	assert.Nil(t, m.active, "active plan must not be set")
}

func TestBeginFeature_AddsExactlyOnePlan(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)

	m.BeginFeature()
	require.Len(t, m.plans, 1, "BeginFeature must add exactly one plan")
	assert.Equal(t, &m.plans[0], m.active, "active must point to the new plan")

	m.BeginFeature()
	require.Len(t, m.plans, 2)
	assert.Equal(t, &m.plans[1], m.active)
}

func TestBeginFeature_IsolatesFeaturePlans(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)

	// Record a mutation in the first feature plan
	m.BeginFeature()
	m.EnsureImagePullSecret("f0-registry")

	// Start a new feature and record a different mutation
	m.BeginFeature()
	m.EnsureImagePullSecret("f1-registry")

	// The initial plan should have exactly one image pull secret edit
	assert.Len(t, m.plans[0].imagePullSecretEdits, 1, "initial plan should have one edit")
	// The second plan should also have exactly one image pull secret edit
	assert.Len(t, m.plans[1].imagePullSecretEdits, 1, "second plan should have one edit")
}

func TestMutator_SingleFeature_PlanCount(t *testing.T) {
	sa := newTestSA()
	m := NewMutator(sa)
	m.BeginFeature()
	m.EnsureImagePullSecret("my-registry")

	require.NoError(t, m.Apply())
	assert.Len(t, m.plans, 1, "no extra plans should be created during Apply")
	require.Len(t, sa.ImagePullSecrets, 1)
	assert.Equal(t, "my-registry", sa.ImagePullSecrets[0].Name)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
