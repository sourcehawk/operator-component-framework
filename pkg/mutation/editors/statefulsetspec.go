package editors

import (
	appsv1 "k8s.io/api/apps/v1"
)

// StatefulSetSpecEditor provides a typed API for mutating a Kubernetes StatefulSetSpec.
type StatefulSetSpecEditor struct {
	spec *appsv1.StatefulSetSpec
}

// NewStatefulSetSpecEditor creates a new StatefulSetSpecEditor for the given StatefulSetSpec.
func NewStatefulSetSpecEditor(spec *appsv1.StatefulSetSpec) *StatefulSetSpecEditor {
	return &StatefulSetSpecEditor{spec: spec}
}

// Raw returns the underlying *appsv1.StatefulSetSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *StatefulSetSpecEditor) Raw() *appsv1.StatefulSetSpec {
	return e.spec
}

// SetReplicas sets the number of desired replicas for the StatefulSet.
func (e *StatefulSetSpecEditor) SetReplicas(replicas int32) {
	e.spec.Replicas = &replicas
}

// SetServiceName sets the name of the governing Service for the StatefulSet.
func (e *StatefulSetSpecEditor) SetServiceName(name string) {
	e.spec.ServiceName = name
}

// SetPodManagementPolicy sets the policy for creating pods under the StatefulSet.
func (e *StatefulSetSpecEditor) SetPodManagementPolicy(policy appsv1.PodManagementPolicyType) {
	e.spec.PodManagementPolicy = policy
}

// SetUpdateStrategy sets the update strategy for the StatefulSet.
func (e *StatefulSetSpecEditor) SetUpdateStrategy(strategy appsv1.StatefulSetUpdateStrategy) {
	e.spec.UpdateStrategy = strategy
}

// SetRevisionHistoryLimit sets the number of revisions to retain in the StatefulSet's revision history.
func (e *StatefulSetSpecEditor) SetRevisionHistoryLimit(limit int32) {
	e.spec.RevisionHistoryLimit = &limit
}

// SetMinReadySeconds sets the minimum number of seconds for which a newly created pod should be ready
// before it is considered available.
func (e *StatefulSetSpecEditor) SetMinReadySeconds(seconds int32) {
	e.spec.MinReadySeconds = seconds
}

// SetPersistentVolumeClaimRetentionPolicy sets the policy for retaining PVCs
// when pods are scaled down or the StatefulSet is deleted.
func (e *StatefulSetSpecEditor) SetPersistentVolumeClaimRetentionPolicy(
	policy *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy,
) {
	e.spec.PersistentVolumeClaimRetentionPolicy = policy
}
