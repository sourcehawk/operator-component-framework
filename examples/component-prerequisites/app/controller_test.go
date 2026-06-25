package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/app"
	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner() *app.ExampleApp {
	owner := &app.ExampleApp{
		Spec: app.ExampleAppSpec{Version: "1.0.0"},
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

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

// TestInfraComponentShape goldens the infra component the controller reconciles.
// It builds through BuildInfraComponent, the same assembly the controller uses,
// so the snapshot reflects the real desired state.
func TestInfraComponentShape(t *testing.T) {
	comp, err := testController().BuildInfraComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/infra-component.yaml", comp,
		golden.WithScheme(testScheme(t)), golden.Update(*update))
}

// TestAppComponentShape goldens the app component the controller reconciles.
// The DependsOn("InfraReady") prerequisite is the point of this example; the
// component is built through BuildAppComponent so the controller and the
// snapshot stay in lockstep.
func TestAppComponentShape(t *testing.T) {
	comp, err := testController().BuildAppComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/app-component.yaml", comp,
		golden.WithScheme(testScheme(t)), golden.Update(*update))
}
