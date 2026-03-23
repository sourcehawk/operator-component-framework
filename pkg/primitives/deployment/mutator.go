package deployment

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a deployment Mutator
// only if its associated feature.ResourceFeature is enabled.
type Mutation feature.Mutation[*Mutator]

type containerEdit struct {
	selector selectors.ContainerSelector
	edit     func(*editors.ContainerEditor) error
}

type containerPresenceOp struct {
	name      string
	container *corev1.Container // nil for remove
}

type featurePlan struct {
	deploymentMetadataEdits  []func(*editors.ObjectMetaEditor) error
	deploymentSpecEdits      []func(*editors.DeploymentSpecEditor) error
	podTemplateMetadataEdits []func(*editors.ObjectMetaEditor) error
	podSpecEdits             []func(*editors.PodSpecEditor) error
	containerPresence        []containerPresenceOp
	containerEdits           []containerEdit
	initContainerPresence    []containerPresenceOp
	initContainerEdits       []containerEdit
}

// Mutator is a high-level helper for modifying a Kubernetes Deployment.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, and then
// applied to the Deployment in a single controlled pass when Apply() is called.
//
// This approach ensures that mutations are applied consistently and minimizes
// repeated scans of the underlying Kubernetes structures.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
type Mutator struct {
	current *appsv1.Deployment

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Deployment.
//
// It is typically used within a Feature's Mutation logic to express desired
// changes to the Deployment.
func NewMutator(current *appsv1.Deployment) *Mutator {
	m := &Mutator{
		current: current,
	}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan until the next
// beginFeature call.
//
// This is used to ensure that mutations from different features are applied
// in registration order while maintaining internal category ordering within
// each feature.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditContainers records a mutation for containers matching the given selector.
//
// Planning:
// All container edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, container edits are executed AFTER container presence operations within the same feature.
//
// Selection:
//   - The selector determines which containers the edit function will be called for.
//   - If either selector or edit function is nil, the registration is ignored.
//   - Selectors are intended to target containers defined by the baseline resource structure or added by earlier presence operations.
//   - Selector matching is evaluated against a snapshot taken after the current feature's container presence operations are applied.
//   - Mutations should not rely on earlier edits in the SAME feature phase changing which selectors match.
func (m *Mutator) EditContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error) {
	if selector == nil || edit == nil {
		return
	}
	m.active.containerEdits = append(m.active.containerEdits, containerEdit{
		selector: selector,
		edit:     edit,
	})
}

// EditInitContainers records a mutation for init containers matching the given selector.
//
// Planning:
// All init container edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, init container edits apply only to spec.template.spec.initContainers.
//   - They run in their own category during Apply(), after init container presence operations within the same feature.
//
// Selection:
//   - The selector determines which init containers the edit function will be called for.
//   - If either selector or edit function is nil, the registration is ignored.
//   - Selector matching is evaluated against a snapshot taken after the current feature's init container presence operations are applied.
func (m *Mutator) EditInitContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error) {
	if selector == nil || edit == nil {
		return
	}
	m.active.initContainerEdits = append(m.active.initContainerEdits, containerEdit{
		selector: selector,
		edit:     edit,
	})
}

// EnsureContainer records that a regular container must be present in the Deployment.
// If a container with the same name exists, it is replaced; otherwise, it is appended.
func (m *Mutator) EnsureContainer(container corev1.Container) {
	m.active.containerPresence = append(m.active.containerPresence, containerPresenceOp{
		name:      container.Name,
		container: &container,
	})
}

// RemoveContainer records that a regular container should be removed by name.
func (m *Mutator) RemoveContainer(name string) {
	m.active.containerPresence = append(m.active.containerPresence, containerPresenceOp{
		name:      name,
		container: nil,
	})
}

// RemoveContainers records that multiple regular containers should be removed by name.
func (m *Mutator) RemoveContainers(names []string) {
	for _, name := range names {
		m.RemoveContainer(name)
	}
}

// EnsureInitContainer records that an init container must be present in the Deployment.
// If an init container with the same name exists, it is replaced; otherwise, it is appended.
func (m *Mutator) EnsureInitContainer(container corev1.Container) {
	m.active.initContainerPresence = append(m.active.initContainerPresence, containerPresenceOp{
		name:      container.Name,
		container: &container,
	})
}

// RemoveInitContainer records that an init container should be removed by name.
func (m *Mutator) RemoveInitContainer(name string) {
	m.active.initContainerPresence = append(m.active.initContainerPresence, containerPresenceOp{
		name:      name,
		container: nil,
	})
}

// RemoveInitContainers records that multiple init containers should be removed by name.
func (m *Mutator) RemoveInitContainers(names []string) {
	for _, name := range names {
		m.RemoveInitContainer(name)
	}
}

// EditDeploymentSpec records a mutation for the Deployment's top-level spec.
//
// Planning:
// All deployment spec edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, deployment spec edits are executed AFTER deployment-metadata edits but BEFORE pod template/spec/container edits within the same feature.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditDeploymentSpec(edit func(*editors.DeploymentSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.deploymentSpecEdits = append(m.active.deploymentSpecEdits, edit)
}

