package static

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
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

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	obj := validObject()
	obj.SetLabels(map[string]string{"team": "platform"})
	builder := NewBuilder(obj)
	ExtractInto(builder, cell, func(o uns.Unstructured) (string, error) {
		return o.GetLabels()["team"], nil
	})

	res, err := builder.Build()
	require.NoError(t, err)

	produced := res.ProducedData()
	require.Len(t, produced, 1)
	assert.Equal(t, "team-label", produced[0].Name())

	require.NoError(t, res.ExtractData())
	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "platform", v)
}

func TestWithDataGuardAndOptionalDataDeclarations(t *testing.T) {
	t.Parallel()
	guarded := concepts.NewData[string]("db-host")
	optional := concepts.NewData[string]("db-port")
	builder := NewBuilder(validObject()).WithDataGuard(guarded).WithOptionalData(optional)

	res, err := builder.Build()
	require.NoError(t, err)

	consumed := res.ConsumedData()
	require.Len(t, consumed, 2)
	assert.Equal(t, "db-host", consumed[0].Cell.Name())
	assert.False(t, consumed[0].Optional)
	assert.Equal(t, "db-port", consumed[1].Cell.Name())
	assert.True(t, consumed[1].Optional)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.Equal(t, `waiting for data "db-host"`, status.Reason)

	guarded.Set("postgres.default.svc")
	status, err = res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
}
