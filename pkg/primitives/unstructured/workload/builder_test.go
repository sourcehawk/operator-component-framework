package workload

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
		Group: "example.com", Version: "v1", Kind: "Worker",
	})
	obj.SetName("test")
	obj.SetNamespace("default")
	return obj
}

func withRequiredHandlers(b *Builder) *Builder {
	return b.WithCustomConvergeStatus(
		func(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.AliveStatusWithReason, error) {
			return concepts.AliveStatusWithReason{Status: concepts.AliveConvergingStatusHealthy}, nil
		},
	)
}

func TestBuilder_Build_Valid_MinimalHandlers(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_Valid_AllHandlers(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).
		WithCustomGraceStatus(func(_ *uns.Unstructured) (concepts.GraceStatusWithReason, error) {
			return concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded}, nil
		}).
		WithCustomSuspendStatus(func(_ *uns.Unstructured) (concepts.SuspensionStatusWithReason, error) {
			return concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil
		}).
		WithCustomSuspendMutation(func(_ *unstruct.Mutator) error { return nil }).
		WithCustomSuspendDeletionDecision(func(_ *uns.Unstructured) bool { return false }).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_MissingConvergeStatus(t *testing.T) {
	_, err := NewBuilder(validObject()).Build()
	assert.ErrorContains(t, err, "converging status handler is required")
}

func TestBuilder_Build_DefaultGraceStatus_ReportsHealthy(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)

	status, err := res.GraceStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GraceStatusHealthy, status.Status)
}

func TestBuilder_Build_NilObject(t *testing.T) {
	_, err := withRequiredHandlers(NewBuilder(nil)).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_MissingName(t *testing.T) {
	obj := validObject()
	obj.SetName("")
	_, err := withRequiredHandlers(NewBuilder(obj)).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_MissingNamespace(t *testing.T) {
	obj := validObject()
	obj.SetNamespace("")
	_, err := withRequiredHandlers(NewBuilder(obj)).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "ClusterWorker",
	})
	obj.SetName("global")

	res, err := withRequiredHandlers(NewBuilder(obj).MarkClusterScoped()).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/ClusterWorker/global", res.Identity())
}

func TestBuilder_Identity_Namespaced(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/Worker/default/test", res.Identity())
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	called := false
	b := withRequiredHandlers(NewBuilder(validObject()))
	b.WithDataExtractor(func(_ uns.Unstructured) error {
		called = true
		return nil
	})
	res, err := b.Build()
	require.NoError(t, err)
	require.NoError(t, res.ExtractData())
	assert.True(t, called)
}
