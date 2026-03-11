// Package editors provides editors for mutating Kubernetes objects.
package editors

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ContainerEditor provides a typed API for mutating a Kubernetes Container.
type ContainerEditor struct {
	container *corev1.Container
}

// NewContainerEditor creates a new ContainerEditor for the given container.
func NewContainerEditor(container *corev1.Container) *ContainerEditor {
	return &ContainerEditor{container: container}
}

// Raw returns the underlying *corev1.Container.
func (e *ContainerEditor) Raw() *corev1.Container {
	return e.container
}

// EnsureEnvVar ensures an environment variable exists in the container.
//
// Behavior:
//   - If an environment variable with the same name exists, it is replaced with the provided EnvVar.
//   - If it does not exist, the new EnvVar is appended.
func (e *ContainerEditor) EnsureEnvVar(ev corev1.EnvVar) {
	for i := range e.container.Env {
		if e.container.Env[i].Name == ev.Name {
			e.container.Env[i] = ev
			return
		}
	}
	e.container.Env = append(e.container.Env, ev)
}

// EnsureEnvVars ensures multiple environment variables exist in the container.
func (e *ContainerEditor) EnsureEnvVars(vars []corev1.EnvVar) {
	for _, ev := range vars {
		e.EnsureEnvVar(ev)
	}
}

// RemoveEnvVar removes all environment variables with the specified name.
func (e *ContainerEditor) RemoveEnvVar(name string) {
	e.container.Env = slices.DeleteFunc(e.container.Env, func(ev corev1.EnvVar) bool {
		return ev.Name == name
	})
}

// RemoveEnvVars removes all environment variables with any of the specified names.
func (e *ContainerEditor) RemoveEnvVars(names []string) {
	for _, name := range names {
		e.RemoveEnvVar(name)
	}
}

// EnsureArg ensures the specified argument exists in the container's args list.
// If the argument is already present, it is not added again.
func (e *ContainerEditor) EnsureArg(arg string) {
	if !slices.Contains(e.container.Args, arg) {
		e.container.Args = append(e.container.Args, arg)
	}
}

// EnsureArgs ensures multiple arguments exist in the container's args list.
func (e *ContainerEditor) EnsureArgs(args []string) {
	for _, arg := range args {
		e.EnsureArg(arg)
	}
}

// RemoveArg removes all occurrences of the specified argument from the container's args list.
func (e *ContainerEditor) RemoveArg(arg string) {
	e.container.Args = slices.DeleteFunc(e.container.Args, func(a string) bool {
		return a == arg
	})
}

// RemoveArgs removes all occurrences of the specified arguments from the container's args list.
func (e *ContainerEditor) RemoveArgs(args []string) {
	for _, arg := range args {
		e.RemoveArg(arg)
	}
}

// SetResourceLimit sets the resource limit for the container.
func (e *ContainerEditor) SetResourceLimit(name corev1.ResourceName, quantity resource.Quantity) {
	if e.container.Resources.Limits == nil {
		e.container.Resources.Limits = make(corev1.ResourceList)
	}
	e.container.Resources.Limits[name] = quantity
}

// SetResourceRequest sets the resource request for the container.
func (e *ContainerEditor) SetResourceRequest(name corev1.ResourceName, quantity resource.Quantity) {
	if e.container.Resources.Requests == nil {
		e.container.Resources.Requests = make(corev1.ResourceList)
	}
	e.container.Resources.Requests[name] = quantity
}

// SetResources sets the resource requirements for the container.
func (e *ContainerEditor) SetResources(res corev1.ResourceRequirements) {
	e.container.Resources = res
}
