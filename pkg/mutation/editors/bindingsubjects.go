package editors

import rbacv1 "k8s.io/api/rbac/v1"

// BindingSubjectsEditor provides a typed API for mutating the subjects list
// of a Kubernetes RoleBinding or ClusterRoleBinding.
//
// It exposes structured operations (EnsureSubject, RemoveSubject) as well as
// Raw() for free-form access when none of the structured methods are sufficient.
type BindingSubjectsEditor struct {
	subjects *[]rbacv1.Subject
}

// NewBindingSubjectsEditor creates a new BindingSubjectsEditor wrapping the
// given subjects slice pointer.
//
// The pointer may refer to a nil slice; methods that add subjects initialise
// it automatically. If a nil pointer is provided, an empty slice is allocated
// and used internally.
func NewBindingSubjectsEditor(subjects *[]rbacv1.Subject) *BindingSubjectsEditor {
	if subjects == nil {
		empty := make([]rbacv1.Subject, 0)
		subjects = &empty
	}
	return &BindingSubjectsEditor{subjects: subjects}
}

// EnsureSubject upserts a subject in the subjects list.
//
// A subject is identified by the combination of Kind, Name, and Namespace.
// If a matching subject already exists it is replaced; otherwise the new subject
// is appended.
func (e *BindingSubjectsEditor) EnsureSubject(subject rbacv1.Subject) {
	for i, s := range *e.subjects {
		if s.Kind == subject.Kind && s.Name == subject.Name && s.Namespace == subject.Namespace {
			(*e.subjects)[i] = subject
			return
		}
	}
	*e.subjects = append(*e.subjects, subject)
}

// RemoveSubject removes a subject identified by kind, name, and namespace
// from the subjects list. It is a no-op if no matching subject exists.
func (e *BindingSubjectsEditor) RemoveSubject(kind, name, namespace string) {
	subjects := *e.subjects
	filtered := subjects[:0]
	for _, s := range subjects {
		if s.Kind == kind && s.Name == name && s.Namespace == namespace {
			continue
		}
		filtered = append(filtered, s)
	}
	// Zero trailing elements to avoid retaining references to removed subjects.
	for i := len(filtered); i < len(subjects); i++ {
		subjects[i] = rbacv1.Subject{}
	}
	*e.subjects = filtered
}

// EnsureServiceAccount ensures a ServiceAccount subject with the given name and
// namespace exists in the binding's subjects list. If an identical subject
// already exists, this is a no-op.
func (e *BindingSubjectsEditor) EnsureServiceAccount(name, namespace string) {
	e.EnsureSubject(rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      name,
		Namespace: namespace,
	})
}

// RemoveServiceAccount removes a ServiceAccount subject with the given name and
// namespace from the binding's subjects list.
func (e *BindingSubjectsEditor) RemoveServiceAccount(name, namespace string) {
	e.RemoveSubject("ServiceAccount", name, namespace)
}

// Raw returns a pointer to the underlying subjects slice, allowing
// free-form modifications when the structured methods are insufficient.
func (e *BindingSubjectsEditor) Raw() *[]rbacv1.Subject {
	return e.subjects
}
