package unstructured

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Compile-time interface assertion.
var _ generic.FeatureMutator = (*Mutator)(nil)

func newTestObject() *uns.Unstructured {
	return &uns.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Widget",
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "default",
			},
		},
	}
}

func TestMutator_NewMutator_CreatesInitialScope(t *testing.T) {
	m := NewMutator(newTestObject())
	assert.Len(t, m.plans, 1)
	assert.NotNil(t, m.active)
}

func TestMutator_NextFeature_AdvancesScope(t *testing.T) {
	m := NewMutator(newTestObject())
	m.NextFeature()
	assert.Len(t, m.plans, 2)
}

func TestMutator_EditObjectMetadata_NilIsIgnored(t *testing.T) {
	m := NewMutator(newTestObject())
	m.EditObjectMetadata(nil)
	assert.Empty(t, m.active.metadataEdits)
}

func TestMutator_EditContent_NilIsIgnored(t *testing.T) {
	m := NewMutator(newTestObject())
	m.EditContent(nil)
	assert.Empty(t, m.active.contentEdits)
}

func TestMutator_Apply_MetadataEdits_Labels(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "test")
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "test", obj.GetLabels()["app"])
}

func TestMutator_Apply_MetadataEdits_Annotations(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureAnnotation("note", "hello")
		return nil
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, "hello", obj.GetAnnotations()["note"])
}

func TestMutator_Apply_ContentEdits(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	m.EditContent(func(e *editors.UnstructuredContentEditor) error {
		return e.SetNestedString("nginx", "spec", "image")
	})

	require.NoError(t, m.Apply())

	spec := obj.Object["spec"].(map[string]interface{})
	assert.Equal(t, "nginx", spec["image"])
}

func TestMutator_Apply_MetadataBeforeContent(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	order := make([]string, 0, 2)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		order = append(order, "metadata")
		e.EnsureLabel("k", "v")
		return nil
	})

	m.EditContent(func(e *editors.UnstructuredContentEditor) error {
		order = append(order, "content")
		return e.SetNestedString("val", "spec", "key")
	})

	require.NoError(t, m.Apply())
	assert.Equal(t, []string{"metadata", "content"}, order)
}

func TestMutator_Apply_MultiFeatureIsolation(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	// Feature 1: set spec.replicas
	m.EditContent(func(e *editors.UnstructuredContentEditor) error {
		return e.SetNestedInt64(3, "spec", "replicas")
	})

	m.NextFeature()

	// Feature 2: set spec.image, reading replicas from feature 1
	m.EditContent(func(e *editors.UnstructuredContentEditor) error {
		return e.SetNestedString("nginx", "spec", "image")
	})

	require.NoError(t, m.Apply())

	spec := obj.Object["spec"].(map[string]interface{})
	assert.Equal(t, int64(3), spec["replicas"])
	assert.Equal(t, "nginx", spec["image"])
}

func TestMutator_Apply_PreservesExistingLabels(t *testing.T) {
	obj := newTestObject()
	obj.SetLabels(map[string]string{"existing": "label"})
	m := NewMutator(obj)

	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("new", "label")
		return nil
	})

	require.NoError(t, m.Apply())
	labels := obj.GetLabels()
	assert.Equal(t, "label", labels["existing"])
	assert.Equal(t, "label", labels["new"])
}

func TestMutator_Apply_ErrorStopsExecution(t *testing.T) {
	obj := newTestObject()
	m := NewMutator(obj)

	m.EditContent(func(e *editors.UnstructuredContentEditor) error {
		return assert.AnError
	})

	secondCalled := false
	m.EditContent(func(_ *editors.UnstructuredContentEditor) error {
		secondCalled = true
		return nil
	})

	assert.Error(t, m.Apply())
	assert.False(t, secondCalled, "second edit should not be called after error")
}
