// Package resources provides resource implementations for the HPA primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/hpa-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/hpa-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/hpa"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// NewHPAResource constructs an HPA primitive resource with all the features.
func NewHPAResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base HPA object.
	base := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-hpa",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       owner.Name + "-deployment",
			},
			MinReplicas: ptr.To(int32(2)),
			MaxReplicas: 10,
		},
	}

	// 2. Initialize the HPA builder.
	builder := hpa.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.CPUMetricFeature(owner.Spec.Version, 70))
	builder.WithMutation(features.MemoryMetricFeature(owner.Spec.EnableMetrics, 80))
	builder.WithMutation(features.ScaleBehaviorFeature())

	// 4. Configure flavors.
	builder.WithFieldApplicationFlavor(features.PreserveLabelsFlavor())
	builder.WithFieldApplicationFlavor(features.PreserveAnnotationsFlavor())

	// 5. Data extraction.
	builder.WithDataExtractor(func(h autoscalingv2.HorizontalPodAutoscaler) error {
		fmt.Printf("HPA %s: min=%d, max=%d, metrics=%d\n",
			h.Name,
			derefInt32(h.Spec.MinReplicas, 1),
			h.Spec.MaxReplicas,
			len(h.Spec.Metrics),
		)

		y, err := yaml.Marshal(h)
		if err != nil {
			return fmt.Errorf("failed to marshal HPA to yaml: %w", err)
		}
		fmt.Printf("Complete HPA Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}

func derefInt32(p *int32, defaultVal int32) int32 {
	if p != nil {
		return *p
	}
	return defaultVal
}
