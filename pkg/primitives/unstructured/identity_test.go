package unstructured

import (
	"testing"

	"github.com/stretchr/testify/assert"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestMakeIdentityFunc_Namespaced(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1alpha1",
		Kind:    "Database",
	})
	obj.SetNamespace("production")
	obj.SetName("my-db")

	fn := MakeIdentityFunc(false)
	assert.Equal(t, "example.com/v1alpha1/Database/production/my-db", fn(obj))
}

func TestMakeIdentityFunc_Namespaced_DefaultNamespace(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Widget",
	})
	obj.SetName("test")

	fn := MakeIdentityFunc(false)
	assert.Equal(t, "example.com/v1/Widget/default/test", fn(obj))
}

func TestMakeIdentityFunc_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "ClusterWidget",
	})
	obj.SetName("global")

	fn := MakeIdentityFunc(true)
	assert.Equal(t, "example.com/v1/ClusterWidget/global", fn(obj))
}

func TestMakeIdentityFunc_ClusterScoped_IgnoresNamespace(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "ClusterWidget",
	})
	obj.SetNamespace("should-be-ignored")
	obj.SetName("global")

	fn := MakeIdentityFunc(true)
	assert.Equal(t, "example.com/v1/ClusterWidget/global", fn(obj))
}
