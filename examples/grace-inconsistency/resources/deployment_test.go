package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"
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

// TestDeploymentShape pins the monitoring Deployment's baseline. This resource
// has no mutations; changes to the base object surface as a golden file diff.
func TestDeploymentShape(t *testing.T) {
	owner := testOwner()
	res, err := deployment.NewBuilder(resources.BaseDeployment(owner)).Build()
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/deployment.yaml", res,
		golden.WithScheme(testScheme()), golden.Update(*update))
}
