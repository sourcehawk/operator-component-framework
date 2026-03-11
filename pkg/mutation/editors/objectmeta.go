package editors

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectMetaEditor provides a typed API for mutating Kubernetes ObjectMeta.
type ObjectMetaEditor struct {
	meta *metav1.ObjectMeta
}

// NewObjectMetaEditor creates a new ObjectMetaEditor for the given ObjectMeta.
func NewObjectMetaEditor(meta *metav1.ObjectMeta) *ObjectMetaEditor {
	return &ObjectMetaEditor{meta: meta}
}

// Raw returns the underlying *metav1.ObjectMeta.
func (e *ObjectMetaEditor) Raw() *metav1.ObjectMeta {
	return e.meta
}

// EnsureLabel ensures a label with the given key and value exists.
func (e *ObjectMetaEditor) EnsureLabel(key, value string) {
	if e.meta.Labels == nil {
		e.meta.Labels = make(map[string]string)
	}
	e.meta.Labels[key] = value
}

// RemoveLabel removes a label with the given key.
func (e *ObjectMetaEditor) RemoveLabel(key string) {
	if e.meta.Labels != nil {
		delete(e.meta.Labels, key)
	}
}

// EnsureAnnotation ensures an annotation with the given key and value exists.
func (e *ObjectMetaEditor) EnsureAnnotation(key, value string) {
	if e.meta.Annotations == nil {
		e.meta.Annotations = make(map[string]string)
	}
	e.meta.Annotations[key] = value
}

// RemoveAnnotation removes an annotation with the given key.
func (e *ObjectMetaEditor) RemoveAnnotation(key string) {
	if e.meta.Annotations != nil {
		delete(e.meta.Annotations, key)
	}
}
