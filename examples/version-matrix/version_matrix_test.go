// Package versionmatrix is a worked example of goldengen: it sweeps a version
// universe over a single StatefulSet fixture, classifies the versions into gating
// regimes, generates one golden per regime, asserts the gating, and proves every
// registered mutation is accounted for.
package versionmatrix

import (
	"flag"
	"os"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/version-matrix/app"
	"github.com/sourcehawk/operator-component-framework/examples/version-matrix/resources"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// update is wired to the -update flag so the generator overwrites goldens and the
// manifest when set: go test ./examples/version-matrix/ -run TestVersionMatrix -update.
var update = flag.Bool("update", false, "update golden files")

// scheme resolves TypeMeta for the rendered StatefulSet.
var scheme = newScheme()

// newScheme returns a scheme with the apps and core Kubernetes types plus the
// example CRD registered.
func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		panic(err)
	}
	if err := app.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

// defaultCluster returns the fixture spec swept across versions. The Build function
// overwrites Spec.Version per version, so the Version set here is just a placeholder.
func defaultCluster() *app.ExampleApp {
	return &app.ExampleApp{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec:       app.ExampleAppSpec{Version: "0.0.0"},
	}
}

// gen declares the whole version matrix. The Versions are listed ascending so a
// regime's representative lands on the lower inclusive boundary of its gating range.
var gen = goldengen.New(goldengen.Config[*app.ExampleApp]{
	Dir:      "testdata/version_matrix",
	Versions: []string{"8.7.0", "8.8.2", "8.9.0"},
	Scheme:   scheme,
	Fixtures: []goldengen.Fixture[*app.ExampleApp]{{
		Name: "default",
		Spec: defaultCluster(),
		Requires: []goldengen.Expect{
			{Name: "ContainerImage"},                     // fires at every version
			{Name: "ClusterEnv/Pre89", For: "8.8.2"},     // legacy discovery before 8.9
			{Name: "ClusterEnv/Unified89", For: "8.9.0"}, // unified discovery from 8.9
		},
		Forbids: []goldengen.Expect{
			{Name: "ClusterEnv/Unified89", For: "8.8.2"}, // not before the boundary
			{Name: "ClusterEnv/Pre89", For: "8.9.0"},     // not after the boundary
		},
	}},
	Build: func(version string, spec *app.ExampleApp) (goldengen.Unit, error) {
		c := spec.DeepCopyObject().(*app.ExampleApp)
		c.Spec.Version = version
		res, err := resources.NewStatefulSetResource(c)
		if err != nil {
			return nil, err
		}
		return goldengen.Resource(res, scheme), nil
	},
})

// TestVersionMatrix runs the sweep: it asserts the gating expectations and writes or
// compares one golden per regime plus the coverage manifest.
func TestVersionMatrix(t *testing.T) {
	gen.WithUpdate(*update)
	gen.Run(t)
}

// TestMain runs the package tests, then proves every registered mutation is required
// or excluded before reporting the final exit code.
func TestMain(m *testing.M) {
	os.Exit(gen.AssertComplete(m.Run()))
}
