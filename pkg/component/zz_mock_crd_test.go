package component

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MockOperatorCRD is a mock implementation of OperatorCRD for testing.
//
//nolint:modernize
type MockOperatorCRD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MockSpec   `json:"spec,omitempty"`
	Status MockStatus `json:"status,omitempty"`
}

// MockSpec is a minimal spec so tests can apply a typed, JSON-decoded object
// (CRDs are not served as protobuf) with a mutable desired field.
type MockSpec struct {
	Value string `json:"value,omitempty"`
}

type MockOperatorCRDList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []MockOperatorCRD `json:"items"`
}

type MockStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is a non-condition status field. It exists so tests can
	// prove that FlushStatus preserves staged status fields the framework does
	// not manage, including across a conflict retry.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func (m *MockOperatorCRD) GetStatusConditions() *[]metav1.Condition {
	return &m.Status.Conditions
}

func (m *MockOperatorCRD) GetKind() string {
	return "MockOperatorCRD"
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&MockOperatorCRD{},
		&MockOperatorCRDList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

var (
	GroupVersion  = schema.GroupVersion{Group: "test.camunda.io", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
	MockCRD       = &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mockoperatorcrds.test.camunda.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "test.camunda.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     "MockOperatorCRD",
				ListKind: "MockOperatorCRDList",
				Plural:   "mockoperatorcrds",
				Singular: "mockoperatorcrd",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: ptrTo(true),
								},
								"status": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"observedGeneration": {Type: "integer", Format: "int64"},
										"conditions": {
											Type: "array",
											Items: &apiextensionsv1.JSONSchemaPropsOrArray{
												Schema: &apiextensionsv1.JSONSchemaProps{
													Type: "object",
													Properties: map[string]apiextensionsv1.JSONSchemaProps{
														"type":               {Type: "string"},
														"status":             {Type: "string"},
														"reason":             {Type: "string"},
														"message":            {Type: "string"},
														"lastTransitionTime": {Type: "string", Format: "date-time"},
														"observedGeneration": {Type: "integer", Format: "int64"},
													},
													Required: []string{"type", "status", "lastTransitionTime", "reason", "message"},
												},
											},
										},
									},
								},
							},
						},
					},
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
				},
			},
		},
	}
)

func (m *MockOperatorCRD) DeepCopyInto(out *MockOperatorCRD) {
	*out = *m
	out.TypeMeta = m.TypeMeta
	out.ObjectMeta = *m.ObjectMeta.DeepCopy()
	if m.Status.Conditions != nil {
		in, out := &m.Status.Conditions, &out.Status.Conditions
		*out = make([]metav1.Condition, len(*in))
		copy(*out, *in)
	}
}

func (m *MockOperatorCRD) DeepCopy() *MockOperatorCRD {
	if m == nil {
		return nil
	}
	out := new(MockOperatorCRD)
	m.DeepCopyInto(out)
	return out
}

func (m *MockOperatorCRD) DeepCopyObject() runtime.Object {
	if c := m.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (m *MockOperatorCRDList) DeepCopyInto(out *MockOperatorCRDList) {
	*out = *m
	out.TypeMeta = m.TypeMeta
	out.ListMeta = *m.ListMeta.DeepCopy()
	if m.Items != nil {
		in, out := &m.Items, &out.Items
		*out = make([]MockOperatorCRD, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (m *MockOperatorCRDList) DeepCopy() *MockOperatorCRDList {
	if m == nil {
		return nil
	}
	out := new(MockOperatorCRDList)
	m.DeepCopyInto(out)
	return out
}

func (m *MockOperatorCRDList) DeepCopyObject() runtime.Object {
	if c := m.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func ptrTo[T any](v T) *T {
	return &v
}
