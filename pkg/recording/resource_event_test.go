package recording

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Test_RecordResourceOperationEvent(t *testing.T) {
	cases := []struct {
		name            string
		object          client.Object
		owner           client.Object
		operation       controllerutil.OperationResult
		keyValuePairs   []string
		expectedReason  string
		expectedMessage string
		expectNoEvent   bool
	}{
		{
			name: "created service account",
			object: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service-account",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultCreated,
			keyValuePairs:   []string{},
			expectedReason:  "CreatedServiceAccount",
			expectedMessage: "Created ServiceAccount 'test-service-account'",
		},
		{
			name: "updated deployment",
			object: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedDeployment",
			expectedMessage: "Updated Deployment 'test-deployment' (foo=bar)",
		},
		{
			name: "updated deployment resource and status",
			object: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdatedStatus,
			keyValuePairs:   []string{"foo=bar", "bar=baz"},
			expectedReason:  "UpdatedResourceAndStatusDeployment",
			expectedMessage: "Updated resource and status of Deployment 'test-deployment' (foo=bar, bar=baz)",
		},
		{
			name: "updated encrypted storage status",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdatedStatusOnly,
			keyValuePairs:   []string{},
			expectedReason:  "UpdatedStatusOnlyService",
			expectedMessage: "Updated status of Service 'test-service'",
		},
		{
			name: "unchanged object",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultNone,
			keyValuePairs:   []string{},
			expectedReason:  "",
			expectedMessage: "",
			expectNoEvent:   true,
		},
		{
			name: "updated unstructured (uses GetKind)",
			object: func() client.Object {
				u := &unstructured.Unstructured{}
				u.SetKind("XRole")
				u.SetAPIVersion("v1")
				u.SetName("test-xrole")
				return u
			}(),
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedXRole",
			expectedMessage: "Updated XRole 'test-xrole' (foo=bar)",
		},
		{
			name: "updated unstructured value (uses GetKind)",
			object: func() client.Object {
				u := unstructured.Unstructured{}
				u.SetKind("XRoleValue")
				u.SetAPIVersion("v1")
				u.SetName("test-xrole-value")
				return &u
			}(),
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedXRoleValue",
			expectedMessage: "Updated XRoleValue 'test-xrole-value' (foo=bar)",
		},
		{
			name: "updated nested pointer to service account",
			object: func() client.Object {
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nested-sa",
					},
				}
				p := &sa
				return *p
			}(),
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdated,
			keyValuePairs:   []string{},
			expectedReason:  "UpdatedServiceAccount",
			expectedMessage: "Updated ServiceAccount 'test-nested-sa'",
		},
		{
			name: "updated nested pointer to unstructured",
			object: func() client.Object {
				u := &unstructured.Unstructured{}
				u.SetKind("XRoleNested")
				u.SetAPIVersion("v1")
				u.SetName("test-xrole-nested")
				p := &u
				return *p
			}(),
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResultUpdated,
			keyValuePairs:   []string{},
			expectedReason:  "UpdatedXRoleNested",
			expectedMessage: "Updated XRoleNested 'test-xrole-nested'",
		},
		{
			name: "unchanged object through custom operation result",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       controllerutil.OperationResult("Unknown"),
			keyValuePairs:   []string{},
			expectedReason:  "UnchangedService",
			expectedMessage: "Service 'test-service' left unchanged",
		},
	}

	recorder := record.NewFakeRecorder(1)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RecordCreateOrUpdateOperationEvent(recorder, tc.operation, tc.object, tc.owner, tc.keyValuePairs...)

			if tc.expectNoEvent {
				select {
				case <-recorder.Events:
					t.Errorf("Expecting no event")
				default:
					// all good
				}
			} else {
				select {
				case event := <-recorder.Events:
					fields := strings.SplitN(event, " ", 3)
					assert.Len(t, fields, 3)
					assert.Equal(t, "Normal", fields[0])
					assert.Equal(t, tc.expectedReason, fields[1])
					assert.Equal(t, tc.expectedMessage, fields[2])
				default:
					t.Errorf("Missing event")
				}
			}
		})
	}
}
