package task

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
		Group: "example.com", Version: "v1", Kind: "BatchJob",
	})
	obj.SetName("test")
	obj.SetNamespace("default")
	return obj
}

func withRequiredHandlers(b *Builder) *Builder {
	return b.WithCustomConvergeStatus(
		func(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.CompletionStatusWithReason, error) {
			return concepts.CompletionStatusWithReason{Status: concepts.CompletionStatusCompleted}, nil
		},
	)
}

func TestBuilder_Build_Valid_MinimalHandlers(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuilder_Build_MissingConvergeStatus(t *testing.T) {
	_, err := NewBuilder(validObject()).Build()
	assert.ErrorContains(t, err, "converging status handler is required")
}

func TestBuilder_Build_NilObject(t *testing.T) {
	_, err := withRequiredHandlers(NewBuilder(nil)).Build()
	assert.Error(t, err)
}

func TestBuilder_Build_ClusterScoped(t *testing.T) {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "example.com", Version: "v1", Kind: "ClusterBatchJob",
	})
	obj.SetName("global")

	res, err := withRequiredHandlers(NewBuilder(obj).MarkClusterScoped()).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/ClusterBatchJob/global", res.Identity())
}

func TestBuilder_Identity_Namespaced(t *testing.T) {
	res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
	require.NoError(t, err)
	assert.Equal(t, "example.com/v1/BatchJob/default/test", res.Identity())
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
