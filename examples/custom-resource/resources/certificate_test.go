package resources_test

import (
	"flag"
	"testing"

	"github.com/sourcehawk/operator-component-framework/examples/custom-resource/resources"
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
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

// TestCertificateShape verifies the CertificateRequest's shape after mutations
// set spec fields (issuerRef, dnsNames) and metadata labels. The golden file
// catches regressions when the mutation logic or base object changes.
func TestCertificateShape(t *testing.T) {
	owner := testOwner()

	res, err := static.NewBuilder(resources.BaseCertificateRequest(owner)).
		WithMutation(unstruct.Mutation{
			Name: "certificate-spec",
			Mutate: func(m *unstruct.Mutator) error {
				m.EditContent(func(e *editors.UnstructuredContentEditor) error {
					if err := e.SetNestedString("letsencrypt-prod", "spec", "issuerRef", "name"); err != nil {
						return err
					}
					if err := e.SetNestedString("ClusterIssuer", "spec", "issuerRef", "kind"); err != nil {
						return err
					}
					return e.SetNestedSlice(
						[]interface{}{owner.Name + ".example.com"},
						"spec", "dnsNames",
					)
				})

				m.EditObjectMetadata(func(meta *editors.ObjectMetaEditor) error {
					meta.EnsureLabel("app", owner.Name)
					return nil
				})

				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	golden.AssertYAML(t, "testdata/certificate.yaml", res, golden.Update(*update))
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
