package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentResourceMutator records feature mutation intent for a Deployment
// and applies the resulting changes in one final pass.
//
// This keeps feature mutations simple and expressive while allowing the mutator
// to own mutation mechanics and resolve slice updates efficiently.
type DeploymentResourceMutator struct {
	current *appsv1.Deployment

	// envOps stores the desired env var operations per container.
	// Keyed by container index, then env var name.
	envOps map[int]map[string]envOp

	// argOps stores the desired arg operations per container.
	// Keyed by container index, then arg value.
	argOps map[int]map[string]argOp
}

type envOp struct {
	remove bool
	value  string
}

type argOp struct {
	ensure bool
	remove bool
}

// NewDeploymentResourceMutator creates a new planner-style mutator for the given resource.
//
// The mutator records intended changes and applies them later via Apply().
func NewDeploymentResourceMutator(current *appsv1.Deployment) *DeploymentResourceMutator {
	return &DeploymentResourceMutator{
		current: current,
		envOps:  map[int]map[string]envOp{},
		argOps:  map[int]map[string]argOp{},
	}
}

// EnsureContainerEnvVar records that an environment variable should exist in all containers.
//
// If the env var already exists when Apply() runs, its value will be updated.
// If it does not exist, it will be appended.
// If the same env var was previously marked for removal, this overrides that removal.
func (m *DeploymentResourceMutator) EnsureContainerEnvVar(name, value string) {
	for i := range m.current.Spec.Template.Spec.Containers {
		if _, ok := m.envOps[i]; !ok {
			m.envOps[i] = map[string]envOp{}
		}
		m.envOps[i][name] = envOp{
			remove: false,
			value:  value,
		}
	}
}

// RemoveContainerEnvVar records that an environment variable should be removed from all containers.
//
// If the same env var was previously ensured, removal takes precedence unless another ensure call
// later overrides it.
func (m *DeploymentResourceMutator) RemoveContainerEnvVar(name string) {
	for i := range m.current.Spec.Template.Spec.Containers {
		if _, ok := m.envOps[i]; !ok {
			m.envOps[i] = map[string]envOp{}
		}
		m.envOps[i][name] = envOp{
			remove: true,
		}
	}
}

// EnsureContainerArg records that a CLI argument should exist in all containers.
//
// If the arg is already present when Apply() runs, it is kept as-is.
// If it is missing, it is appended.
// If the arg was previously marked for removal, this overrides that removal.
func (m *DeploymentResourceMutator) EnsureContainerArg(arg string) {
	for i := range m.current.Spec.Template.Spec.Containers {
		if _, ok := m.argOps[i]; !ok {
			m.argOps[i] = map[string]argOp{}
		}
		m.argOps[i][arg] = argOp{
			ensure: true,
			remove: false,
		}
	}
}

// RemoveContainerArg records that a CLI argument should be removed from all containers.
//
// If the same arg was previously ensured, removal takes precedence unless another ensure call
// later overrides it.
func (m *DeploymentResourceMutator) RemoveContainerArg(arg string) {
	for i := range m.current.Spec.Template.Spec.Containers {
		if _, ok := m.argOps[i]; !ok {
			m.argOps[i] = map[string]argOp{}
		}
		m.argOps[i][arg] = argOp{
			ensure: false,
			remove: true,
		}
	}
}

// Apply computes and writes the final Deployment state from the recorded mutation intent.
// The core desired state is already applied in Mutate() before this is called.
func (m *DeploymentResourceMutator) Apply() error {
	for i := range m.current.Spec.Template.Spec.Containers {
		m.applyEnvOps(i)
		m.applyArgOps(i)
	}
	return nil
}

// applyEnvOps applies all recorded env var operations for a single container.
func (m *DeploymentResourceMutator) applyEnvOps(containerIndex int) {
	ops, ok := m.envOps[containerIndex]
	if !ok || len(ops) == 0 {
		return
	}

	container := &m.current.Spec.Template.Spec.Containers[containerIndex]

	// Create a copy of the env slice to avoid modifying the original if it was shared.
	// But actually, we want to modify m.current.

	// Start from current env vars and filter/update them.
	newEnv := make([]corev1.EnvVar, 0, len(container.Env))
	seen := map[string]bool{}

	for _, env := range container.Env {
		op, hasOp := ops[env.Name]
		if hasOp {
			seen[env.Name] = true

			if op.remove {
				continue
			}

			env.Value = op.value
			newEnv = append(newEnv, env)
			continue
		}

		newEnv = append(newEnv, env)
	}

	// Append ensured env vars that were not already present.
	for name, op := range ops {
		if op.remove || seen[name] {
			continue
		}

		newEnv = append(newEnv, corev1.EnvVar{
			Name:  name,
			Value: op.value,
		})
	}

	container.Env = newEnv
}

// applyArgOps applies all recorded arg operations for a single container.
func (m *DeploymentResourceMutator) applyArgOps(containerIndex int) {
	ops, ok := m.argOps[containerIndex]
	if !ok || len(ops) == 0 {
		return
	}

	container := &m.current.Spec.Template.Spec.Containers[containerIndex]

	// Start from current args and filter them.
	newArgs := make([]string, 0, len(container.Args))
	existing := map[string]bool{}

	for _, arg := range container.Args {
		op, hasOp := ops[arg]
		if hasOp {
			existing[arg] = true

			if op.remove {
				continue
			}
		}

		newArgs = append(newArgs, arg)
	}

	// Append ensured args that were not already present.
	for arg, op := range ops {
		if !op.ensure || op.remove || existing[arg] {
			continue
		}

		newArgs = append(newArgs, arg)
	}

	container.Args = newArgs
}

// HasPlannedEnvVar reports whether an env var currently has a planned operation
// for any container. This is optional and mainly useful for debugging or tests.
func (m *DeploymentResourceMutator) HasPlannedEnvVar(name string) bool {
	for _, ops := range m.envOps {
		if _, ok := ops[name]; ok {
			return true
		}
	}
	return false
}

// HasPlannedArg reports whether an arg currently has a planned operation
// for any container. This is optional and mainly useful for debugging or tests.
func (m *DeploymentResourceMutator) HasPlannedArg(arg string) bool {
	for _, ops := range m.argOps {
		if _, ok := ops[arg]; ok {
			return true
		}
	}
	return false
}

// EnsureContainerArgOrdered records an argument and places it after Apply() only if absent.
// If order matters more strictly than simple append semantics, prefer a dedicated helper.
// This helper is optional and can be removed if not needed.
func (m *DeploymentResourceMutator) EnsureContainerArgOrdered(arg string) {
	m.EnsureContainerArg(arg)
}
