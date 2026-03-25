// Package framework provides shared infrastructure for E2E tests.
package framework

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestAppSpec defines the desired state of TestApp.
type TestAppSpec struct {
	// Suspended determines whether the application is suspended.
	Suspended bool `json:"suspended,omitempty"`
}

// TestAppStatus defines the observed state of TestApp.
type TestAppStatus struct {
	// Conditions store the status of the application's components.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// TestApp is the CRD used as the owner object in E2E tests.
type TestApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestAppSpec   `json:"spec,omitempty"`
	Status TestAppStatus `json:"status,omitempty"`
}

// GetStatusConditions returns a pointer to the slice of status conditions.
func (t *TestApp) GetStatusConditions() *[]metav1.Condition {
	return &t.Status.Conditions
}

// GetKind returns the kind of the object.
func (t *TestApp) GetKind() string {
	return "TestApp"
}

// DeepCopyInto copies all properties into another TestApp.
func (t *TestApp) DeepCopyInto(out *TestApp) {
	*out = *t
	out.TypeMeta = t.TypeMeta
	out.ObjectMeta = *t.ObjectMeta.DeepCopy()
	out.Spec = t.Spec
	if t.Status.Conditions != nil {
		in, outC := &t.Status.Conditions, &out.Status.Conditions
		*outC = make([]metav1.Condition, len(*in))
		copy(*outC, *in)
	}
}

// DeepCopy returns a deep copy of the TestApp.
func (t *TestApp) DeepCopy() *TestApp {
	if t == nil {
		return nil
	}
	out := new(TestApp)
	t.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (t *TestApp) DeepCopyObject() runtime.Object {
	return t.DeepCopy()
}

// TestAppList contains a list of TestApp.
type TestAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TestApp `json:"items"`
}

// DeepCopyInto copies all properties into another TestAppList.
func (t *TestAppList) DeepCopyInto(out *TestAppList) {
	*out = *t
	out.TypeMeta = t.TypeMeta
	out.ListMeta = *t.ListMeta.DeepCopy()
	if t.Items != nil {
		in, outI := &t.Items, &out.Items
		*outI = make([]TestApp, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*outI)[i])
		}
	}
}

// DeepCopy returns a deep copy of the TestAppList.
func (t *TestAppList) DeepCopy() *TestAppList {
	if t == nil {
		return nil
	}
	out := new(TestAppList)
	t.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (t *TestAppList) DeepCopyObject() runtime.Object {
	return t.DeepCopy()
}

var (
	// GroupVersion is the group version used to register the TestApp types.
	GroupVersion = schema.GroupVersion{Group: "e2e.ocf.io", Version: "v1alpha1"}

	// SchemeBuilder is used to add Go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &TestApp{}, &TestAppList{})
		metav1.AddToGroupVersion(scheme, GroupVersion)
		return nil
	})

	// AddToScheme adds the TestApp types to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme

	// TestAppCRD is the programmatic CRD definition for TestApp.
	TestAppCRD = &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testapps.e2e.ocf.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "e2e.ocf.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     "TestApp",
				ListKind: "TestAppList",
				Plural:   "testapps",
				Singular: "testapp",
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
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"suspended": {Type: "boolean"},
									},
								},
								"status": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
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
