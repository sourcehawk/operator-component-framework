// Package recording provides utilities for recording Kubernetes events.
package recording

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func typeName(obj any) string {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.GetKind()
	}

	if u, ok := obj.(unstructured.Unstructured); ok {
		return u.GetKind()
	}

	t := reflect.TypeOf(obj)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Name()
}

func applyOperationReason(op concepts.ConvergingOperation, object client.Object) string {
	switch op {
	case concepts.ConvergingOperationCreated:
		return "Created" + typeName(object)
	case concepts.ConvergingOperationUpdated:
		return "Updated" + typeName(object)
	default:
		return "Unchanged" + typeName(object)
	}
}

// applyOperationAction returns the event action verb for the operation. It is
// only meaningful for operations that changed the object; ConvergingOperationNone
// never reaches the recorder.
func applyOperationAction(op concepts.ConvergingOperation) string {
	switch op {
	case concepts.ConvergingOperationCreated:
		return "Create"
	case concepts.ConvergingOperationUpdated:
		return "Update"
	default:
		return "Unchanged"
	}
}

func applyOperationMessage(op concepts.ConvergingOperation, object client.Object) string {
	switch op {
	case concepts.ConvergingOperationCreated:
		return fmt.Sprintf("Created %s '%s'", typeName(object), object.GetName())
	case concepts.ConvergingOperationUpdated:
		return fmt.Sprintf("Updated %s '%s'", typeName(object), object.GetName())
	default:
		return fmt.Sprintf("%s '%s' left unchanged", typeName(object), object.GetName())
	}
}

// RecordApplyOperationEvent records an event for Server-Side Apply operations on a resource (object)
// for its owning resource (owner).
//
// The event is recorded on the owner, with the applied object attached as the event's
// related object and the operation verb ("Create" or "Update") as its action.
//
//   - ConvergingOperationNone: No event is recorded
//   - Other operations: An event of type Normal is recorded
//   - messageKeyValuePairs are strings expected in the format "myKey=myValue" and are prepended to the generated
//     event message for additional information
func RecordApplyOperationEvent(
	recorder events.EventRecorder, op concepts.ConvergingOperation, object client.Object, owner runtime.Object,
	messageKeyValuePairs ...string,
) {
	if op == concepts.ConvergingOperationNone {
		return
	}
	message := applyOperationMessage(op, object)
	if len(messageKeyValuePairs) > 0 {
		message = fmt.Sprintf("%s (%s)", message, strings.Join(messageKeyValuePairs, ", "))
	}

	recorder.Eventf(
		owner, object, v1.EventTypeNormal,
		applyOperationReason(op, object), applyOperationAction(op), "%s", message,
	)
}
