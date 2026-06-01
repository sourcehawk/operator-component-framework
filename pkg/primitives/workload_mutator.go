// Package primitives hosts cross-kind contracts shared by the concrete primitive
// packages under pkg/primitives. It depends only on the mutation editor and
// selector packages, never on its own subpackages, so the subpackages can import
// it without creating an import cycle.
package primitives

import (
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	corev1 "k8s.io/api/core/v1"
)

// WorkloadMutator is the editing surface shared by every pod-workload mutator:
// *statefulset.Mutator, *deployment.Mutator, and *daemonset.Mutator. It lets a
// consumer express one workload-kind-agnostic mutation (for example, emitting a
// shared set of environment variables on the application container) and apply it
// to any of those kinds through the per-package LiftMutation adapters.
//
// It is exactly the intersection of the three concrete mutators' editing methods.
// Kind-specific operations are intentionally excluded and remain on the concrete
// types: the spec editors (EditStatefulSetSpec, EditDeploymentSpec,
// EditDaemonSetSpec), EnsureReplicas (absent from the DaemonSet mutator, which
// has no replica field), and the StatefulSet-only VolumeClaimTemplate methods. The lifecycle methods Apply and
// NextFeature are also excluded; they are driven by the framework, not by an
// emitter.
type WorkloadMutator interface {
	EditContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error)
	EditInitContainers(selector selectors.ContainerSelector, edit func(*editors.ContainerEditor) error)
	EnsureContainer(container corev1.Container)
	RemoveContainer(name string)
	RemoveContainers(names []string)
	EnsureInitContainer(container corev1.Container)
	RemoveInitContainer(name string)
	RemoveInitContainers(names []string)
	EditPodSpec(edit func(*editors.PodSpecEditor) error)
	EditPodTemplateMetadata(edit func(*editors.ObjectMetaEditor) error)
	EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error)
	EnsureContainerEnvVar(ev corev1.EnvVar)
	RemoveContainerEnvVar(name string)
	RemoveContainerEnvVars(names []string)
	EnsureContainerArg(arg string)
	RemoveContainerArg(arg string)
	RemoveContainerArgs(args []string)
}
