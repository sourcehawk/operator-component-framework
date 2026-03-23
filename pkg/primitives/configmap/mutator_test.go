package configmap

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func newTestCM(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: data,
	}
}

func parseYAML(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(s), &out))
	return out
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", cm.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditData (raw escape hatch) ---

func TestMutator_EditData_RawAccess(t *testing.T) {
	cm := newTestCM(map[string]string{"existing": "keep"})
	m := NewMutator(cm)
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		raw := e.Raw()
		raw["new"] = "added"
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "keep", cm.Data["existing"])
	assert.Equal(t, "added", cm.Data["new"])
}

func TestMutator_EditData_Nil(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.EditData(nil)
	assert.NoError(t, m.Apply())
}

// --- SetEntry ---

func TestMutator_SetEntry(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.SetEntry("key", "value")
	require.NoError(t, m.Apply())
	assert.Equal(t, "value", cm.Data["key"])
}

func TestMutator_SetEntry_Overwrites(t *testing.T) {
	cm := newTestCM(map[string]string{"key": "old"})
	m := NewMutator(cm)
	m.SetEntry("key", "new")
	require.NoError(t, m.Apply())
	assert.Equal(t, "new", cm.Data["key"])
}

// --- RemoveEntry ---

func TestMutator_RemoveEntry(t *testing.T) {
	cm := newTestCM(map[string]string{"key": "value", "other": "keep"})
	m := NewMutator(cm)
	m.RemoveEntry("key")
	require.NoError(t, m.Apply())
	assert.NotContains(t, cm.Data, "key")
	assert.Equal(t, "keep", cm.Data["other"])
}

func TestMutator_RemoveEntry_NotPresent(t *testing.T) {
	cm := newTestCM(map[string]string{"other": "keep"})
	m := NewMutator(cm)
	m.RemoveEntry("missing")
	require.NoError(t, m.Apply())
	assert.Equal(t, "keep", cm.Data["other"])
}

// --- MergeYAML ---

func TestMutator_MergeYAML_NewKey(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.MergeYAML("config.yaml", "key: value\n")
	require.NoError(t, m.Apply())
	assert.NotEmpty(t, cm.Data["config.yaml"])
}

func TestMutator_MergeYAML_MergesKeys(t *testing.T) {
	cm := newTestCM(map[string]string{"config.yaml": "a: 1\nb: 2\n"})
	m := NewMutator(cm)
	m.MergeYAML("config.yaml", "b: 99\nc: 3\n")
	require.NoError(t, m.Apply())

	out := parseYAML(t, cm.Data["config.yaml"])
	assert.EqualValues(t, 1, out["a"])
	assert.EqualValues(t, 99, out["b"])
	assert.EqualValues(t, 3, out["c"])
}

func TestMutator_MergeYAML_RecursiveMerge(t *testing.T) {
	cm := newTestCM(map[string]string{"config.yaml": "parent:\n  a: 1\n  b: 2\n"})
	m := NewMutator(cm)
	m.MergeYAML("config.yaml", "parent:\n  b: 99\n  c: 3\n")
	require.NoError(t, m.Apply())

	out := parseYAML(t, cm.Data["config.yaml"])
	parent, ok := out["parent"].(map[string]interface{})
	require.True(t, ok, "expected parent to be a map")
	assert.EqualValues(t, 1, parent["a"])
	assert.EqualValues(t, 99, parent["b"])
	assert.EqualValues(t, 3, parent["c"])
}

func TestMutator_MergeYAML_PatchWinsForNonMap(t *testing.T) {
	cm := newTestCM(map[string]string{"key": "scalar-value\n"})
	m := NewMutator(cm)
	m.MergeYAML("key", "new-value\n")
	require.NoError(t, m.Apply())
	assert.NotEqual(t, "scalar-value\n", cm.Data["key"])
}

func TestMutator_MergeYAML_Composable(t *testing.T) {
	// Two separate features both contribute to the same key.
	cm := newTestCM(map[string]string{"config.yaml": "a: 1\n"})
	m := NewMutator(cm)
	m.MergeYAML("config.yaml", "b: 2\n")
	m.beginFeature()
	m.MergeYAML("config.yaml", "c: 3\n")
	require.NoError(t, m.Apply())

	out := parseYAML(t, cm.Data["config.yaml"])
	assert.EqualValues(t, 1, out["a"])
	assert.EqualValues(t, 2, out["b"])
	assert.EqualValues(t, 3, out["c"])
}

func TestMutator_MergeYAML_InvalidBase(t *testing.T) {
	cm := newTestCM(map[string]string{"key": "{"})
	m := NewMutator(cm)
	m.MergeYAML("key", "a: 1\n")
	assert.Error(t, m.Apply())
}

func TestMutator_MergeYAML_InvalidPatch(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.MergeYAML("key", "{")
	assert.Error(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before data edits.
	cm := newTestCM(map[string]string{"cfg": "base: 0\n"})
	m := NewMutator(cm)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.MergeYAML("cfg", "merged: 1\n")
	m.SetEntry("direct", "yes")
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", cm.Labels["order"])
	assert.Equal(t, "yes", cm.Data["direct"])

	out := parseYAML(t, cm.Data["cfg"])
	assert.EqualValues(t, 0, out["base"])
	assert.EqualValues(t, 1, out["merged"])
}

func TestMutator_MultipleFeatures(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.SetEntry("feature1", "on")
	m.beginFeature()
	m.SetEntry("feature2", "on")
	require.NoError(t, m.Apply())

	assert.Equal(t, "on", cm.Data["feature1"])
	assert.Equal(t, "on", cm.Data["feature2"])
}

// --- Binary data ---

func TestMutator_SetBinary(t *testing.T) {
	cm := newTestCM(nil)
	m := NewMutator(cm)
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		e.SetBinary("cert.pem", []byte("binary-content"))
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, []byte("binary-content"), cm.BinaryData["cert.pem"])
}

func TestMutator_RemoveBinary(t *testing.T) {
	cm := newTestCM(nil)
	cm.BinaryData = map[string][]byte{"cert.pem": []byte("data"), "other": []byte("keep")}
	m := NewMutator(cm)
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		e.RemoveBinary("cert.pem")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.NotContains(t, cm.BinaryData, "cert.pem")
	assert.Equal(t, []byte("keep"), cm.BinaryData["other"])
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
