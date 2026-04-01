// Package selectors provides selectors for filtering Kubernetes objects.
package selectors

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
)

// ContainerSelector is a function that determines if a container matches
// a specific criteria for mutation.
//
// In the context of a Mutator Apply() pass, the selector is evaluated against
// the original container snapshot before any edits are applied. This ensures
// stable matching even if an earlier edit renames the container.
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

// ContainerNotNamed returns a ContainerSelector that matches all containers except the one with the given name.
func ContainerNotNamed(name string) ContainerSelector {
	return func(_ int, c *corev1.Container) bool {
		return c.Name != name
	}
}

// ContainersNotNamed returns a ContainerSelector that matches all containers except those with any of the given names.
func ContainersNotNamed(names ...string) ContainerSelector {
	return func(_ int, c *corev1.Container) bool {
		return !slices.Contains(names, c.Name)
	}
}

// ContainerAtIndex returns a ContainerSelector that matches a container at the given index.
func ContainerAtIndex(index int) ContainerSelector {
	return func(i int, _ *corev1.Container) bool {
		return i == index
	}
}
