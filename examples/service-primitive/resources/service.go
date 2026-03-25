// Package resources provides resource implementations for the service primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/service-primitive/features"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/service"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewServiceResource constructs a service primitive resource with all the features.
func NewServiceResource(owner *sharedapp.ExampleApp) (component.Resource, error) {
	// 1. Create the base Service object.
	base := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-service",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": owner.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// 2. Initialize the service builder.
	builder := service.NewBuilder(base)

	// 3. Register mutations in dependency order.
	builder.WithMutation(features.VersionLabelMutation(owner.Spec.Version))
	builder.WithMutation(features.MetricsPortMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

	// 4. Extract data from the reconciled Service.
	builder.WithDataExtractor(func(svc corev1.Service) error {
		fmt.Printf("Reconciled Service: %s (ClusterIP: %s)\n", svc.Name, svc.Spec.ClusterIP)
		for _, port := range svc.Spec.Ports {
			fmt.Printf("  Port: %s %d -> %s\n", port.Name, port.Port, port.TargetPort.String())
		}
		return nil
	})

	// 5. Build the final resource.
	return builder.Build()
}
