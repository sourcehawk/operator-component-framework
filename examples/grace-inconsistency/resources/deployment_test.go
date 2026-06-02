package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	return s
}

// TestDeploymentShape pins the monitoring Deployment as built by its factory.
// The factory registers a custom grace handler but no mutations, so the golden
// file captures the full desired state. Changes to the base object surface as a
// golden file diff.
func TestDeploymentShape(t *testing.T) {
	owner := testOwner()
	res, err := resources.NewDeploymentResource(owner)
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/deployment.yaml", res.(golden.Previewer),
		golden.WithScheme(testScheme()), golden.Update(*update))
}
