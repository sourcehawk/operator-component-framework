package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestSecretShape pins the database credentials Secret as built by its factory.
// The factory registers a guard but no mutations, so the golden file captures
// the full desired state. The guard is not exercised here; this test only
// verifies the resource's desired state before reconciliation.
func TestSecretShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	owner := testOwner()
	var dbHost string
	res, err := resources.NewSecretResource(owner, &dbHost)
	require.NoError(t, err)

	previewer, ok := res.(golden.Previewer)
	require.True(t, ok, "resource does not implement golden.Previewer")
	golden.AssertYAML(t, "testdata/secret.yaml", previewer,
		golden.WithScheme(scheme), golden.Update(*update))
}
