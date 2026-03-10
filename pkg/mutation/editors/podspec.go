package editors

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

// PodSpecEditor provides a typed API for mutating a Kubernetes PodSpec.
// It remains a scoped editor, and Raw() can be used for less common changes.
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

// SetServiceAccountName sets the spec.ServiceAccountName field.
func (e *PodSpecEditor) SetServiceAccountName(name string) {
	e.spec.ServiceAccountName = name
}

// EnsureImagePullSecret appends the given secret name to spec.ImagePullSecrets if it's not already present.
// It identifies existing secrets by name.
func (e *PodSpecEditor) EnsureImagePullSecret(name string) {
	for _, s := range e.spec.ImagePullSecrets {
		if s.Name == name {
			return
		}
	}
	e.spec.ImagePullSecrets = append(e.spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
}

// RemoveImagePullSecret removes all entries from spec.ImagePullSecrets that match the given name.
// It's a no-op if no matching entries are present.
func (e *PodSpecEditor) RemoveImagePullSecret(name string) {
	newSecrets := make([]corev1.LocalObjectReference, 0, len(e.spec.ImagePullSecrets))
	for _, s := range e.spec.ImagePullSecrets {
		if s.Name != name {
			newSecrets = append(newSecrets, s)
		}
	}
	e.spec.ImagePullSecrets = newSecrets
}

// EnsureNodeSelector initializes spec.NodeSelector if nil and upserts the given key/value pair.
func (e *PodSpecEditor) EnsureNodeSelector(key, value string) {
	if e.spec.NodeSelector == nil {
		e.spec.NodeSelector = make(map[string]string)
	}
	e.spec.NodeSelector[key] = value
}

// RemoveNodeSelector removes the key from spec.NodeSelector.
// It's a no-op if NodeSelector is nil or the key is not present.
func (e *PodSpecEditor) RemoveNodeSelector(key string) {
	if e.spec.NodeSelector == nil {
		return
	}
	delete(e.spec.NodeSelector, key)
}

// EnsureToleration appends the given toleration to spec.Tolerations if an equal toleration is not already present.
// It uses exact struct equality for comparison.
func (e *PodSpecEditor) EnsureToleration(t corev1.Toleration) {
	for _, existing := range e.spec.Tolerations {
		if reflect.DeepEqual(existing, t) {
			return
		}
	}
	e.spec.Tolerations = append(e.spec.Tolerations, t)
}

// RemoveTolerations removes all tolerations for which the match function returns true.
// It ignores the call if the match function is nil.
func (e *PodSpecEditor) RemoveTolerations(match func(corev1.Toleration) bool) {
	if match == nil {
		return
	}
	newTolerations := make([]corev1.Toleration, 0, len(e.spec.Tolerations))
	for _, t := range e.spec.Tolerations {
		if !match(t) {
			newTolerations = append(newTolerations, t)
		}
	}
	e.spec.Tolerations = newTolerations
}

// EnsureVolume replaces an existing volume with the same name in spec.Volumes or appends it if missing.
// It identifies volumes by their Name field.
func (e *PodSpecEditor) EnsureVolume(v corev1.Volume) {
	for i, existing := range e.spec.Volumes {
		if existing.Name == v.Name {
			e.spec.Volumes[i] = v
			return
		}
	}
	e.spec.Volumes = append(e.spec.Volumes, v)
}

// RemoveVolume removes all volumes with the given name from spec.Volumes.
// It's a no-op if no volume with the given name is present.
func (e *PodSpecEditor) RemoveVolume(name string) {
	newVolumes := make([]corev1.Volume, 0, len(e.spec.Volumes))
	for _, v := range e.spec.Volumes {
		if v.Name != name {
			newVolumes = append(newVolumes, v)
		}
	}
	e.spec.Volumes = newVolumes
}

// SetPriorityClassName sets the spec.PriorityClassName field.
func (e *PodSpecEditor) SetPriorityClassName(name string) {
	e.spec.PriorityClassName = name
}

// SetHostNetwork sets the spec.HostNetwork field.
func (e *PodSpecEditor) SetHostNetwork(enabled bool) {
	e.spec.HostNetwork = enabled
}

// SetHostPID sets the spec.HostPID field.
func (e *PodSpecEditor) SetHostPID(enabled bool) {
	e.spec.HostPID = enabled
}

// SetHostIPC sets the spec.HostIPC field.
func (e *PodSpecEditor) SetHostIPC(enabled bool) {
	e.spec.HostIPC = enabled
}

// SetSecurityContext sets the spec.SecurityContext field.
func (e *PodSpecEditor) SetSecurityContext(ctx *corev1.PodSecurityContext) {
	e.spec.SecurityContext = ctx
}
