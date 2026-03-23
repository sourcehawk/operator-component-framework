package scope

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCanSetOwnerReference(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		corev1.SchemeGroupVersion,
		rbacv1.SchemeGroupVersion,
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	namespacedOwner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default"},
	}
	clusterOwner := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ns"},
	}
	namespacedResource := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "default"},
	}
	clusterResource := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "child-role"},
	}

	tests := []struct {
		name     string
		owner    client.Object
		owned    client.Object
		expected bool
	}{
		{
			name:     "namespace-scoped owner and namespace-scoped resource",
			owner:    namespacedOwner,
			owned:    namespacedResource,
			expected: true,
		},
		{
			name:     "namespace-scoped owner and cluster-scoped resource",
			owner:    namespacedOwner,
			owned:    clusterResource,
			expected: false,
		},
		{
			name:     "cluster-scoped owner and cluster-scoped resource",
			owner:    clusterOwner,
			owned:    clusterResource,
			expected: true,
		},
		{
			name:     "cluster-scoped owner and namespace-scoped resource",
			owner:    clusterOwner,
			owned:    namespacedResource,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CanSetOwnerReference(tt.owner, tt.owned, scheme, mapper)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("CanSetOwnerReference() = %v, want %v", result, tt.expected)
			}
		})
	}
}
