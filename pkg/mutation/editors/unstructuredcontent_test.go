package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SetNestedField ---

func TestUnstructuredContentEditor_SetNestedField(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedField("hello", "spec", "message"))
	assert.Equal(t, "hello", content["spec"].(map[string]interface{})["message"])
}

func TestUnstructuredContentEditor_SetNestedField_CreatesIntermediateMaps(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedField(int64(3), "spec", "deep", "replicas"))
	deep := content["spec"].(map[string]interface{})["deep"].(map[string]interface{})
	assert.Equal(t, int64(3), deep["replicas"])
}

func TestUnstructuredContentEditor_SetNestedField_Overwrites(t *testing.T) {
	content := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(1),
		},
	}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedField(int64(5), "spec", "replicas"))
	assert.Equal(t, int64(5), content["spec"].(map[string]interface{})["replicas"])
}

// --- RemoveNestedField ---

func TestUnstructuredContentEditor_RemoveNestedField(t *testing.T) {
	content := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"paused":   true,
		},
	}
	e := NewUnstructuredContentEditor(content)
	e.RemoveNestedField("spec", "paused")
	spec := content["spec"].(map[string]interface{})
	assert.NotContains(t, spec, "paused")
	assert.Equal(t, int64(3), spec["replicas"])
}

func TestUnstructuredContentEditor_RemoveNestedField_MissingPath(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	e.RemoveNestedField("spec", "nonexistent")
}

// --- SetNestedString ---

func TestUnstructuredContentEditor_SetNestedString(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedString("nginx", "spec", "image"))
	assert.Equal(t, "nginx", content["spec"].(map[string]interface{})["image"])
}

// --- SetNestedBool ---

func TestUnstructuredContentEditor_SetNestedBool(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedBool(true, "spec", "suspended"))
	assert.Equal(t, true, content["spec"].(map[string]interface{})["suspended"])
}

// --- SetNestedInt64 ---

func TestUnstructuredContentEditor_SetNestedInt64(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedInt64(42, "spec", "count"))
	assert.Equal(t, int64(42), content["spec"].(map[string]interface{})["count"])
}

// --- SetNestedFloat64 ---

func TestUnstructuredContentEditor_SetNestedFloat64(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.SetNestedFloat64(3.14, "spec", "ratio"))
	assert.Equal(t, 3.14, content["spec"].(map[string]interface{})["ratio"])
}

// --- SetNestedStringMap ---

func TestUnstructuredContentEditor_SetNestedStringMap(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	m := map[string]string{"app": "web", "env": "prod"}
	require.NoError(t, e.SetNestedStringMap(m, "metadata", "labels"))
	labels := content["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	assert.Equal(t, "web", labels["app"])
	assert.Equal(t, "prod", labels["env"])
}

// --- EnsureNestedStringMapEntry ---

func TestUnstructuredContentEditor_EnsureNestedStringMapEntry_NewMap(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.EnsureNestedStringMapEntry("app", "web", "metadata", "labels"))
	labels := content["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	assert.Equal(t, "web", labels["app"])
}

func TestUnstructuredContentEditor_EnsureNestedStringMapEntry_ExistingMap(t *testing.T) {
	content := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"existing": "value",
			},
		},
	}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.EnsureNestedStringMapEntry("app", "web", "metadata", "labels"))
	labels := content["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	assert.Equal(t, "web", labels["app"])
	assert.Equal(t, "value", labels["existing"])
}

// --- RemoveNestedStringMapEntry ---

func TestUnstructuredContentEditor_RemoveNestedStringMapEntry(t *testing.T) {
	content := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"app":   "web",
				"other": "keep",
			},
		},
	}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.RemoveNestedStringMapEntry("app", "metadata", "labels"))
	labels := content["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	assert.NotContains(t, labels, "app")
	assert.Equal(t, "keep", labels["other"])
}

func TestUnstructuredContentEditor_RemoveNestedStringMapEntry_MissingMap(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	require.NoError(t, e.RemoveNestedStringMapEntry("app", "metadata", "labels"))
}

// --- SetNestedSlice ---

func TestUnstructuredContentEditor_SetNestedSlice(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	s := []interface{}{"a", "b", "c"}
	require.NoError(t, e.SetNestedSlice(s, "spec", "items"))
	items := content["spec"].(map[string]interface{})["items"].([]interface{})
	assert.Equal(t, []interface{}{"a", "b", "c"}, items)
}

// --- SetNestedMap ---

func TestUnstructuredContentEditor_SetNestedMap(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	m := map[string]interface{}{"port": int64(8080), "protocol": "TCP"}
	require.NoError(t, e.SetNestedMap(m, "spec", "endpoint"))
	endpoint := content["spec"].(map[string]interface{})["endpoint"].(map[string]interface{})
	assert.Equal(t, int64(8080), endpoint["port"])
	assert.Equal(t, "TCP", endpoint["protocol"])
}

// --- Raw ---

func TestUnstructuredContentEditor_Raw(t *testing.T) {
	content := map[string]interface{}{"key": "value"}
	e := NewUnstructuredContentEditor(content)
	raw := e.Raw()
	raw["new"] = "added"
	assert.Equal(t, "added", content["new"])
}

func TestUnstructuredContentEditor_Raw_ReturnsSameMap(t *testing.T) {
	content := map[string]interface{}{}
	e := NewUnstructuredContentEditor(content)
	assert.Equal(t, content, e.Raw())
}
