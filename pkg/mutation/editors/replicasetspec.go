package editors

import (
	appsv1 "k8s.io/api/apps/v1"
)

// ReplicaSetSpecEditor provides a typed API for mutating a Kubernetes ReplicaSetSpec.
//
// Note: spec.selector is immutable after creation and is not exposed here. Set it via
// the desired object passed to the ReplicaSet builder.
type ReplicaSetSpecEditor struct {
	spec *appsv1.ReplicaSetSpec
}

// NewReplicaSetSpecEditor creates a new ReplicaSetSpecEditor for the given ReplicaSetSpec.
func NewReplicaSetSpecEditor(spec *appsv1.ReplicaSetSpec) *ReplicaSetSpecEditor {
	return &ReplicaSetSpecEditor{spec: spec}
}

// Raw returns the underlying *appsv1.ReplicaSetSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *ReplicaSetSpecEditor) Raw() *appsv1.ReplicaSetSpec {
	return e.spec
}

// SetReplicas sets the number of desired replicas for the ReplicaSet.
func (e *ReplicaSetSpecEditor) SetReplicas(replicas int32) {
	e.spec.Replicas = &replicas
}

// SetMinReadySeconds sets the minimum number of seconds for which a newly created pod should be ready
// without any of its container crashing, for it to be considered available.
func (e *ReplicaSetSpecEditor) SetMinReadySeconds(seconds int32) {
	e.spec.MinReadySeconds = seconds
}
