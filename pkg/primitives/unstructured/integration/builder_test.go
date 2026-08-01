package integration

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func validObject() *uns.Unstructured {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "Gateway",
	})
	obj.SetName("test")
	obj.SetNamespace("default")
	return obj
}

func withRequiredHandlers(b *Builder) *Builder {
	return b.WithCustomOperationalStatus(
		func(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.OperationalStatusWithReason, error) {
			return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
		},
	)
}

func TestBuilder_Build_Valid_MinimalHandlers(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_MissingOperationalStatus(t *testing.T) {
	_, err := NewBuilder(validObject()).Build()
	assert.ErrorContains(t, err, "operational status handler is required")
}

func TestBuilder_Build_NilObject(t *testing.T) {
	_, err := withRequiredHandlers(NewBuilder(nil)).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "ClusterGateway",
	})
	obj.SetName("global")

	res, err := withRequiredHandlers(NewBuilder(obj).MarkClusterScoped()).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/ClusterGateway/global", res.Identity())
}

func TestBuilder_Identity_Namespaced(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/Gateway/default/test", res.Identity())
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	obj := validObject()
	obj.SetLabels(map[string]string{"team": "platform"})
	builder := withRequiredHandlers(NewBuilder(obj))
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
	builder := withRequiredHandlers(NewBuilder(validObject())).WithDataGuard(guarded).WithOptionalData(optional)

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
