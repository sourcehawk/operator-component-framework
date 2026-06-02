package resources_test

import (
	"flag"
	"os"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// update is wired to the -update flag so both resource generators overwrite their
// goldens and manifests when set:
//
//	go test ./examples/mutations-and-gating/resources/ -update
//
// It is declared once for the whole resources test package; the Deployment and
// ConfigMap generators share it.
var update = flag.Bool("update", false, "update golden files")

// scheme resolves TypeMeta for the rendered resources.
var scheme = newScheme()

// newScheme returns a scheme with the core and apps Kubernetes types registered.
func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

// owner returns a fixture owner. The Build functions overwrite Spec.Version per
// version, so the Version set on a fixture spec is just a placeholder.
func owner(spec sharedapp.ExampleAppSpec) *sharedapp.ExampleApp {
	o := &sharedapp.ExampleApp{Spec: spec}
	o.Name = "my-app"
	o.Namespace = "default"
	return o
}

// deploymentGen declares the Deployment resource matrix. The Deployment registers
// three mutations:
//
//   - BackwardCompatV1Container is version-gated to fire for versions < 2.0.0, so the
//     version sweep splits the default fixture into a pre-2.0.0 regime (the mutation
//     fires) and a 2.0.0 regime (it does not).
//   - DebugLogging is boolean-gated on EnableDebugLogging.
//   - Tracing is boolean-gated on EnableTracing.
//
// One fixture per boolean flag exercises the gate, and the version sweep covers the
// version gate. Together the fixtures' Requires account for all three mutations.
var deploymentGen = goldengen.New(goldengen.Config[*sharedapp.ExampleApp]{
	Dir:      "testdata/deployment",
	Versions: []string{"1.9.0", "2.0.0"},
	Fixtures: []goldengen.Fixture[*sharedapp.ExampleApp]{
		{
			Name: "default",
			Spec: owner(sharedapp.ExampleAppSpec{}),
			Requires: []goldengen.Expect{
				{Name: "BackwardCompatV1Container", For: "1.9.0"}, // legacy container before 2.0.0
			},
			Forbids: []goldengen.Expect{
				{Name: "BackwardCompatV1Container", For: "2.0.0"}, // not from 2.0.0 onward
			},
		},
		{
			Name: "debug",
			Spec: owner(sharedapp.ExampleAppSpec{EnableDebugLogging: true}),
			Requires: []goldengen.Expect{
				{Name: "DebugLogging"}, // boolean-gated on EnableDebugLogging
			},
		},
		{
			Name: "tracing",
			Spec: owner(sharedapp.ExampleAppSpec{EnableTracing: true}),
			Requires: []goldengen.Expect{
				{Name: "Tracing"}, // boolean-gated on EnableTracing
			},
		},
	},
	Build: func(version string, spec *sharedapp.ExampleApp) (goldengen.Unit, error) {
		o := spec.DeepCopyObject().(*sharedapp.ExampleApp)
		o.Spec.Version = version
		res, err := resources.NewDeploymentResource(o)
		if err != nil {
			return nil, err
		}
		return goldengen.Resource(res.(goldengen.ResourcePreviewer), scheme), nil
	},
})

// TestDeploymentVersionMatrix runs the Deployment sweep: it asserts the gating
// expectations and writes or compares one golden per regime plus the coverage
// manifest.
func TestDeploymentVersionMatrix(t *testing.T) {
	deploymentGen.WithUpdate(*update)
	deploymentGen.Run(t)
}

// TestMain runs the package tests, then proves every registered mutation across
// both resource generators is required or excluded before reporting the exit code.
// Chaining the AssertComplete calls means the package fails if either resource
// leaves a mutation unaccounted.
func TestMain(m *testing.M) {
	code := m.Run()
	code = deploymentGen.AssertComplete(code)
	code = configMapGen.AssertComplete(code)
	os.Exit(code)
}
