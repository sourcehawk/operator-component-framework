package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObjectMetaEditor_Raw(t *testing.T) {
	meta := &metav1.ObjectMeta{}
	e := NewObjectMetaEditor(meta)
	assert.Equal(t, meta, e.Raw())
}

func TestObjectMetaEditor_EnsureLabel(t *testing.T) {
	meta := &metav1.ObjectMeta{}
	e := NewObjectMetaEditor(meta)

	e.EnsureLabel("foo", "bar")
	assert.Equal(t, "bar", meta.Labels["foo"])
	e.EnsureLabel("foo", "updated")
	assert.Equal(t, "updated", meta.Labels["foo"])
}

func TestObjectMetaEditor_RemoveLabel(t *testing.T) {
	meta := &metav1.ObjectMeta{
		Labels: map[string]string{"foo": "bar"},
	}
	e := NewObjectMetaEditor(meta)

	e.RemoveLabel("foo")
	assert.NotContains(t, meta.Labels, "foo")
	e.RemoveLabel("nonexistent") // safe

	// Nil safety
	meta2 := &metav1.ObjectMeta{}
	e2 := NewObjectMetaEditor(meta2)
	e2.RemoveLabel("any")
	assert.Nil(t, meta2.Labels)
}

func TestObjectMetaEditor_EnsureAnnotation(t *testing.T) {
	meta := &metav1.ObjectMeta{}
	e := NewObjectMetaEditor(meta)

	e.EnsureAnnotation("ann", "val")
	assert.Equal(t, "val", meta.Annotations["ann"])
	e.EnsureAnnotation("ann", "updated")
	assert.Equal(t, "updated", meta.Annotations["ann"])
}

func TestObjectMetaEditor_RemoveAnnotation(t *testing.T) {
	meta := &metav1.ObjectMeta{
		Annotations: map[string]string{"ann": "val"},
	}
	e := NewObjectMetaEditor(meta)

	e.RemoveAnnotation("ann")
	assert.NotContains(t, meta.Annotations, "ann")
	e.RemoveAnnotation("nonexistent") // safe

	// Nil safety
	meta2 := &metav1.ObjectMeta{}
	e2 := NewObjectMetaEditor(meta2)
	e2.RemoveAnnotation("any")
	assert.Nil(t, meta2.Annotations)
}
