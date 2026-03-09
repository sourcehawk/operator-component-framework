package exampleapp

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ExamplePlatform is a mock of a Custom Resource that owns our component.
// It implements the component.OperatorCRD interface, providing access to
// status conditions and basic metadata, which the Component Framework uses
// for reporting reconciliation progress.
//
//nolint:modernize
type ExamplePlatform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExamplePlatformSpec   `json:"spec,omitempty"`
	Status ExamplePlatformStatus `json:"status,omitempty"`
}

// ExamplePlatformSpec defines the desired state of the mock resource.
type ExamplePlatformSpec struct {
	// Version of the application to deploy, used for Feature Mutation gating.
	Version string `json:"version,omitempty"`
	// EnableTracing determines if the tracing feature should be enabled.
	// This demonstrates the use of additional boolean conditions in Feature Mutations.
	EnableTracing bool `json:"enableTracing,omitempty"`
	// Suspended determines if the component should be scaled down or deleted.
	Suspended bool `json:"suspended,omitempty"`
}

// ExamplePlatformStatus defines the observed state of the mock resource.
type ExamplePlatformStatus struct {
	// Conditions is a list of status conditions, managed by the Component Framework.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetStatusConditions returns the list of conditions from the status, implementing OperatorCRD.
func (cp *ExamplePlatform) GetStatusConditions() *[]metav1.Condition {
	return &cp.Status.Conditions
}

// GetKind returns the resource kind, used for condition messaging and logging.
func (cp *ExamplePlatform) GetKind() string {
	return "ExamplePlatform"
}

// DeepCopyObject implements runtime.Object.
func (cp *ExamplePlatform) DeepCopyObject() runtime.Object {
	// Simplified mock deep copy
	return cp
}
