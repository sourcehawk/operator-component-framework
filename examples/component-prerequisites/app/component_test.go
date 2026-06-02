package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/app"
	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// update is this package's own -update flag. The resources package declares its
// own; the two live in separate test binaries, so there is no conflict.
var update = flag.Bool("update", false, "update golden files")

// scheme resolves TypeMeta for the rendered resources in the component stream.
var scheme = newScheme()

// newScheme returns a scheme with the core and apps Kubernetes types registered.
func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

func testOwner() *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{Version: "1.0.0"},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

func testController() *app.Controller {
	return &app.Controller{
		NewConfigMapResource:  resources.NewConfigMapResource,
		NewDeploymentResource: resources.NewDeploymentResource,
	}
}

// TestBuildInfraComponent goldens the infra component the controller reconciles
// first: a single ConfigMap with no prerequisites. The controller and this test
// build the component the same way, so the reconciled component and the snapshot
// stay in lockstep.
func TestBuildInfraComponent(t *testing.T) {
	comp, err := testController().BuildInfraComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/infra-component.yaml", comp,
		golden.WithScheme(scheme), golden.Update(*update))
}

// TestBuildAppComponent goldens the app component the controller reconciles
// second: a Deployment gated behind the InfraReady prerequisite. The DependsOn
// ordering is the point of this example. The controller and this test build the
// component the same way, so the reconciled component and the snapshot stay in
// lockstep.
func TestBuildAppComponent(t *testing.T) {
	comp, err := testController().BuildAppComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/app-component.yaml", comp,
		golden.WithScheme(scheme), golden.Update(*update))
}
