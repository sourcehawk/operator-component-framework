package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewSecretDataEditor ---

func TestNewSecretDataEditor_NilPointers(t *testing.T) {
	// Nil pointers must not panic; operations succeed but writes are local only.
	e := NewSecretDataEditor(nil, nil)
	require.NotNil(t, e)
	e.Set("k", []byte("v"))
	e.SetString("k", "v")
}

// --- Set and Remove (.data) ---

func TestSecretDataEditor_Set(t *testing.T) {
	data := map[string][]byte{}
	e := NewSecretDataEditor(&data, nil)
	e.Set("key", []byte("value"))
	assert.Equal(t, []byte("value"), data["key"])
}

func TestSecretDataEditor_Set_InitialisesNilMap(t *testing.T) {
	var data map[string][]byte
	e := NewSecretDataEditor(&data, nil)
	e.Set("key", []byte("value"))
	require.NotNil(t, data)
	assert.Equal(t, []byte("value"), data["key"])
}

func TestSecretDataEditor_Set_Overwrites(t *testing.T) {
	data := map[string][]byte{"key": []byte("old")}
	e := NewSecretDataEditor(&data, nil)
	e.Set("key", []byte("new"))
	assert.Equal(t, []byte("new"), data["key"])
}

func TestSecretDataEditor_Remove(t *testing.T) {
	data := map[string][]byte{"key": []byte("v"), "other": []byte("keep")}
	e := NewSecretDataEditor(&data, nil)
	e.Remove("key")
	assert.NotContains(t, data, "key")
	assert.Equal(t, []byte("keep"), data["other"])
}

func TestSecretDataEditor_Remove_MissingKey(t *testing.T) {
	data := map[string][]byte{"other": []byte("keep")}
	e := NewSecretDataEditor(&data, nil)
	e.Remove("missing")
	assert.Equal(t, []byte("keep"), data["other"])
}

// --- SetString and RemoveString (.stringData) ---

func TestSecretDataEditor_SetString(t *testing.T) {
	sd := map[string]string{}
	e := NewSecretDataEditor(nil, &sd)
	e.SetString("key", "value")
	assert.Equal(t, "value", sd["key"])
}

func TestSecretDataEditor_SetString_InitialisesNilMap(t *testing.T) {
	var sd map[string]string
	e := NewSecretDataEditor(nil, &sd)
	e.SetString("key", "value")
	require.NotNil(t, sd)
	assert.Equal(t, "value", sd["key"])
}

func TestSecretDataEditor_RemoveString(t *testing.T) {
	sd := map[string]string{"key": "v", "other": "keep"}
	e := NewSecretDataEditor(nil, &sd)
	e.RemoveString("key")
	assert.NotContains(t, sd, "key")
	assert.Equal(t, "keep", sd["other"])
}

func TestSecretDataEditor_RemoveString_MissingKey(t *testing.T) {
	sd := map[string]string{"other": "keep"}
	e := NewSecretDataEditor(nil, &sd)
	e.RemoveString("missing")
	assert.Equal(t, "keep", sd["other"])
}

// --- Raw and RawStringData ---

func TestSecretDataEditor_Raw(t *testing.T) {
	data := map[string][]byte{"existing": []byte("keep")}
	e := NewSecretDataEditor(&data, nil)
	raw := e.Raw()
	raw["new"] = []byte("added")
	assert.Equal(t, []byte("keep"), data["existing"])
	assert.Equal(t, []byte("added"), data["new"])
}

func TestSecretDataEditor_Raw_InitialisesNilMap(t *testing.T) {
	var data map[string][]byte
	e := NewSecretDataEditor(&data, nil)
	raw := e.Raw()
	require.NotNil(t, raw)
	raw["k"] = []byte("v")
	assert.Equal(t, []byte("v"), data["k"])
}

func TestSecretDataEditor_RawStringData(t *testing.T) {
	sd := map[string]string{"existing": "keep"}
	e := NewSecretDataEditor(nil, &sd)
	raw := e.RawStringData()
	raw["new"] = "added"
	assert.Equal(t, "keep", sd["existing"])
	assert.Equal(t, "added", sd["new"])
}

func TestSecretDataEditor_RawStringData_InitialisesNilMap(t *testing.T) {
	var sd map[string]string
	e := NewSecretDataEditor(nil, &sd)
	raw := e.RawStringData()
	require.NotNil(t, raw)
	raw["k"] = "v"
	assert.Equal(t, "v", sd["k"])
}
