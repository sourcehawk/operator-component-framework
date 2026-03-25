// Package resources provides resource implementations for the networkpolicy primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/networkpolicy-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/networkpolicy"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewNetworkPolicyResource constructs a networkpolicy primitive resource with all the features.
func NewNetworkPolicyResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base NetworkPolicy object.
	base := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-netpol",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": owner.Name,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	// 2. Initialize the networkpolicy builder.
	builder := networkpolicy.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.HTTPIngressMutation())
	builder.WithMutation(features.MetricsIngressMutation(owner.Spec.Version, owner.Spec.EnableMetrics))
	builder.WithMutation(features.DNSEgressMutation())

	// 4. Extract data from the reconciled NetworkPolicy.
	builder.WithDataExtractor(func(np networkingv1.NetworkPolicy) error {
		fmt.Printf("Reconciled NetworkPolicy: %s\n", np.Name)
		fmt.Printf("  PodSelector: %v\n", np.Spec.PodSelector.MatchLabels)
		fmt.Printf("  PolicyTypes: %v\n", np.Spec.PolicyTypes)
		fmt.Printf("  IngressRules: %d\n", len(np.Spec.Ingress))
		fmt.Printf("  EgressRules: %d\n", len(np.Spec.Egress))
		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
