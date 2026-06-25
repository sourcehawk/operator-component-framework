package app_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/app"
	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
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

// TestComponentShape goldens the database component the controller reconciles.
// The ConfigMap (extractor) and Secret (guard) are wired to a shared dbHost
// pointer through BuildComponent, the same assembly the controller uses, so the
// snapshot reflects the real desired state of both resources in registration
// order.
func TestComponentShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	controller := &app.Controller{
		NewConfigMapResource: resources.NewConfigMapResource,
		NewSecretResource:    resources.NewSecretResource,
	}

	comp, err := controller.BuildComponent(testOwner())
	require.NoError(t, err)

	golden.AssertComponentYAML(t, "testdata/component.yaml", comp,
		golden.WithScheme(scheme), golden.Update(*update))
}
