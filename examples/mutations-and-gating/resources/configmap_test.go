package resources_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
)

// configMapGen declares the ConfigMap resource matrix. The ConfigMap registers one
// mutation, "metrics-config", boolean-gated on EnableMetrics. The default fixture
// pins that it does not fire with metrics off; the metrics fixture pins that it
// fires with metrics on, which is what accounts for the mutation in AssertComplete.
//
// The version sweep is the same universe as the Deployment, but no ConfigMap
// mutation is version-gated, so both versions collapse to a single regime per
// fixture and only one golden is written per fixture.
var configMapGen = goldengen.New(goldengen.Config[*sharedapp.ExampleApp]{
	Dir:      "testdata/configmap",
	Versions: []string{"1.9.0", "2.0.0"},
	Fixtures: []goldengen.Fixture[*sharedapp.ExampleApp]{
		{
			Name: "default",
			Spec: owner(sharedapp.ExampleAppSpec{}),
			Forbids: []goldengen.Expect{
				{Name: "metrics-config"}, // metrics off, so it never fires
			},
		},
		{
			Name: "metrics",
			Spec: owner(sharedapp.ExampleAppSpec{EnableMetrics: true}),
			Requires: []goldengen.Expect{
				{Name: "metrics-config"}, // boolean-gated on EnableMetrics
			},
		},
	},
	Build: func(version string, spec *sharedapp.ExampleApp) (goldengen.Unit, error) {
		o := spec.DeepCopyObject().(*sharedapp.ExampleApp)
		o.Spec.Version = version
		res, err := resources.NewConfigMapResource(o)
		if err != nil {
			return nil, err
		}
		return goldengen.Resource(res.(goldengen.ResourcePreviewer), scheme), nil
	},
})

// TestConfigMapVersionMatrix runs the ConfigMap sweep: it asserts the gating
// expectations and writes or compares one golden per regime plus the coverage
// manifest.
func TestConfigMapVersionMatrix(t *testing.T) {
	configMapGen.WithUpdate(*update)
	configMapGen.Run(t)
}
