package secret

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: data,
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", s.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditData ---

func TestMutator_EditData_RawAccess(t *testing.T) {
	s := newTestSecret(map[string][]byte{"existing": []byte("keep")})
	m := NewMutator(s)
	m.EditData(func(e *editors.SecretDataEditor) error {
		raw := e.Raw()
		raw["new"] = []byte("added")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, []byte("keep"), s.Data["existing"])
	assert.Equal(t, []byte("added"), s.Data["new"])
}

func TestMutator_EditData_Nil(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.EditData(nil)
	assert.NoError(t, m.Apply())
}

// --- SetData ---

func TestMutator_SetData(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.SetData("key", []byte("value"))
	require.NoError(t, m.Apply())
	assert.Equal(t, []byte("value"), s.Data["key"])
}

func TestMutator_SetData_Overwrites(t *testing.T) {
	s := newTestSecret(map[string][]byte{"key": []byte("old")})
	m := NewMutator(s)
	m.SetData("key", []byte("new"))
	require.NoError(t, m.Apply())
	assert.Equal(t, []byte("new"), s.Data["key"])
}

// --- RemoveData ---

func TestMutator_RemoveData(t *testing.T) {
	s := newTestSecret(map[string][]byte{"key": []byte("value"), "other": []byte("keep")})
	m := NewMutator(s)
	m.RemoveData("key")
	require.NoError(t, m.Apply())
	assert.NotContains(t, s.Data, "key")
	assert.Equal(t, []byte("keep"), s.Data["other"])
}

func TestMutator_RemoveData_NotPresent(t *testing.T) {
	s := newTestSecret(map[string][]byte{"other": []byte("keep")})
	m := NewMutator(s)
	m.RemoveData("missing")
	require.NoError(t, m.Apply())
	assert.Equal(t, []byte("keep"), s.Data["other"])
}

// --- SetStringData ---

func TestMutator_SetStringData(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.SetStringData("key", "value")
	require.NoError(t, m.Apply())
	assert.Equal(t, "value", s.StringData["key"])
}

// --- RemoveStringData ---

func TestMutator_RemoveStringData(t *testing.T) {
	s := newTestSecret(nil)
	s.StringData = map[string]string{"key": "value", "other": "keep"}
	m := NewMutator(s)
	m.RemoveStringData("key")
	require.NoError(t, m.Apply())
	assert.NotContains(t, s.StringData, "key")
	assert.Equal(t, "keep", s.StringData["other"])
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before data edits.
	s := newTestSecret(nil)
	m := NewMutator(s)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.SetData("direct", []byte("yes"))
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", s.Labels["order"])
	assert.Equal(t, []byte("yes"), s.Data["direct"])
}

func TestMutator_MultipleFeatures(t *testing.T) {
	s := newTestSecret(nil)
	m := NewMutator(s)
	m.SetData("feature1", []byte("on"))
	m.BeginFeature()
	m.SetData("feature2", []byte("on"))
	require.NoError(t, m.Apply())

	assert.Equal(t, []byte("on"), s.Data["feature1"])
	assert.Equal(t, []byte("on"), s.Data["feature2"])
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
