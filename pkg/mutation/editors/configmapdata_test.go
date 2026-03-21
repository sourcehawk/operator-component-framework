package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// --- Set ---

func TestConfigMapDataEditor_Set(t *testing.T) {
	data := map[string]string{}
	e := NewConfigMapDataEditor(&data, nil)
	e.Set("key", "value")
	assert.Equal(t, "value", data["key"])
}

func TestConfigMapDataEditor_Set_InitialisesNilMap(t *testing.T) {
	var data map[string]string
	e := NewConfigMapDataEditor(&data, nil)
	e.Set("key", "value")
	assert.Equal(t, "value", data["key"])
}

func TestConfigMapDataEditor_Set_Overwrites(t *testing.T) {
	data := map[string]string{"key": "old"}
	e := NewConfigMapDataEditor(&data, nil)
	e.Set("key", "new")
	assert.Equal(t, "new", data["key"])
}

// --- Remove ---

func TestConfigMapDataEditor_Remove(t *testing.T) {
	data := map[string]string{"key": "value", "other": "keep"}
	e := NewConfigMapDataEditor(&data, nil)
	e.Remove("key")
	assert.NotContains(t, data, "key")
	assert.Equal(t, "keep", data["other"])
}

func TestConfigMapDataEditor_Remove_NotPresent(t *testing.T) {
	data := map[string]string{"other": "keep"}
	e := NewConfigMapDataEditor(&data, nil)
	e.Remove("missing")
	assert.Equal(t, "keep", data["other"])
}

// --- Raw ---

func TestConfigMapDataEditor_Raw(t *testing.T) {
	data := map[string]string{"key": "value"}
	e := NewConfigMapDataEditor(&data, nil)
	raw := e.Raw()
	raw["new"] = "added"
	assert.Equal(t, "added", data["new"])
}

func TestConfigMapDataEditor_Raw_InitialisesNilMap(t *testing.T) {
	var data map[string]string
	e := NewConfigMapDataEditor(&data, nil)
	raw := e.Raw()
	raw["key"] = "value"
	assert.Equal(t, "value", data["key"])
}

// --- MergeYAML ---

func TestConfigMapDataEditor_MergeYAML_NewKey(t *testing.T) {
	data := map[string]string{}
	e := NewConfigMapDataEditor(&data, nil)
	require.NoError(t, e.MergeYAML("cfg", "a: 1\n"))
	assert.NotEmpty(t, data["cfg"])
}

func TestConfigMapDataEditor_MergeYAML_MergesKeys(t *testing.T) {
	data := map[string]string{"cfg": "a: 1\nb: 2\n"}
	e := NewConfigMapDataEditor(&data, nil)
	require.NoError(t, e.MergeYAML("cfg", "b: 99\nc: 3\n"))

	var out map[string]interface{}
	require.NoError(t, unmarshalYAML(data["cfg"], &out))
	assert.EqualValues(t, 1, out["a"])
	assert.EqualValues(t, 99, out["b"])
	assert.EqualValues(t, 3, out["c"])
}

func TestConfigMapDataEditor_MergeYAML_RecursiveMerge(t *testing.T) {
	data := map[string]string{"cfg": "parent:\n  a: 1\n  b: 2\n"}
	e := NewConfigMapDataEditor(&data, nil)
	require.NoError(t, e.MergeYAML("cfg", "parent:\n  b: 99\n  c: 3\n"))

	var out map[string]interface{}
	require.NoError(t, unmarshalYAML(data["cfg"], &out))
	parent, ok := out["parent"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 1, parent["a"])
	assert.EqualValues(t, 99, parent["b"])
	assert.EqualValues(t, 3, parent["c"])
}

func TestConfigMapDataEditor_MergeYAML_PatchWinsForNonMap(t *testing.T) {
	data := map[string]string{"key": "old-scalar\n"}
	e := NewConfigMapDataEditor(&data, nil)
	require.NoError(t, e.MergeYAML("key", "new-scalar\n"))
	assert.NotEqual(t, "old-scalar\n", data["key"])
}

func TestConfigMapDataEditor_MergeYAML_InvalidBase(t *testing.T) {
	data := map[string]string{"key": "{"}
	e := NewConfigMapDataEditor(&data, nil)
	assert.Error(t, e.MergeYAML("key", "a: 1\n"))
}

func TestConfigMapDataEditor_MergeYAML_InvalidPatch(t *testing.T) {
	data := map[string]string{}
	e := NewConfigMapDataEditor(&data, nil)
	assert.Error(t, e.MergeYAML("key", "{"))
}

func TestConfigMapDataEditor_MergeYAML_InitialisesNilMap(t *testing.T) {
	var data map[string]string
	e := NewConfigMapDataEditor(&data, nil)
	require.NoError(t, e.MergeYAML("key", "a: 1\n"))
	assert.NotEmpty(t, data["key"])
}

// --- SetBinary ---

func TestConfigMapDataEditor_SetBinary(t *testing.T) {
	binaryData := map[string][]byte{}
	e := NewConfigMapDataEditor(nil, &binaryData)
	e.SetBinary("cert", []byte("binary-content"))
	assert.Equal(t, []byte("binary-content"), binaryData["cert"])
}

func TestConfigMapDataEditor_SetBinary_InitialisesNilMap(t *testing.T) {
	var binaryData map[string][]byte
	e := NewConfigMapDataEditor(nil, &binaryData)
	e.SetBinary("cert", []byte("data"))
	assert.Equal(t, []byte("data"), binaryData["cert"])
}

func TestConfigMapDataEditor_SetBinary_Overwrites(t *testing.T) {
	binaryData := map[string][]byte{"cert": []byte("old")}
	e := NewConfigMapDataEditor(nil, &binaryData)
	e.SetBinary("cert", []byte("new"))
	assert.Equal(t, []byte("new"), binaryData["cert"])
}

// --- RemoveBinary ---

func TestConfigMapDataEditor_RemoveBinary(t *testing.T) {
	binaryData := map[string][]byte{"cert": []byte("data"), "other": []byte("keep")}
	e := NewConfigMapDataEditor(nil, &binaryData)
	e.RemoveBinary("cert")
	assert.NotContains(t, binaryData, "cert")
	assert.Equal(t, []byte("keep"), binaryData["other"])
}

func TestConfigMapDataEditor_RemoveBinary_NotPresent(t *testing.T) {
	binaryData := map[string][]byte{"other": []byte("keep")}
	e := NewConfigMapDataEditor(nil, &binaryData)
	e.RemoveBinary("missing")
	assert.Equal(t, []byte("keep"), binaryData["other"])
}

// --- RawBinary ---

func TestConfigMapDataEditor_RawBinary(t *testing.T) {
	binaryData := map[string][]byte{"cert": []byte("data")}
	e := NewConfigMapDataEditor(nil, &binaryData)
	raw := e.RawBinary()
	raw["new"] = []byte("added")
	assert.Equal(t, []byte("added"), binaryData["new"])
}

func TestConfigMapDataEditor_RawBinary_InitialisesNilMap(t *testing.T) {
	var binaryData map[string][]byte
	e := NewConfigMapDataEditor(nil, &binaryData)
	raw := e.RawBinary()
	raw["cert"] = []byte("data")
	assert.Equal(t, []byte("data"), binaryData["cert"])
}

// --- helpers ---

func unmarshalYAML(s string, v interface{}) error {
	return yaml.Unmarshal([]byte(s), v)
}
