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
