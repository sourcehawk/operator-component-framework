package selectors

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
)

// ContainerSelector is a function that determines if a container matches
// a specific criteria for mutation.
type ContainerSelector func(index int, c *corev1.Container) bool

// AllContainers returns a ContainerSelector that matches all containers.
func AllContainers() ContainerSelector {
	return func(int, *corev1.Container) bool {
		return true
	}
}

// ContainerNamed returns a ContainerSelector that matches a container with the given name.
func ContainerNamed(name string) ContainerSelector {
	return func(_ int, c *corev1.Container) bool {
		return c.Name == name
	}
}

// ContainersNamed returns a ContainerSelector that matches containers with any of the given names.
func ContainersNamed(names ...string) ContainerSelector {
	return func(_ int, c *corev1.Container) bool {
		return slices.Contains(names, c.Name)
	}
}

// ContainerAtIndex returns a ContainerSelector that matches a container at the given index.
func ContainerAtIndex(index int) ContainerSelector {
	return func(i int, _ *corev1.Container) bool {
		return i == index
	}
}
