// Package resources provides resource implementations for the cronjob primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/cronjob-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/cronjob-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/cronjob"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewCronJobResource constructs a cronjob primitive resource with all the features.
func NewCronJobResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base CronJob object.
	base := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-cronjob",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          "0 2 * * *",
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": owner.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "worker",
									Image: "my-worker:latest", // Will be overwritten by VersionFeature
								},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
						},
					},
				},
			},
		},
	}

	// 2. Initialize the cronjob builder.
	builder := cronjob.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))
	builder.WithMutation(features.MetricsFeature(owner.Spec.EnableMetrics))

	// 4. Configure flavors.
	builder.WithFieldApplicationFlavor(cronjob.PreserveCurrentLabels)
	builder.WithFieldApplicationFlavor(cronjob.PreserveCurrentAnnotations)

	// 5. Data extraction (optional).
	builder.WithDataExtractor(func(cj batchv1.CronJob) error {
		fmt.Printf("Reconciling CronJob: %s, schedule: %s\n", cj.Name, cj.Spec.Schedule)

		y, err := yaml.Marshal(cj)
		if err != nil {
			return fmt.Errorf("failed to marshal cronjob to yaml: %w", err)
		}
		fmt.Printf("Complete CronJob Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
