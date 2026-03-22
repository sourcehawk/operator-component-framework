// Package resources provides resource implementations for the ingress primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/ingress-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/ingress-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/ingress"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// NewIngressResource constructs an ingress primitive resource with all the features.
func NewIngressResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base ingress object.
	base := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-ingress",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: owner.Name + ".example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: ptr.To(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: owner.Name + "-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// 2. Initialize the ingress builder.
	builder := ingress.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionAnnotation(owner.Spec.Version))
	builder.WithMutation(features.TLSFeature(owner.Spec.EnableTracing, owner.Name))

	// 4. Configure flavors.
	builder.WithFieldApplicationFlavor(ingress.PreserveCurrentLabels)
	builder.WithFieldApplicationFlavor(ingress.PreserveCurrentAnnotations)

	// 5. Data extraction.
	builder.WithDataExtractor(func(ing networkingv1.Ingress) error {
		fmt.Printf("Reconciling ingress: %s\n", ing.Name)

		y, err := yaml.Marshal(ing)
		if err != nil {
			return fmt.Errorf("failed to marshal ingress to yaml: %w", err)
		}
		fmt.Printf("Complete Ingress Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
