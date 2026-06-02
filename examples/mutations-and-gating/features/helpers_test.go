package features_test

import (
	"flag"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The mutation-level tests own no golden files, so -update is a no-op here. It is
// declared only so that running the whole example with `go test ./... -update`
// (to regenerate the resource and component goldens) does not fail flag parsing
// in this package's test binary.
var _ = flag.Bool("update", false, "no-op: this package has no golden files")

// baseDeployment returns the minimal baseline Deployment the mutation-level tests
// apply a single mutation to. It carries one container named "app" with the v2
// port layout (http + health), which is the smallest object the deployment
// mutations in this package operate on. Each mutation test starts from this base,
// applies exactly one mutation, previews, and asserts the specific field change.
func baseDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
								{Name: "health", ContainerPort: 8081},
							},
						},
					},
				},
			},
		},
	}
}

// baseConfigMap returns the minimal baseline ConfigMap the ConfigMap
// mutation-level tests apply a single mutation to. It carries the core server
// config in app.yaml, which the metrics mutation merges into.
func baseConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data: map[string]string{
			"app.yaml": "server:\n  port: 8080\n",
		},
	}
}