// EditPodSpec records a mutation for the Deployment's pod spec.
//
// Planning:
// All pod spec edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, pod spec edits are executed AFTER replica/metadata edits but BEFORE container edits within the same feature.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodSpec(edit func(*editors.PodSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.podSpecEdits = append(m.active.podSpecEdits, edit)
}

// EditPodTemplateMetadata records a mutation for the Deployment's pod template metadata.
//
// Planning:
// All pod template metadata edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, pod template metadata edits are executed AFTER replica/deployment-metadata edits but BEFORE pod spec/container edits within the same feature.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.podTemplateMetadataEdits = append(m.active.podTemplateMetadataEdits, edit)
}

// EditObjectMetadata records a mutation for the Deployment's own metadata.
//
// Planning:
// All object metadata edits are stored and executed during Apply().
//
// Execution Order:
//   - Within a feature, edits are applied in registration order.
//   - Overall, object metadata edits are executed BEFORE all other categories within the same feature.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.deploymentMetadataEdits = append(m.active.deploymentMetadataEdits, edit)
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
// Features are applied in the order they were registered.
// Within each feature, mutations are applied in this fixed category order:
// 1. Object metadata edits
// 2. DeploymentSpec edits
// 3. Pod template metadata edits
// 4. Pod spec edits
// 5. Regular container presence operations
// 6. Regular container edits
// 7. Init container presence operations
// 8. Init container edits
//
// Within each category of a single feature, edits are applied in their registration order.
//
// Selection & Identity:
//   - Container selectors target containers in the state they are in at the start of that feature's
//     container phase (after presence operations of the SAME feature have been applied).
//   - Selector matching within a phase is evaluated against a snapshot of containers at the start
//     of that phase, not the progressively mutated live containers.
//   - Later features observe the Deployment as modified by all previous features.
//
// Timing:
// No changes are made to the Deployment until Apply() is called.
// Selectors and edit functions are executed during this pass.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Object metadata
		if len(plan.deploymentMetadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.current.ObjectMeta)
			for _, edit := range plan.deploymentMetadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. DeploymentSpec
		if len(plan.deploymentSpecEdits) > 0 {
			editor := editors.NewDeploymentSpecEditor(&m.current.Spec)
			for _, edit := range plan.deploymentSpecEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 3. Pod template metadata
		if len(plan.podTemplateMetadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.current.Spec.Template.ObjectMeta)
			for _, edit := range plan.podTemplateMetadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 4. Pod spec
		if len(plan.podSpecEdits) > 0 {
			editor := editors.NewPodSpecEditor(&m.current.Spec.Template.Spec)
			for _, edit := range plan.podSpecEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 5. Regular container presence
		for _, op := range plan.containerPresence {
			applyPresenceOp(&m.current.Spec.Template.Spec.Containers, op)
		}

		// 6. Regular container edits
		if len(plan.containerEdits) > 0 {
			// Take snapshot of containers AFTER presence ops but BEFORE applying any edits for stable selector matching
			snapshots := make([]corev1.Container, len(m.current.Spec.Template.Spec.Containers))
			for i := range m.current.Spec.Template.Spec.Containers {
				m.current.Spec.Template.Spec.Containers[i].DeepCopyInto(&snapshots[i])
			}

			for i := range m.current.Spec.Template.Spec.Containers {
				container := &m.current.Spec.Template.Spec.Containers[i]
				snapshot := &snapshots[i]
				editor := editors.NewContainerEditor(container)
				for _, ce := range plan.containerEdits {
					if ce.selector(i, snapshot) {
						if err := ce.edit(editor); err != nil {
							return err
						}
					}
				}
			}
		}

		// 7. Init container presence
		for _, op := range plan.initContainerPresence {
			applyPresenceOp(&m.current.Spec.Template.Spec.InitContainers, op)
		}

		// 8. Init container edits
		if len(plan.initContainerEdits) > 0 {
			// Take snapshot of init containers AFTER presence ops but BEFORE applying any edits
			snapshots := make([]corev1.Container, len(m.current.Spec.Template.Spec.InitContainers))
			for i := range m.current.Spec.Template.Spec.InitContainers {
				m.current.Spec.Template.Spec.InitContainers[i].DeepCopyInto(&snapshots[i])
			}

			for i := range m.current.Spec.Template.Spec.InitContainers {
				container := &m.current.Spec.Template.Spec.InitContainers[i]
				snapshot := &snapshots[i]
				editor := editors.NewContainerEditor(container)
				for _, ce := range plan.initContainerEdits {
					if ce.selector(i, snapshot) {
						if err := ce.edit(editor); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func applyPresenceOp(containers *[]corev1.Container, op containerPresenceOp) {
	found := -1
	for i, c := range *containers {
		if c.Name == op.name {
			found = i
			break
		}
	}

	if op.container == nil {
		// Remove
		if found != -1 {
			*containers = append((*containers)[:found], (*containers)[found+1:]...)
		}
		return
	}

	// Ensure
	if found != -1 {
		(*containers)[found] = *op.container
	} else {
		*containers = append(*containers, *op.container)
	}
}
