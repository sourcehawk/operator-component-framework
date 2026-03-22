package cronjob

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a cronjob Mutator
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
	cronjobMetadataEdits     []func(*editors.ObjectMetaEditor) error
	cronjobSpecEdits         []func(*editors.CronJobSpecEditor) error
	jobSpecEdits             []func(*editors.JobSpecEditor) error
	podTemplateMetadataEdits []func(*editors.ObjectMetaEditor) error
	podSpecEdits             []func(*editors.PodSpecEditor) error
	containerPresence        []containerPresenceOp
	containerEdits           []containerEdit
	initContainerPresence    []containerPresenceOp
	initContainerEdits       []containerEdit
}

// Mutator is a high-level helper for modifying a Kubernetes CronJob.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, and then
// applied to the CronJob in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
type Mutator struct {
	current *batchv1.CronJob

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given CronJob.
func NewMutator(current *batchv1.CronJob) *Mutator {
	m := &Mutator{
		current: current,
	}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the CronJob's own metadata.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.cronjobMetadataEdits = append(m.active.cronjobMetadataEdits, edit)
}

// EditCronJobSpec records a mutation for the CronJob's top-level spec.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditCronJobSpec(edit func(*editors.CronJobSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.cronjobSpecEdits = append(m.active.cronjobSpecEdits, edit)
}

// EditJobSpec records a mutation for the CronJob's embedded job template spec.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditJobSpec(edit func(*editors.JobSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.jobSpecEdits = append(m.active.jobSpecEdits, edit)
}

// EditPodTemplateMetadata records a mutation for the CronJob's pod template metadata.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.podTemplateMetadataEdits = append(m.active.podTemplateMetadataEdits, edit)
}

// EditPodSpec records a mutation for the CronJob's pod spec.
//
// If the edit function is nil, the registration is ignored.
func (m *Mutator) EditPodSpec(edit func(*editors.PodSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.podSpecEdits = append(m.active.podSpecEdits, edit)
}

// EditContainers records a mutation for containers matching the given selector.
//
// Selector matching is evaluated against a snapshot taken after the current feature's
// container presence operations are applied.
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
// Selector matching is evaluated against a snapshot taken after the current feature's
// init container presence operations are applied.
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

// EnsureContainer records that a regular container must be present in the CronJob.
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

// EnsureInitContainer records that an init container must be present in the CronJob.
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
// in all containers of the CronJob.
func (m *Mutator) EnsureContainerEnvVar(ev corev1.EnvVar) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureEnvVar(ev)
		return nil
	})
}

// RemoveContainerEnvVar records that an environment variable should be
// removed from all containers of the CronJob.
func (m *Mutator) RemoveContainerEnvVar(name string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveEnvVar(name)
		return nil
	})
}

// RemoveContainerEnvVars records that multiple environment variables should be
// removed from all containers of the CronJob.
func (m *Mutator) RemoveContainerEnvVars(names []string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveEnvVars(names)
		return nil
	})
}

// EnsureContainerArg records that a command-line argument must be present
// in all containers of the CronJob.
func (m *Mutator) EnsureContainerArg(arg string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.EnsureArg(arg)
		return nil
	})
}

// RemoveContainerArg records that a command-line argument should be
// removed from all containers of the CronJob.
func (m *Mutator) RemoveContainerArg(arg string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveArg(arg)
		return nil
	})
}

// RemoveContainerArgs records that multiple command-line arguments should be
// removed from all containers of the CronJob.
func (m *Mutator) RemoveContainerArgs(args []string) {
	m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
		e.RemoveArgs(args)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying CronJob.
//
// Execution Order:
// Features are applied in the order they were registered.
// Within each feature, mutations are applied in this fixed category order:
//  1. Object metadata edits
//  2. CronJobSpec edits
//  3. JobSpec edits
//  4. Pod template metadata edits
//  5. Pod spec edits
//  6. Regular container presence operations
//  7. Regular container edits
//  8. Init container presence operations
//  9. Init container edits
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		if err := m.applyPlan(plan); err != nil {
			return err
		}
	}

	return nil
}

func (m *Mutator) applyPlan(plan featurePlan) error {
	// 1. Object metadata
	if len(plan.cronjobMetadataEdits) > 0 {
		editor := editors.NewObjectMetaEditor(&m.current.ObjectMeta)
		for _, edit := range plan.cronjobMetadataEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 2. CronJobSpec
	if len(plan.cronjobSpecEdits) > 0 {
		editor := editors.NewCronJobSpecEditor(&m.current.Spec)
		for _, edit := range plan.cronjobSpecEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 3. JobSpec
	if len(plan.jobSpecEdits) > 0 {
		editor := editors.NewJobSpecEditor(&m.current.Spec.JobTemplate.Spec)
		for _, edit := range plan.jobSpecEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 4. Pod template metadata
	if len(plan.podTemplateMetadataEdits) > 0 {
		editor := editors.NewObjectMetaEditor(&m.current.Spec.JobTemplate.Spec.Template.ObjectMeta)
		for _, edit := range plan.podTemplateMetadataEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 5. Pod spec
	if len(plan.podSpecEdits) > 0 {
		editor := editors.NewPodSpecEditor(&m.current.Spec.JobTemplate.Spec.Template.Spec)
		for _, edit := range plan.podSpecEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 6. Regular container presence
	for _, op := range plan.containerPresence {
		applyPresenceOp(&m.current.Spec.JobTemplate.Spec.Template.Spec.Containers, op)
	}

	// 7. Regular container edits
	if err := applyContainerEdits(m.current.Spec.JobTemplate.Spec.Template.Spec.Containers, plan.containerEdits); err != nil {
		return err
	}

	// 8. Init container presence
	for _, op := range plan.initContainerPresence {
		applyPresenceOp(&m.current.Spec.JobTemplate.Spec.Template.Spec.InitContainers, op)
	}

	// 9. Init container edits
	return applyContainerEdits(m.current.Spec.JobTemplate.Spec.Template.Spec.InitContainers, plan.initContainerEdits)
}

func applyContainerEdits(containers []corev1.Container, edits []containerEdit) error {
	if len(edits) == 0 {
		return nil
	}

	snapshots := make([]corev1.Container, len(containers))
	for i := range containers {
		containers[i].DeepCopyInto(&snapshots[i])
	}

	for i := range containers {
		container := &containers[i]
		snapshot := &snapshots[i]
		editor := editors.NewContainerEditor(container)
		for _, ce := range edits {
			if ce.selector(i, snapshot) {
				if err := ce.edit(editor); err != nil {
					return err
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
