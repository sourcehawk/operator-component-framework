package editors

import (
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PodDisruptionBudgetSpecEditor provides a typed API for mutating a Kubernetes
// PodDisruptionBudgetSpec.
type PodDisruptionBudgetSpecEditor struct {
	spec *policyv1.PodDisruptionBudgetSpec
}

// NewPodDisruptionBudgetSpecEditor creates a new PodDisruptionBudgetSpecEditor for
// the given PodDisruptionBudgetSpec.
func NewPodDisruptionBudgetSpecEditor(spec *policyv1.PodDisruptionBudgetSpec) *PodDisruptionBudgetSpecEditor {
	return &PodDisruptionBudgetSpecEditor{spec: spec}
}

// Raw returns the underlying *policyv1.PodDisruptionBudgetSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *PodDisruptionBudgetSpecEditor) Raw() *policyv1.PodDisruptionBudgetSpec {
	return e.spec
}

// SetMinAvailable sets the minimum number of pods that must be available after an eviction.
// Accepts either an integer or a percentage string (e.g. "50%").
func (e *PodDisruptionBudgetSpecEditor) SetMinAvailable(val intstr.IntOrString) {
	e.spec.MinAvailable = &val
}

// SetMaxUnavailable sets the maximum number of pods that can be unavailable after an eviction.
// Accepts either an integer or a percentage string (e.g. "25%").
func (e *PodDisruptionBudgetSpecEditor) SetMaxUnavailable(val intstr.IntOrString) {
	e.spec.MaxUnavailable = &val
}

// ClearMinAvailable removes the MinAvailable constraint.
func (e *PodDisruptionBudgetSpecEditor) ClearMinAvailable() {
	e.spec.MinAvailable = nil
}

// ClearMaxUnavailable removes the MaxUnavailable constraint.
func (e *PodDisruptionBudgetSpecEditor) ClearMaxUnavailable() {
	e.spec.MaxUnavailable = nil
}

// SetSelector replaces the pod selector used by the PodDisruptionBudget.
func (e *PodDisruptionBudgetSpecEditor) SetSelector(selector *metav1.LabelSelector) {
	e.spec.Selector = selector
}

// SetUnhealthyPodEvictionPolicy sets the unhealthy pod eviction policy.
//
// Valid values are policyv1.IfHealthyBudget and policyv1.AlwaysAllow.
func (e *PodDisruptionBudgetSpecEditor) SetUnhealthyPodEvictionPolicy(
	policy policyv1.UnhealthyPodEvictionPolicyType,
) {
	e.spec.UnhealthyPodEvictionPolicy = &policy
}

// ClearUnhealthyPodEvictionPolicy removes the unhealthy pod eviction policy,
// reverting to the cluster default.
func (e *PodDisruptionBudgetSpecEditor) ClearUnhealthyPodEvictionPolicy() {
	e.spec.UnhealthyPodEvictionPolicy = nil
}
