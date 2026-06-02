package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/app"
	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/resources"
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

// TestBuildComponent goldens the whole component the controller reconciles. The
// point of this example is grace-inconsistency suppression: the component carries
// a grace period and registers its Deployment with the
// SuppressGraceInconsistencyWarning resource option. The multi-document golden
// pins the rendered desired state of the resource the component applies. The
// controller and this test build the component the same way, so the reconciled
// component and the snapshot stay in lockstep.
func TestBuildComponent(t *testing.T) {
	controller := &app.Controller{
		NewDeploymentResource: resources.NewDeploymentResource,
	}

	comp, err := controller.BuildComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/component.yaml", comp,
		golden.WithScheme(scheme), golden.Update(*update))
}
