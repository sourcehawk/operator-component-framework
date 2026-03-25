package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNetworkPolicySpecEditor_Raw(t *testing.T) {
	spec := &networkingv1.NetworkPolicySpec{}
	e := NewNetworkPolicySpecEditor(spec)
	assert.Same(t, spec, e.Raw())
}

func TestNetworkPolicySpecEditor_SetPodSelector(t *testing.T) {
	spec := &networkingv1.NetworkPolicySpec{}
	e := NewNetworkPolicySpecEditor(spec)
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "web"},
	}
	e.SetPodSelector(selector)
	assert.Equal(t, selector, spec.PodSelector)
}

func TestNetworkPolicySpecEditor_AppendIngressRule_Appends(t *testing.T) {
	spec := &networkingv1.NetworkPolicySpec{}
	e := NewNetworkPolicySpecEditor(spec)

	port80 := intstr.FromInt32(80)
	port443 := intstr.FromInt32(443)
	tcp := corev1.ProtocolTCP

	rule1 := networkingv1.NetworkPolicyIngressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}},
	}
	rule2 := networkingv1.NetworkPolicyIngressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port443}},
	}

	e.AppendIngressRule(rule1)
	e.AppendIngressRule(rule2)

	assert.Len(t, spec.Ingress, 2)
	assert.Equal(t, rule1, spec.Ingress[0])
	assert.Equal(t, rule2, spec.Ingress[1])
}

func TestNetworkPolicySpecEditor_RemoveIngressRules(t *testing.T) {
	port80 := intstr.FromInt32(80)
	tcp := corev1.ProtocolTCP

	spec := &networkingv1.NetworkPolicySpec{
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}}},
		},
	}
	e := NewNetworkPolicySpecEditor(spec)
	e.RemoveIngressRules()
	assert.Nil(t, spec.Ingress)
}

func TestNetworkPolicySpecEditor_AppendEgressRule_Appends(t *testing.T) {
	spec := &networkingv1.NetworkPolicySpec{}
	e := NewNetworkPolicySpecEditor(spec)

	port53 := intstr.FromInt32(53)
	port443 := intstr.FromInt32(443)
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP

	rule1 := networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &udp, Port: &port53}},
	}
	rule2 := networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port443}},
	}

	e.AppendEgressRule(rule1)
	e.AppendEgressRule(rule2)

	assert.Len(t, spec.Egress, 2)
	assert.Equal(t, rule1, spec.Egress[0])
	assert.Equal(t, rule2, spec.Egress[1])
}

func TestNetworkPolicySpecEditor_RemoveEgressRules(t *testing.T) {
	port53 := intstr.FromInt32(53)
	udp := corev1.ProtocolUDP

	spec := &networkingv1.NetworkPolicySpec{
		Egress: []networkingv1.NetworkPolicyEgressRule{
			{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &udp, Port: &port53}}},
		},
	}
	e := NewNetworkPolicySpecEditor(spec)
	e.RemoveEgressRules()
	assert.Nil(t, spec.Egress)
}

func TestNetworkPolicySpecEditor_SetPolicyTypes(t *testing.T) {
	spec := &networkingv1.NetworkPolicySpec{}
	e := NewNetworkPolicySpecEditor(spec)
	types := []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}
	e.SetPolicyTypes(types)
	assert.Equal(t, types, spec.PolicyTypes)
}

func TestNetworkPolicySpecEditor_ReplaceIngressAtomically(t *testing.T) {
	port80 := intstr.FromInt32(80)
	port443 := intstr.FromInt32(443)
	port8080 := intstr.FromInt32(8080)
	tcp := corev1.ProtocolTCP

	spec := &networkingv1.NetworkPolicySpec{
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port80}}},
			{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port443}}},
		},
	}
	e := NewNetworkPolicySpecEditor(spec)

	e.RemoveIngressRules()
	newRule := networkingv1.NetworkPolicyIngressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port8080}},
	}
	e.AppendIngressRule(newRule)

	assert.Len(t, spec.Ingress, 1)
	assert.Equal(t, newRule, spec.Ingress[0])
}
