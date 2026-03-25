package editors

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

// BindingSubjectsEditor provides a typed API for mutating the .subjects field
// of a Kubernetes RoleBinding or ClusterRoleBinding.
type BindingSubjectsEditor struct {
	subjects *[]rbacv1.Subject
}

// NewBindingSubjectsEditor creates a new BindingSubjectsEditor wrapping the given
// subjects slice pointer.
//
// The pointer must be non-nil; the slice it points to may be nil.
func NewBindingSubjectsEditor(subjects *[]rbacv1.Subject) *BindingSubjectsEditor {
	return &BindingSubjectsEditor{subjects: subjects}
}

// Raw returns a pointer to the underlying subjects slice without modifying it.
//
// This is an escape hatch for free-form editing when none of the structured
// methods are sufficient. A pointer is returned so that operations which change
// the slice header (e.g., append, re-slicing) are reflected in the editor.
func (e *BindingSubjectsEditor) Raw() *[]rbacv1.Subject {
	return e.subjects
}

// Add appends a subject to the binding's subjects list.
// Duplicates are not checked — callers are responsible for ensuring uniqueness
// if required.
func (e *BindingSubjectsEditor) Add(subject rbacv1.Subject) {
	*e.subjects = append(*e.subjects, subject)
}

// Remove removes all subjects matching the given kind, name, and namespace.
// It is a no-op if no matching subject is found.
func (e *BindingSubjectsEditor) Remove(kind, name, namespace string) {
	if *e.subjects == nil {
		return
	}
	filtered := make([]rbacv1.Subject, 0, len(*e.subjects))
	for _, s := range *e.subjects {
		if s.Kind == kind && s.Name == name && s.Namespace == namespace {
			continue
		}
		filtered = append(filtered, s)
	}
	*e.subjects = filtered
}

// EnsureServiceAccount ensures a ServiceAccount subject with the given name and
// namespace exists in the binding's subjects list. If an identical subject
// already exists, this is a no-op.
func (e *BindingSubjectsEditor) EnsureServiceAccount(name, namespace string) {
	for _, s := range *e.subjects {
		if s.Kind == "ServiceAccount" && s.Name == name && s.Namespace == namespace {
			return
		}
	}
	e.Add(rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      name,
		Namespace: namespace,
	})
}

// RemoveServiceAccount removes a ServiceAccount subject with the given name and
// namespace from the binding's subjects list.
func (e *BindingSubjectsEditor) RemoveServiceAccount(name, namespace string) {
	e.Remove("ServiceAccount", name, namespace)
}
