// Package resources provides resource implementations for the job primitive example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/job-primitive/app"
	"github.com/sourcehawk/operator-component-framework/examples/job-primitive/features"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/job"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// NewJobResource constructs a job primitive resource with all the features.
func NewJobResource(owner *app.ExampleApp) (component.Resource, error) {
	// 1. Create the base Job object.
	base := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-migration",
			Namespace: owner.Namespace,
			Labels: map[string]string{
				"app": owner.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": owner.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  "migrate",
							Image: "my-app-migration:latest", // Will be overwritten by VersionFeature
						},
					},
				},
			},
		},
	}

	// 2. Initialize the job builder.
	builder := job.NewBuilder(base)

	// 3. Apply mutations (features) based on the owner spec.
	builder.WithMutation(features.VersionFeature(owner.Spec.Version))
	builder.WithMutation(features.TracingFeature(owner.Spec.EnableTracing))
	builder.WithMutation(features.RetryPolicyFeature(owner.Spec.Version))

	// 4. Configure custom status handler.
	builder.WithCustomConvergeStatus(features.CustomConvergeStatus())

	// 5. Data extraction (optional).
	builder.WithDataExtractor(func(j batchv1.Job) error {
		fmt.Printf("Reconciling job: %s, active: %d, succeeded: %d, failed: %d\n",
			j.Name, j.Status.Active, j.Status.Succeeded, j.Status.Failed)

		// Print the complete job resource object as yaml
		y, err := yaml.Marshal(j)
		if err != nil {
			return fmt.Errorf("failed to marshal job to yaml: %w", err)
		}
		fmt.Printf("Complete Job Resource:\n---\n%s\n---\n", string(y))

		return nil
	})

	// 6. Build the final resource.
	return builder.Build()
}
