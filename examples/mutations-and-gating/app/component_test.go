package app_test

import (
	"flag"
	"os"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/app"
	"github.com/sourcehawk/operator-component-framework/examples/mutations-and-gating/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
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

// owner returns a fixture owner. The Build function overwrites Spec.Version per
// version, so the Version set on a fixture spec is just a placeholder.
func owner(spec sharedapp.ExampleAppSpec) *app.ExampleApp {
	o := &app.ExampleApp{Spec: spec}
	o.Name = "my-app"
	o.Namespace = "default"
	return o
}

// gen declares the component matrix. It builds the whole component the controller
// reconciles (via the shared app.BuildComponent) and goldens the multi-document
// YAML of every resource it would apply.
//
// The component aggregates the registered mutations of both resources, so its four
// registered mutations are BackwardCompatV1Container, DebugLogging, and Tracing from
// the Deployment plus metrics-config from the ConfigMap. The "all" fixture turns
// every flag on at a pre-2.0.0 version so all four fire and are accounted for; the
// "minimal" fixture turns them off at 2.0.0 and forbids them, pinning the boundary
// from the other side.
var gen = goldengen.New(goldengen.Config[*app.ExampleApp]{
	Dir:      "testdata/component",
	Versions: []string{"1.9.0", "2.0.0"},
	Fixtures: []goldengen.Fixture[*app.ExampleApp]{
		{
			Name: "all",
			Spec: owner(sharedapp.ExampleAppSpec{
				EnableDebugLogging: true,
				EnableTracing:      true,
				EnableMetrics:      true,
			}),
			Requires: []goldengen.Expect{
				{Name: "DebugLogging"},
				{Name: "Tracing"},
				{Name: "metrics-config"},
				{Name: "BackwardCompatV1Container", For: "1.9.0"}, // legacy container before 2.0.0
			},
			Forbids: []goldengen.Expect{
				{Name: "BackwardCompatV1Container", For: "2.0.0"}, // not from 2.0.0 onward
			},
		},
		{
			Name: "minimal",
			Spec: owner(sharedapp.ExampleAppSpec{}),
			Forbids: []goldengen.Expect{
				{Name: "DebugLogging"},
				{Name: "Tracing"},
				{Name: "metrics-config"},
				{Name: "BackwardCompatV1Container", For: "2.0.0"},
			},
		},
	},
	Build: func(version string, spec *app.ExampleApp) (goldengen.Unit, error) {
		o := spec.DeepCopyObject().(*app.ExampleApp)
		o.Spec.Version = version
		comp, err := app.BuildComponent(o, resources.NewDeploymentResource, resources.NewConfigMapResource)
		if err != nil {
			return nil, err
		}
		return goldengen.Component(comp, scheme), nil
	},
})

// TestBuildComponentVersionMatrix runs the component sweep: it asserts the gating
// expectations across the composed desired state and writes or compares one golden
// per regime plus the coverage manifest. Unlike the resource-level matrices, which
// pin one resource at a time, this goldens the Deployment and ConfigMap rendered
// together, in the order the component applies them.
func TestBuildComponentVersionMatrix(t *testing.T) {
	gen.WithUpdate(*update)
	gen.Run(t)
}

// TestMain runs the package tests, then proves every registered mutation across the
// composed component is required or excluded before reporting the exit code.
func TestMain(m *testing.M) {
	os.Exit(gen.AssertComplete(m.Run()))
}
