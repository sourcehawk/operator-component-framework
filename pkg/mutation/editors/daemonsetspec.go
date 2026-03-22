package editors

import (
	appsv1 "k8s.io/api/apps/v1"
)

// DaemonSetSpecEditor provides a typed API for mutating a Kubernetes DaemonSetSpec.
type DaemonSetSpecEditor struct {
	spec *appsv1.DaemonSetSpec
}

// NewDaemonSetSpecEditor creates a new DaemonSetSpecEditor for the given DaemonSetSpec.
func NewDaemonSetSpecEditor(spec *appsv1.DaemonSetSpec) *DaemonSetSpecEditor {
	return &DaemonSetSpecEditor{spec: spec}
}

// Raw returns the underlying *appsv1.DaemonSetSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *DaemonSetSpecEditor) Raw() *appsv1.DaemonSetSpec {
	return e.spec
}

// SetUpdateStrategy sets the update strategy for the DaemonSet.
func (e *DaemonSetSpecEditor) SetUpdateStrategy(strategy appsv1.DaemonSetUpdateStrategy) {
	e.spec.UpdateStrategy = strategy
}

// SetMinReadySeconds sets the minimum number of seconds for which a newly created pod
// should be ready without any of its containers crashing, for it to be considered available.
func (e *DaemonSetSpecEditor) SetMinReadySeconds(seconds int32) {
	e.spec.MinReadySeconds = seconds
}

// SetRevisionHistoryLimit sets the number of old DaemonSet revisions to retain to allow rollback.
func (e *DaemonSetSpecEditor) SetRevisionHistoryLimit(limit int32) {
	e.spec.RevisionHistoryLimit = &limit
}
