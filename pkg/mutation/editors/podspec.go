package editors

import corev1 "k8s.io/api/core/v1"

// PodSpecEditor provides a typed API for mutating a Kubernetes PodSpec.
type PodSpecEditor struct {
	spec *corev1.PodSpec
}

// NewPodSpecEditor creates a new PodSpecEditor for the given PodSpec.
func NewPodSpecEditor(spec *corev1.PodSpec) *PodSpecEditor {
	return &PodSpecEditor{spec: spec}
}

// Raw returns the underlying *corev1.PodSpec.
func (e *PodSpecEditor) Raw() *corev1.PodSpec {
	return e.spec
}
