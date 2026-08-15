package recording

import (
	"fmt"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// recordedEvent captures every argument handed to [events.EventRecorder.Eventf].
// The client-go fake recorder collapses events into a string and drops the
// related object and the action, which are exactly the fields under test here.
type recordedEvent struct {
	regarding runtime.Object
	related   runtime.Object
	eventType string
	reason    string
	action    string
	note      string
}

// spyRecorder is an [events.EventRecorder] that records events in memory.
type spyRecorder struct {
	events []recordedEvent
}

var _ events.EventRecorder = &spyRecorder{}

func (s *spyRecorder) Eventf(
	regarding, related runtime.Object, eventtype, reason, action, note string, args ...any,
) {
	s.events = append(s.events, recordedEvent{
		regarding: regarding,
		related:   related,
		eventType: eventtype,
		reason:    reason,
		action:    action,
		note:      fmt.Sprintf(note, args...),
	})
}

func Test_RecordResourceOperationEvent(t *testing.T) {
	cases := []struct {
		name            string
		object          client.Object
		owner           client.Object
		operation       concepts.ConvergingOperation
		keyValuePairs   []string
		expectedReason  string
		expectedAction  string
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
			operation:       concepts.ConvergingOperationCreated,
			keyValuePairs:   []string{},
			expectedReason:  "CreatedServiceAccount",
			expectedAction:  "Create",
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
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedDeployment",
			expectedAction:  "Update",
			expectedMessage: "Updated Deployment 'test-deployment' (foo=bar)",
		},
		{
			name: "unchanged object",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       concepts.ConvergingOperationNone,
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
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedXRole",
			expectedAction:  "Update",
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
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{"foo=bar"},
			expectedReason:  "UpdatedXRoleValue",
			expectedAction:  "Update",
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
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{},
			expectedReason:  "UpdatedServiceAccount",
			expectedAction:  "Update",
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
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{},
			expectedReason:  "UpdatedXRoleNested",
			expectedAction:  "Update",
			expectedMessage: "Updated XRoleNested 'test-xrole-nested'",
		},
		{
			name: "key value pairs containing percent signs are not interpreted as format verbs",
			object: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       concepts.ConvergingOperationUpdated,
			keyValuePairs:   []string{"progress=50%", "template=%s"},
			expectedReason:  "UpdatedDeployment",
			expectedAction:  "Update",
			expectedMessage: "Updated Deployment 'test-deployment' (progress=50%, template=%s)",
		},
		{
			name: "unchanged object through custom operation",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-service",
				},
			},
			owner:           &appsv1.Deployment{},
			operation:       concepts.ConvergingOperation("Unknown"),
			keyValuePairs:   []string{},
			expectedReason:  "UnchangedService",
			expectedAction:  "Unchanged",
			expectedMessage: "Service 'test-service' left unchanged",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &spyRecorder{}

			RecordApplyOperationEvent(recorder, tc.operation, tc.object, tc.owner, tc.keyValuePairs...)

			if tc.expectNoEvent {
				assert.Empty(t, recorder.events, "expecting no event")
				return
			}

			require.Len(t, recorder.events, 1)
			event := recorder.events[0]
			assert.Equal(t, tc.owner, event.regarding, "event is recorded on the owner")
			assert.Equal(t, tc.object, event.related, "applied object is attached as the related object")
			assert.Equal(t, corev1.EventTypeNormal, event.eventType)
			assert.Equal(t, tc.expectedReason, event.reason)
			assert.Equal(t, tc.expectedAction, event.action)
			assert.Equal(t, tc.expectedMessage, event.note)
		})
	}
}
