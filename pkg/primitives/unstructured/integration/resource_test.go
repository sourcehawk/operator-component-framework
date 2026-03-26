package integration

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func buildResource(t *testing.T) *Resource {
	t.Helper()
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "Gateway",
	})
	obj.SetName("test")
	obj.SetNamespace("default")

	res, err := withRequiredHandlers(NewBuilder(obj)).Build()
	require.NoError(t, err)
	return res
}

func TestResource_Identity(t *testing.T) {
	res := buildResource(t)
	assert.Equal(t, "example.com/v1/Gateway/default/test", res.Identity())
}

func TestResource_Object(t *testing.T) {
	res := buildResource(t)
	obj, err := res.Object()
	require.NoError(t, err)
	assert.Equal(t, "test", obj.GetName())
}

func TestResource_ConvergingStatus(t *testing.T) {
	res := buildResource(t)
	status, err := res.ConvergingStatus(concepts.ConvergingOperationNone)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
}

func TestResource_Mutate_WithContentEdit(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "Gateway",
	})
	obj.SetName("test")
	obj.SetNamespace("default")

	b := withRequiredHandlers(NewBuilder(obj))
	b.WithMutation(unstruct.Mutation{
		Name: "set-port",
		Mutate: func(m *unstruct.Mutator) error {
			m.EditContent(func(e *editors.UnstructuredContentEditor) error {
				return e.SetNestedInt64(8080, "spec", "port")
			})
			return nil
		},
	})

	res, err := b.Build()
	require.NoError(t, err)

	current, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(current))

	u := current.(*uns.Unstructured)
	spec := u.Object["spec"].(map[string]interface{})
	assert.Equal(t, int64(8080), spec["port"])
}

func TestResource_ExtractData(t *testing.T) {
	res := buildResource(t)
	require.NoError(t, res.ExtractData())
}
