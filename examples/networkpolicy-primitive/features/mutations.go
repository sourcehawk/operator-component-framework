// Package features provides sample mutations for the networkpolicy primitive example.
package features

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/networkpolicy"
)

// VersionLabelMutation sets the app.kubernetes.io/version label on the
// NetworkPolicy. It is always enabled.
func VersionLabelMutation(version string) networkpolicy.Mutation {
	return networkpolicy.Mutation{
		Name:    "version-label",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *networkpolicy.Mutator) error {
			m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
				e.EnsureLabel("app.kubernetes.io/version", version)
				return nil
			})
			return nil
		},
	}
}

// HTTPIngressMutation adds an ingress rule allowing TCP traffic on port 8080.
// It is always enabled.
func HTTPIngressMutation(version string) networkpolicy.Mutation {
	return networkpolicy.Mutation{
		Name:    "http-ingress",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *networkpolicy.Mutator) error {
			m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
				port := intstr.FromInt32(8080)
				tcp := corev1.ProtocolTCP
				e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &tcp, Port: &port},
					},
				})
				return nil
			})
			return nil
		},
	}
}

// MetricsIngressMutation adds an ingress rule allowing TCP traffic on port 9090
// for Prometheus scraping. It is enabled when enableMetrics is true.
func MetricsIngressMutation(version string, enableMetrics bool) networkpolicy.Mutation {
	return networkpolicy.Mutation{
		Name:    "metrics-ingress",
		Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
		Mutate: func(m *networkpolicy.Mutator) error {
			m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
				port := intstr.FromInt32(9090)
				tcp := corev1.ProtocolTCP
				e.EnsureIngressRule(networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &tcp, Port: &port},
					},
				})
				return nil
			})
			return nil
		},
	}
}

// DNSEgressMutation adds an egress rule allowing UDP traffic on port 53 for DNS
// resolution. It is always enabled.
func DNSEgressMutation(version string) networkpolicy.Mutation {
	return networkpolicy.Mutation{
		Name:    "dns-egress",
		Feature: feature.NewResourceFeature(version, nil),
		Mutate: func(m *networkpolicy.Mutator) error {
			m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
				port := intstr.FromInt32(53)
				udp := corev1.ProtocolUDP
				e.EnsureEgressRule(networkingv1.NetworkPolicyEgressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &port},
					},
				})
				return nil
			})
			return nil
		},
	}
}
