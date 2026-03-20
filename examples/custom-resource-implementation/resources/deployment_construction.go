package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// NewCoreDeployment returns the current latest version of the desired Deployment baseline.
// It encapsulates the core desired state (replicas, containers, etc.) that remains consistent
// across all versions of the component. Feature mutations are then applied on top of this
// baseline to implement version-specific or conditional logic.
func NewCoreDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       name,
				"app.kubernetes.io/managed-by": "example-operator",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                          name,
						"app.kubernetes.io/name":       name,
						"app.kubernetes.io/managed-by": "example-operator",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest", // Example image
							Env: []corev1.EnvVar{
								{Name: "LOG_LEVEL", Value: "info"},
								{Name: "NEW_MANDATORY_SETTING", Value: "standard-value"},
							},
						},
					},
				},
			},
		},
	}
}
