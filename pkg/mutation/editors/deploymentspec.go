package editors

import (
	appsv1 "k8s.io/api/apps/v1"
)

// DeploymentSpecEditor provides a typed API for mutating a Kubernetes DeploymentSpec.
type DeploymentSpecEditor struct {
	spec *appsv1.DeploymentSpec
}

// NewDeploymentSpecEditor creates a new DeploymentSpecEditor for the given DeploymentSpec.
func NewDeploymentSpecEditor(spec *appsv1.DeploymentSpec) *DeploymentSpecEditor {
	return &DeploymentSpecEditor{spec: spec}
}

// Raw returns the underlying *appsv1.DeploymentSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *DeploymentSpecEditor) Raw() *appsv1.DeploymentSpec {
	return e.spec
}

// SetReplicas sets the number of replicas for the deployment.
func (e *DeploymentSpecEditor) SetReplicas(replicas int32) {
	e.spec.Replicas = &replicas
}

// SetPaused sets the paused field of the deployment.
func (e *DeploymentSpecEditor) SetPaused(paused bool) {
	e.spec.Paused = paused
}

// SetMinReadySeconds sets the minimum number of seconds for which a newly created pod should be ready
// without any of its container crashing, for it to be considered available.
func (e *DeploymentSpecEditor) SetMinReadySeconds(seconds int32) {
	e.spec.MinReadySeconds = seconds
}

// SetRevisionHistoryLimit sets the number of old ReplicaSets to retain to allow rollback.
func (e *DeploymentSpecEditor) SetRevisionHistoryLimit(limit int32) {
	e.spec.RevisionHistoryLimit = &limit
}

// SetProgressDeadlineSeconds sets the maximum time in seconds for a deployment to make progress before it
// is considered to be failed.
func (e *DeploymentSpecEditor) SetProgressDeadlineSeconds(seconds int32) {
	e.spec.ProgressDeadlineSeconds = &seconds
}
