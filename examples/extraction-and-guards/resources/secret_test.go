package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestSecretShape pins the database credentials Secret's baseline shape.
// The guard is not exercised here; this test only verifies the resource's
// desired state before reconciliation.
func TestSecretShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	owner := testOwner()
	res, err := secret.NewBuilder(resources.BaseSecret(owner)).Build()
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/secret.yaml", res,
		golden.WithScheme(scheme), golden.Update(*update))
}
