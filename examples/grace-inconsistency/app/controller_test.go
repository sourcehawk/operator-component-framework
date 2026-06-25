package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/app"
	"github.com/sourcehawk/operator-component-framework/examples/grace-inconsistency/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

// TestComponentShape goldens the monitoring component the controller reconciles.
// The grace period and SuppressGraceInconsistencyWarning option are the point of
// this example; the component is built through BuildComponent, the same assembly
// the controller uses, so the snapshot reflects the real desired state.
func TestComponentShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	controller := &app.Controller{
		NewDeploymentResource: resources.NewDeploymentResource,
	}

	comp, err := controller.BuildComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/component.yaml", comp,
		golden.WithScheme(scheme), golden.Update(*update))
}
