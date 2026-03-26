package editors

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// UnstructuredContentEditor provides a typed API for mutating the content of
// a Kubernetes unstructured object.
//
// It wraps the object's underlying map[string]interface{} and exposes
// structured operations for setting and removing values at nested paths.
// The editor delegates to the k8s.io/apimachinery/pkg/apis/meta/v1/unstructured
// helper functions for path traversal and creation.
//
// For reads during mutation, use Raw() to access the underlying map directly.
type UnstructuredContentEditor struct {
	content map[string]interface{}
}

// NewUnstructuredContentEditor creates a new UnstructuredContentEditor wrapping
// the given content map.
//
// The content map is typically obtained from unstructured.Unstructured.Object.
// Mutations are applied directly to the provided map.
func NewUnstructuredContentEditor(content map[string]interface{}) *UnstructuredContentEditor {
	return &UnstructuredContentEditor{content: content}
}

// Raw returns the underlying content map directly.
//
// This is an escape hatch for free-form editing when none of the structured
// methods are sufficient.
func (e *UnstructuredContentEditor) Raw() map[string]interface{} {
	return e.content
}

// SetNestedField sets a value at the specified nested path, creating
// intermediate maps as needed.
func (e *UnstructuredContentEditor) SetNestedField(value interface{}, fields ...string) error {
	return unstructured.SetNestedField(e.content, value, fields...)
}

// RemoveNestedField removes the field at the specified nested path.
// It is a no-op if the path does not exist.
func (e *UnstructuredContentEditor) RemoveNestedField(fields ...string) {
	unstructured.RemoveNestedField(e.content, fields...)
}

// SetNestedString sets a string value at the specified nested path.
func (e *UnstructuredContentEditor) SetNestedString(value string, fields ...string) error {
	return unstructured.SetNestedField(e.content, value, fields...)
}

// SetNestedBool sets a boolean value at the specified nested path.
func (e *UnstructuredContentEditor) SetNestedBool(value bool, fields ...string) error {
	return unstructured.SetNestedField(e.content, value, fields...)
}

// SetNestedInt64 sets an int64 value at the specified nested path.
func (e *UnstructuredContentEditor) SetNestedInt64(value int64, fields ...string) error {
	return unstructured.SetNestedField(e.content, value, fields...)
}

// SetNestedFloat64 sets a float64 value at the specified nested path.
func (e *UnstructuredContentEditor) SetNestedFloat64(value float64, fields ...string) error {
	return unstructured.SetNestedField(e.content, value, fields...)
}

// SetNestedStringMap sets a map[string]string value at the specified nested path,
// replacing any existing value at that path.
func (e *UnstructuredContentEditor) SetNestedStringMap(value map[string]string, fields ...string) error {
	return unstructured.SetNestedStringMap(e.content, value, fields...)
}

// EnsureNestedStringMapEntry adds or updates a single entry in a nested
// string map. If the map does not exist at the given path it is created.
func (e *UnstructuredContentEditor) EnsureNestedStringMapEntry(key, value string, fields ...string) error {
	existing, _, _ := unstructured.NestedStringMap(e.content, fields...)
	if existing == nil {
		existing = make(map[string]string)
	}
	existing[key] = value
	return unstructured.SetNestedStringMap(e.content, existing, fields...)
}

// RemoveNestedStringMapEntry removes a single entry from a nested string map.
// It is a no-op if the map or the key does not exist.
func (e *UnstructuredContentEditor) RemoveNestedStringMapEntry(key string, fields ...string) error {
	existing, found, err := unstructured.NestedStringMap(e.content, fields...)
	if err != nil {
		return err
	}
	if !found || existing == nil {
		return nil
	}
	delete(existing, key)
	return unstructured.SetNestedStringMap(e.content, existing, fields...)
}

// SetNestedSlice sets a []interface{} value at the specified nested path,
// replacing any existing value at that path.
func (e *UnstructuredContentEditor) SetNestedSlice(value []interface{}, fields ...string) error {
	return unstructured.SetNestedSlice(e.content, value, fields...)
}

// SetNestedMap sets a map[string]interface{} value at the specified nested path,
// replacing any existing value at that path.
func (e *UnstructuredContentEditor) SetNestedMap(value map[string]interface{}, fields ...string) error {
	return unstructured.SetNestedMap(e.content, value, fields...)
}
