package deployment

import (
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type containerEdit struct {
	selector selectors.ContainerSelector
	edit     func(*editors.ContainerEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes Deployment.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, and then
// applied to the Deployment in a single controlled pass when Apply() is called.
//
// This approach ensures that mutations are applied consistently and minimizes
// repeated scans of the underlying Kubernetes structures.
type Mutator struct {
	current *appsv1.Deployment

	containerEdits           []containerEdit
	podSpecEdits             []func(*editors.PodSpecEditor) error
	deploymentSpecEdits      []func(*editors.DeploymentSpecEditor) error
	podTemplateMetadataEdits []func(*editors.ObjectMetaEditor) error
	deploymentMetadataEdits  []func(*editors.ObjectMetaEditor) error
}

// NewMutator creates a new Mutator for the given Deployment.
//
// It is typically used within a Feature's Mutation logic to express desired
// changes to the Deployment.
func NewMutator(current *appsv1.Deployment) *Mutator {
	return &Mutator{
		current: current,
	}
}

// EditContainers records a mutation for containers matching the given selector.
//
// Planning:
// All container edits are stored and executed during Apply().
//
// Execution Order:
//   - Within this category, edits are applied in registration order.
//   - Overall, container edits are the LAST category to be executed by Apply().
//
// Selection:
//   - The selector determines which containers the edit function will be called for.
//   - If either selector or edit function is nil, the registration is ignored.
func (m *Mutator) EditContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error) {
	if selector == nil || edit == nil {
		return
	}
	m.containerEdits = append(m.containerEdits, containerEdit{
		selector: selector,
		edit:     edit,
	})
}

// EditDeploymentSpec records a mutation for the Deployment's top-level spec.
//
// Planning:
// All deployment spec edits are stored and executed during Apply().
//
// Execution Order:
//   - Within this category, edits are applied in registration order.
//   - Overall, deployment spec edits are executed AFTER deployment-metadata edits but BEFORE pod template/spec/container edits.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditDeploymentSpec(edit func(*editors.DeploymentSpecEditor) error) {
	if edit == nil {
		return
	}
	m.deploymentSpecEdits = append(m.deploymentSpecEdits, edit)
}

// EditPodSpec records a mutation for the Deployment's pod spec.
//
// Planning:
// All pod spec edits are stored and executed during Apply().
//
// Execution Order:
//   - Within this category, edits are applied in registration order.
//   - Overall, pod spec edits are executed AFTER replica/metadata edits but BEFORE container edits.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodSpec(edit func(*editors.PodSpecEditor) error) {
	if edit == nil {
		return
	}
	m.podSpecEdits = append(m.podSpecEdits, edit)
}

// EditPodTemplateMetadata records a mutation for the Deployment's pod template metadata.
//
// Planning:
// All pod template metadata edits are stored and executed during Apply().
//
// Execution Order:
//   - Within this category, edits are applied in registration order.
//   - Overall, pod template metadata edits are executed AFTER replica/deployment-metadata edits but BEFORE pod spec/container edits.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.podTemplateMetadataEdits = append(m.podTemplateMetadataEdits, edit)
}

// EditDeploymentMetadata records a mutation for the Deployment's own metadata.
//
// Planning:
// All deployment metadata edits are stored and executed during Apply().
//
// Execution Order:
//   - Within this category, edits are applied in registration order.
//   - Overall, deployment metadata edits are executed AFTER replica edits but BEFORE pod template/spec/container edits.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditDeploymentMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.deploymentMetadataEdits = append(m.deploymentMetadataEdits, edit)
}

// EnsureReplicas records the desired number of replicas for the Deployment.
func (m *Mutator) EnsureReplicas(replicas int32) {
	m.EditDeploymentSpec(func(e *editors.DeploymentSpecEditor) error {
		e.SetReplicas(replicas)
		return nil
	})
}

// EnsureContainerEnvVar records that an environment variable must be present
// in all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) EnsureContainerEnvVar(ev corev1.EnvVar) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureEnvVar(ev)
		return nil
	})
}

// RemoveContainerEnvVar records that an environment variable should be
// removed from all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) RemoveContainerEnvVar(name string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveEnvVar(name)
		return nil
	})
}

// RemoveContainerEnvVars records that multiple environment variables should be
// removed from all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) RemoveContainerEnvVars(names []string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveEnvVars(names)
		return nil
	})
}

// EnsureContainerArg records that a command-line argument must be present
// in all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) EnsureContainerArg(arg string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureArg(arg)
		return nil
	})
}

// RemoveContainerArg records that a command-line argument should be
// removed from all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) RemoveContainerArg(arg string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveArg(arg)
		return nil
	})
}

// RemoveContainerArgs records that multiple command-line arguments should be
// removed from all containers of the Deployment.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) RemoveContainerArgs(args []string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveArgs(args)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying Deployment.
//
// Execution Order:
// Edits are applied in a fixed category-based order:
// 1. Replicas (via DeploymentSpec edits)
// 2. Deployment metadata edits
// 3. DeploymentSpec edits
// 4. Pod template metadata edits
// 5. Pod spec edits
// 6. Container edits (applied per container in registration order)
//
// Within each category, edits are applied in their registration order.
//
// Timing:
// No changes are made to the Deployment until Apply() is called.
// Selectors and edit functions are executed during this pass.
func (m *Mutator) Apply() error {
	// 2. Deployment metadata
	if len(m.deploymentMetadataEdits) > 0 {
		editor := editors.NewObjectMetaEditor(&m.current.ObjectMeta)
		for _, edit := range m.deploymentMetadataEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 3. DeploymentSpec
	if len(m.deploymentSpecEdits) > 0 {
		editor := editors.NewDeploymentSpecEditor(&m.current.Spec)
		for _, edit := range m.deploymentSpecEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 4. Pod template metadata
	if len(m.podTemplateMetadataEdits) > 0 {
		editor := editors.NewObjectMetaEditor(&m.current.Spec.Template.ObjectMeta)
		for _, edit := range m.podTemplateMetadataEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 5. Pod spec
	if len(m.podSpecEdits) > 0 {
		editor := editors.NewPodSpecEditor(&m.current.Spec.Template.Spec)
		for _, edit := range m.podSpecEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 6. Container edits
	if len(m.containerEdits) > 0 {
		for i := range m.current.Spec.Template.Spec.Containers {
			container := &m.current.Spec.Template.Spec.Containers[i]
			editor := editors.NewContainerEditor(container)
			for _, ce := range m.containerEdits {
				if ce.selector(i, container) {
					if err := ce.edit(editor); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
