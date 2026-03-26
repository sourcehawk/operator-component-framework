package static

import (
	"testing"

	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func validObject() *uns.Unstructured {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "Widget",
	})
	obj.SetName("test")
	obj.SetNamespace("default")
	return obj
}

func TestBuilder_Build_Valid(t *testing.T) {
	res, err := NewBuilder(validObject()).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_NilObject(t *testing.T) {
	_, err := NewBuilder(nil).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_MissingName(t *testing.T) {
	obj := validObject()
	obj.SetName("")
	_, err := NewBuilder(obj).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_MissingNamespace(t *testing.T) {
	obj := validObject()
	obj.SetNamespace("")
	_, err := NewBuilder(obj).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "ClusterWidget",
	})
	obj.SetName("global")

	res, err := NewBuilder(obj).MarkClusterScoped().Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_ClusterScoped_RejectsNamespace(t *testing.T) {
	obj := validObject()
	_, err := NewBuilder(obj).MarkClusterScoped().Build()
	assert.Error(t, err)
}

func TestBuilder_Identity_Namespaced(t *testing.T) {
	res, err := NewBuilder(validObject()).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/Widget/default/test", res.Identity())
}

func TestBuilder_Identity_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "ClusterWidget",
	})
	obj.SetName("global")

	res, err := NewBuilder(obj).MarkClusterScoped().Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/ClusterWidget/global", res.Identity())
}

func TestBuilder_WithMutation(t *testing.T) {
	obj := validObject()
	b := NewBuilder(obj)
	b.WithMutation(unstruct.Mutation{
		Name: "test-mutation",
		Mutate: func(m *unstruct.Mutator) error {
			return nil
		},
	})
	res, err := b.Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	obj := validObject()
	called := false
	b := NewBuilder(obj)
	b.WithDataExtractor(func(_ uns.Unstructured) error {
		called = true
		return nil
	})
	res, err := b.Build()
	require.NoError(t, err)
	require.NoError(t, res.ExtractData())
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_NilIgnored(t *testing.T) {
	b := NewBuilder(validObject())
	b.WithDataExtractor(nil)
	res, err := b.Build()
	require.NoError(t, err)
	require.NoError(t, res.ExtractData())
}
