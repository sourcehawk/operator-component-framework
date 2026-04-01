package selectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestAllContainers(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}

	assert.True(t, AllContainers()(0, c1))
	assert.True(t, AllContainers()(1, c2))
}

func TestContainerNamed(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}

	assert.True(t, ContainerNamed("c1")(0, c1))
	assert.False(t, ContainerNamed("c1")(1, c2))
}

func TestContainersNamed(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}

	assert.True(t, ContainersNamed("c1", "c3")(0, c1))
	assert.False(t, ContainersNamed("c1", "c3")(1, c2))
}

func TestContainerNotNamed(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}

	assert.False(t, ContainerNotNamed("c1")(0, c1))
	assert.True(t, ContainerNotNamed("c1")(1, c2))
}

func TestContainersNotNamed(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}
	c3 := &corev1.Container{Name: "c3"}

	assert.False(t, ContainersNotNamed("c1", "c3")(0, c1))
	assert.True(t, ContainersNotNamed("c1", "c3")(1, c2))
	assert.False(t, ContainersNotNamed("c1", "c3")(2, c3))
}

func TestContainerAtIndex(t *testing.T) {
	c1 := &corev1.Container{Name: "c1"}
	c2 := &corev1.Container{Name: "c2"}

	assert.True(t, ContainerAtIndex(0)(0, c1))
	assert.False(t, ContainerAtIndex(0)(1, c2))
	assert.True(t, ContainerAtIndex(1)(1, c2))
}
