package concepts_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type stubPreviewable struct{ obj client.Object }

func (s stubPreviewable) Preview() (client.Object, error) { return s.obj, nil }

// Compile-time assertion that the interface shape is what we expect.
var _ concepts.Previewable = stubPreviewable{}

func TestPreviewableConformance(t *testing.T) {
	var p concepts.Previewable = stubPreviewable{obj: &appsv1.Deployment{}}
	obj, err := p.Preview()
	require.NoError(t, err)
	require.NotNil(t, obj)
}
