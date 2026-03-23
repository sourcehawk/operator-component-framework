package editors

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicySpecEditor provides a typed API for mutating a Kubernetes
// NetworkPolicySpec.
//
// It exposes structured operations for managing the pod selector, ingress rules,
// egress rules, and policy types, as well as Raw() for free-form access when
// none of the structured methods are sufficient.
//
// Note: ingress and egress rules have no unique key. EnsureIngressRule and
// EnsureEgressRule append unconditionally. Use RemoveIngressRules or
// RemoveEgressRules to replace the full set atomically, or use Raw() for
// fine-grained manipulation.
type NetworkPolicySpecEditor struct {
	spec *networkingv1.NetworkPolicySpec
}

// NewNetworkPolicySpecEditor creates a new NetworkPolicySpecEditor wrapping the
// given NetworkPolicySpec pointer.
func NewNetworkPolicySpecEditor(spec *networkingv1.NetworkPolicySpec) *NetworkPolicySpecEditor {
	return &NetworkPolicySpecEditor{spec: spec}
}

// Raw returns the underlying *networkingv1.NetworkPolicySpec.
func (e *NetworkPolicySpecEditor) Raw() *networkingv1.NetworkPolicySpec {
	return e.spec
}

// SetPodSelector sets the pod selector for the NetworkPolicy.
//
// The selector determines which pods this policy applies to within the
// namespace. An empty LabelSelector matches all pods in the namespace.
func (e *NetworkPolicySpecEditor) SetPodSelector(selector metav1.LabelSelector) {
	e.spec.PodSelector = selector
}

// EnsureIngressRule appends an ingress rule to the NetworkPolicy.
//
// Rules have no unique key, so this method always appends. To replace the
// full set of ingress rules atomically, call RemoveIngressRules first and
// then add the desired rules, or use Raw() for fine-grained manipulation.
func (e *NetworkPolicySpecEditor) EnsureIngressRule(rule networkingv1.NetworkPolicyIngressRule) {
	e.spec.Ingress = append(e.spec.Ingress, rule)
}

// RemoveIngressRules clears all ingress rules from the NetworkPolicy.
//
// Use this before calling EnsureIngressRule to replace the full set atomically.
func (e *NetworkPolicySpecEditor) RemoveIngressRules() {
	e.spec.Ingress = nil
}

// EnsureEgressRule appends an egress rule to the NetworkPolicy.
//
// Rules have no unique key, so this method always appends. To replace the
// full set of egress rules atomically, call RemoveEgressRules first and
// then add the desired rules, or use Raw() for fine-grained manipulation.
func (e *NetworkPolicySpecEditor) EnsureEgressRule(rule networkingv1.NetworkPolicyEgressRule) {
	e.spec.Egress = append(e.spec.Egress, rule)
}

// RemoveEgressRules clears all egress rules from the NetworkPolicy.
//
// Use this before calling EnsureEgressRule to replace the full set atomically.
func (e *NetworkPolicySpecEditor) RemoveEgressRules() {
	e.spec.Egress = nil
}

// SetPolicyTypes sets the policy types for the NetworkPolicy.
//
// Valid values are networkingv1.PolicyTypeIngress and networkingv1.PolicyTypeEgress.
// When networkingv1.PolicyTypeEgress is included in PolicyTypes, the .Egress field
// (egress rules) must be set explicitly to permit traffic; an empty list denies all egress.
func (e *NetworkPolicySpecEditor) SetPolicyTypes(types []networkingv1.PolicyType) {
	e.spec.PolicyTypes = types
}
