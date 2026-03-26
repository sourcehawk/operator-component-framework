package unstructured

import (
	"fmt"

	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MakeIdentityFunc constructs an identity function for unstructured objects.
//
// The returned function produces a stable framework identity derived from the
// object's GVK, namespace, and name:
//   - Cluster-scoped: "{group}/{version}/{kind}/{name}"
//   - Namespaced:     "{group}/{version}/{kind}/{namespace}/{name}"
func MakeIdentityFunc(clusterScoped bool) func(*uns.Unstructured) string {
	return func(obj *uns.Unstructured) string {
		gvk := obj.GroupVersionKind()
		if clusterScoped {
			return fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, obj.GetName())
		}
		return fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, obj.GetNamespace(), obj.GetName())
	}
}
