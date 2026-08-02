package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/custom-resource/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var update = flag.Bool("update", false, "update golden files")

func testOwner() *sharedapp.ExampleApp {
	owner := &sharedapp.ExampleApp{
		Spec: sharedapp.ExampleAppSpec{Version: "1.0.0"},
	}
	owner.Name = "my-app"
	owner.Namespace = "default"
	return owner
}

// TestCertificateMutations verifies the factory registers the certificate-spec
// mutation and that it fires. The mutation has no gate, so it always applies.
func TestCertificateMutations(t *testing.T) {
	res, err := resources.NewCertificateResource(testOwner())
	require.NoError(t, err)

	inspector, ok := res.(concepts.MutationInspector)
	require.True(t, ok)

	assert.ElementsMatch(t, []string{"certificate-spec"}, inspector.RegisteredMutations())

	firing, err := inspector.FiringSet()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"certificate-spec"}, firing)
}

// TestCertificateShape verifies the CertificateRequest as built by its factory,
// after the certificate-spec mutation sets spec fields (issuerRef, dnsNames)
// and metadata labels. The golden file catches regressions when the mutation
// logic or base object changes.
func TestCertificateShape(t *testing.T) {
	res, err := resources.NewCertificateResource(testOwner())
	require.NoError(t, err)

	previewer, ok := res.(golden.Previewer)
	require.True(t, ok)

	golden.AssertYAML(t, "testdata/certificate.yaml", previewer, golden.Update(*update))
}

// TestCertificateBaseShape pins the bare base object before any mutations.
// This isolates baseline regressions from mutation regressions.
func TestCertificateBaseShape(t *testing.T) {
	owner := testOwner()
	base := resources.BaseCertificateRequest(owner)

	res, err := static.NewBuilder(base).Build()
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/certificate-base.yaml", res, golden.Update(*update))
}

// Verify the unstructured object uses the expected GVK type metadata, since
// golden.AssertYAML relies on it for the apiVersion and kind fields.
func TestCertificateGVK(t *testing.T) {
	owner := testOwner()
	base := resources.BaseCertificateRequest(owner)

	require.Equal(t, resources.CertificateRequestGVK, base.GroupVersionKind())
	require.Equal(t, "my-app-cert", base.GetName())
	require.Equal(t, "default", base.GetNamespace())
}

// Verify the Previewer interface conformance for the unstructured static resource.
func TestCertificatePreview(t *testing.T) {
	owner := testOwner()
	res, err := static.NewBuilder(resources.BaseCertificateRequest(owner)).Build()
	require.NoError(t, err)

	obj, err := res.Preview()
	require.NoError(t, err)
	require.IsType(t, &uns.Unstructured{}, obj)
}
