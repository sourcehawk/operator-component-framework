package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner() *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{Version: "1.0.0"},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestConfigMapShape pins the database config ConfigMap as built by its factory.
// The factory registers a declared extraction but no mutations, so the golden
// file captures the full desired state. If the base object changes (e.g. new
// keys added or defaults changed), the golden file catches it.
func TestConfigMapShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	owner := testOwner()
	dbHost := concepts.NewData[string]("db-host")
	res, err := resources.NewConfigMapResource(owner, dbHost)
	require.NoError(t, err)

	previewer, ok := res.(golden.Previewer)
	require.True(t, ok)

	golden.AssertYAML(t, "testdata/configmap.yaml", previewer,
		golden.WithScheme(scheme), golden.Update(*update))
}
