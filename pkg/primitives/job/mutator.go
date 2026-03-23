// Package job provides a builder and resource for managing Kubernetes Jobs.
package job

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a job Mutator
// only if its associated feature gate is enabled.
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
	jobMetadataEdits         []func(*editors.ObjectMetaEditor) error
	jobSpecEdits             []func(*editors.JobSpecEditor) error
	podTemplateMetadataEdits []func(*editors.ObjectMetaEditor) error
	podSpecEdits             []func(*editors.PodSpecEditor) error
	containerPresence        []containerPresenceOp
	containerEdits           []containerEdit
	initContainerPresence    []containerPresenceOp
	initContainerEdits       []containerEdit
}

// Mutator is a high-level helper for modifying a Kubernetes Job.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then applied
// to the Job in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	current *batchv1.Job

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Job.
func NewMutator(current *batchv1.Job) *Mutator {
	m := &Mutator{
		current: current,
		plans:   []featurePlan{{}},
	}
	m.active = &m.plans[0]
	return m
}

// BeginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) BeginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the Job's own metadata.
//
// Metadata edits are applied before all other categories within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.jobMetadataEdits = append(m.active.jobMetadataEdits, edit)
}

// EditJobSpec records a mutation for the Job's top-level spec.
//
// Job spec edits are applied after metadata edits but before pod template edits
// within the same feature. A nil edit function is ignored.
func (m *Mutator) EditJobSpec(edit func(*editors.JobSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.jobSpecEdits = append(m.active.jobSpecEdits, edit)
}

// EditPodTemplateMetadata records a mutation for the Job's pod template metadata.
//
// Pod template metadata edits are applied after job spec edits but before pod spec
// edits within the same feature. A nil edit function is ignored.
func (m *Mutator) EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.podTemplateMetadataEdits = append(m.active.podTemplateMetadataEdits, edit)
}

// EditPodSpec records a mutation for the Job's pod spec.
//
// Pod spec edits are applied after pod template metadata edits but before container
// edits within the same feature. A nil edit function is ignored.
func (m *Mutator) EditPodSpec(edit func(*editors.PodSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.podSpecEdits = append(m.active.podSpecEdits, edit)
}

// EditContainers records a mutation for containers matching the given selector.
//
// Edits are applied after container presence operations within the same feature.
// Selector matching is evaluated against a snapshot taken after the current
// feature's container presence operations have been applied.
//
// If either selector or edit function is nil, the registration is ignored.
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
// Edits are applied after init container presence operations within the same feature.
// Selector matching is evaluated against a snapshot taken after the current
// feature's init container presence operations have been applied.
//
// If either selector or edit function is nil, the registration is ignored.
func (m *Mutator) EditInitContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error) {
	if selector == nil || edit == nil {
		return
	}
	m.active.initContainerEdits = append(m.active.initContainerEdits, containerEdit{
		selector: selector,
		edit:     edit,
	})
}

// EnsureContainer records that a regular container must be present in the Job.
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

// EnsureInitContainer records that an init container must be present in the Job.
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

// EnsureContainerEnvVar records that an environment variable must be present
// in all containers of the Job.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) EnsureContainerEnvVar(ev corev1.EnvVar) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureEnvVar(ev)
		return nil
	})
}

// RemoveContainerEnvVar records that an environment variable should be
// removed from all containers of the Job.
//
// This is a convenience wrapper over EditContainers.
func (m *Mutator) RemoveContainerEnvVar(name string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveEnvVar(name)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying Job.
//
// Execution order across all registered features:
//
//  1. Object metadata edits
//  2. Job spec edits
//  3. Pod template metadata edits
//  4. Pod spec edits
//  5. Regular container presence operations
//  6. Regular container edits
//  7. Init container presence operations
//  8. Init container edits
//
// Features are applied in the order they were registered. Within each category
// of a single feature, edits are applied in their registration order.
//
// Container selectors are evaluated against a snapshot taken after the current
// feature's container presence operations have been applied.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Object metadata
		if len(plan.jobMetadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.current.ObjectMeta)
			for _, edit := range plan.jobMetadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Job spec
		if len(plan.jobSpecEdits) > 0 {
			editor := editors.NewJobSpecEditor(&m.current.Spec)
			for _, edit := range plan.jobSpecEdits {
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
			// Take snapshot of containers AFTER presence ops but BEFORE applying any edits
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
