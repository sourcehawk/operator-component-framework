package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestSecretShape pins the database credentials Secret as built by its
// factory. The factory registers a data guard and a mutation that reads the
// extracted value, so the golden file captures the full desired state
// including the db-host entry the mutation writes. The seeded cell simulates
// the value a preceding ConfigMap extraction would have produced.
func TestSecretShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	owner := testOwner()
	dbHost := concepts.NewData[string]("db-host")
	dbHost.Set("postgres.default.svc")
	res, err := resources.NewSecretResource(owner, dbHost)
	require.NoError(t, err)

	previewer, ok := res.(golden.Previewer)
	require.True(t, ok)

	golden.AssertYAML(t, "testdata/secret.yaml", previewer,
		golden.WithScheme(scheme), golden.Update(*update))
}
