package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestPodSpecEditor_Raw(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)

	assert.Equal(t, spec, e.Raw())

	e.Raw().ServiceAccountName = "test-sa"
	assert.Equal(t, "test-sa", spec.ServiceAccountName)
}

func TestPodSpecEditor_SetServiceAccountName(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)
	e.SetServiceAccountName("my-sa")
	assert.Equal(t, "my-sa", spec.ServiceAccountName)
}

func TestPodSpecEditor_ImagePullSecrets(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)

	// EnsureImagePullSecret
	e.EnsureImagePullSecret("secret1")
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret1"}}, spec.ImagePullSecrets)

	e.EnsureImagePullSecret("secret1") // no-op
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret1"}}, spec.ImagePullSecrets)

	e.EnsureImagePullSecret("secret2")
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret1"}, {Name: "secret2"}}, spec.ImagePullSecrets)

	// RemoveImagePullSecret
	spec.ImagePullSecrets = append(spec.ImagePullSecrets, corev1.LocalObjectReference{Name: "secret1"})
	e.RemoveImagePullSecret("secret1")
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret2"}}, spec.ImagePullSecrets)

	e.RemoveImagePullSecret("non-existent") // safe
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "secret2"}}, spec.ImagePullSecrets)
}

func TestPodSpecEditor_NodeSelector(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)

	// EnsureNodeSelector
	e.EnsureNodeSelector("key1", "val1")
	assert.Equal(t, map[string]string{"key1": "val1"}, spec.NodeSelector)

	e.EnsureNodeSelector("key1", "val2") // update
	assert.Equal(t, map[string]string{"key1": "val2"}, spec.NodeSelector)

	e.EnsureNodeSelector("key2", "val2")
	assert.Equal(t, map[string]string{"key1": "val2", "key2": "val2"}, spec.NodeSelector)

	// RemoveNodeSelector
	e.RemoveNodeSelector("key1")
	assert.Equal(t, map[string]string{"key2": "val2"}, spec.NodeSelector)

	e.RemoveNodeSelector("non-existent") // safe
	assert.Equal(t, map[string]string{"key2": "val2"}, spec.NodeSelector)

	spec.NodeSelector = nil
	e.RemoveNodeSelector("key2") // safe on nil map
	assert.Nil(t, spec.NodeSelector)
}

func TestPodSpecEditor_Tolerations(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)
	tol1 := corev1.Toleration{Key: "key1", Operator: corev1.TolerationOpEqual, Value: "val1"}

	// EnsureToleration
	e.EnsureToleration(tol1)
	assert.Equal(t, []corev1.Toleration{tol1}, spec.Tolerations)

	e.EnsureToleration(tol1) // no-op
	assert.Equal(t, []corev1.Toleration{tol1}, spec.Tolerations)

	// RemoveTolerations
	tol2 := corev1.Toleration{Key: "key2", Operator: corev1.TolerationOpEqual, Value: "val2"}
	spec.Tolerations = append(spec.Tolerations, tol2, tol1)

	e.RemoveTolerations(func(t corev1.Toleration) bool { return t.Key == "key1" })
	assert.Equal(t, []corev1.Toleration{tol2}, spec.Tolerations)

	e.RemoveTolerations(nil) // no-op
	assert.Equal(t, []corev1.Toleration{tol2}, spec.Tolerations)
}

func TestPodSpecEditor_Volumes(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)
	vol1 := corev1.Volume{Name: "vol1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}

	// EnsureVolume
	e.EnsureVolume(vol1)
	assert.Equal(t, []corev1.Volume{vol1}, spec.Volumes)

	vol1Updated := corev1.Volume{Name: "vol1", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/foo"}}}
	e.EnsureVolume(vol1Updated) // replace
	assert.Equal(t, []corev1.Volume{vol1Updated}, spec.Volumes)

	// RemoveVolume
	spec.Volumes = append(spec.Volumes, corev1.Volume{Name: "vol2"}, vol1Updated)
	e.RemoveVolume("vol1")
	assert.Equal(t, []corev1.Volume{{Name: "vol2"}}, spec.Volumes)

	e.RemoveVolume("non-existent") // safe
	assert.Equal(t, []corev1.Volume{{Name: "vol2"}}, spec.Volumes)
}

func TestPodSpecEditor_SetPriorityClassName(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)
	e.SetPriorityClassName("high-priority")
	assert.Equal(t, "high-priority", spec.PriorityClassName)
}

func TestPodSpecEditor_HostToggles(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)

	e.SetHostNetwork(true)
	assert.True(t, spec.HostNetwork)
	e.SetHostPID(true)
	assert.True(t, spec.HostPID)
	e.SetHostIPC(true)
	assert.True(t, spec.HostIPC)
}

func TestPodSpecEditor_SetSecurityContext(t *testing.T) {
	spec := &corev1.PodSpec{}
	e := NewPodSpecEditor(spec)
	sc := &corev1.PodSecurityContext{RunAsUser: ptr(int64(1000))}
	e.SetSecurityContext(sc)
	assert.Equal(t, sc, spec.SecurityContext)
}

func ptr[T any](v T) *T {
	return &v
}
