// Package resources provides resource factories for the custom-resource example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/custom-resource/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CertificateRequestGVK is the GVK for the fictional CertificateRequest CRD.
var CertificateRequestGVK = schema.GroupVersionKind{
	Group:   "cert.example.io",
	Version: "v1",
	Kind:    "CertificateRequest",
}

// BaseCertificateRequest returns the desired-state unstructured object for a
// CertificateRequest. This is the baseline before mutations are applied.
func BaseCertificateRequest(owner *app.ExampleApp) *uns.Unstructured {
	obj := &uns.Unstructured{}
	obj.SetGroupVersionKind(CertificateRequestGVK)
	obj.SetName(owner.Name + "-cert")
	obj.SetNamespace(owner.Namespace)
	return obj
}

// NewCertificateResource constructs an unstructured CertificateRequest with
// mutations that set the spec fields. This demonstrates how to manage any CRD
// without a typed primitive wrapper.
func NewCertificateResource(owner *app.ExampleApp) (component.Resource, error) {
	builder := static.NewBuilder(BaseCertificateRequest(owner))

	builder.WithMutation(unstruct.Mutation{
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
	})

	// A real consumer would receive this cell from the assembly function, the
	// way the extraction-and-guards example wires a shared cell across
	// resource factories. Here nothing downstream reads it, so it is declared
	// locally and only its extracted value is printed.
	dnsNames := concepts.NewData[[]string]("certificate-dns-names")
	static.ExtractInto(builder, dnsNames, func(obj uns.Unstructured) ([]string, error) {
		names, _, _ := uns.NestedStringSlice(obj.Object, "spec", "dnsNames")
		fmt.Printf("  Certificate DNS names: %v\n", names)
		return names, nil
	})

	return builder.Build()
}
