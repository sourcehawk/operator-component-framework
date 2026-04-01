package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/component-prerequisites/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner(version string) *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{Version: version},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestConfigMapShape pins the infra component's ConfigMap baseline. Changes
// to the base object surface as a diff against the golden file.
func TestConfigMapShape(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	owner := testOwner("1.0.0")
	res, err := configmap.NewBuilder(resources.BaseConfigMap(owner)).Build()
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/configmap.yaml", res,
		golden.WithScheme(scheme), golden.Update(*update))
}
