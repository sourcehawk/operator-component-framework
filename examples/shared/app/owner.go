// Package app provides the ExampleApp CRD definition shared across examples.
package app

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ExampleAppSpec defines the desired state of ExampleApp.
type ExampleAppSpec struct {
	// Version of the application to deploy.
	Version string `json:"version"`

	// EnableTracing adds tracing configuration to the application.
	EnableTracing bool `json:"enableTracing"`

	// EnableMetrics adds metrics configuration to the application.
	EnableMetrics bool `json:"enableMetrics"`

	// Suspended determines whether the application is active.
	Suspended bool `json:"suspended"`
}

// ExampleAppStatus defines the observed state of ExampleApp.
type ExampleAppStatus struct {
	// Conditions store the status of the application's components.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExampleApp is a mock CRD representing an application managed by our operator.
type ExampleApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExampleAppSpec   `json:"spec,omitempty"`
	Status ExampleAppStatus `json:"status,omitempty"`
}

// DeepCopyObject implements runtime.Object.
func (in *ExampleApp) DeepCopyObject() runtime.Object {
	out := &ExampleApp{}
	*out = *in
	out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
	copy(out.Status.Conditions, in.Status.Conditions)
	return out
}

// GetStatusConditions returns the status conditions for the ExampleApp.
func (in *ExampleApp) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

// GetKind returns the kind of the object.
func (in *ExampleApp) GetKind() string {
	return "ExampleApp"
}

// ExampleAppList contains a list of ExampleApp.
type ExampleAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExampleApp `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (in *ExampleAppList) DeepCopyObject() runtime.Object {
	out := &ExampleAppList{}
	*out = *in
	if in.Items != nil {
		out.Items = make([]ExampleApp, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*ExampleApp)
		}
	}
	return out
}

var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "apps.example.io", Version: "v1"}

	// SchemeBuilder is used to add Go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &ExampleApp{}, &ExampleAppList{})
		metav1.AddToGroupVersion(scheme, GroupVersion)
		return nil
	})

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
