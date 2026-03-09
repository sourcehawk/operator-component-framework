// Package recording provides utilities for recording Kubernetes events.
package recording

import (
	"fmt"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func typeName(obj any) string {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.GetKind()
	}

	if u, ok := obj.(unstructured.Unstructured); ok {
		return u.GetKind()
	}

	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.Name()
}

func operationReason(op controllerutil.OperationResult, object client.Object) string {
	switch op {
	case controllerutil.OperationResultCreated:
		return "Created" + typeName(object)
	case controllerutil.OperationResultUpdated:
		return "Updated" + typeName(object)
	case controllerutil.OperationResultUpdatedStatus:
		return "UpdatedResourceAndStatus" + typeName(object)
	case controllerutil.OperationResultUpdatedStatusOnly:
		return "UpdatedStatusOnly" + typeName(object)
	default:
		return "Unchanged" + typeName(object)
	}
}

func operationMessage(op controllerutil.OperationResult, object client.Object) string {
	switch op {
	case controllerutil.OperationResultCreated:
		return fmt.Sprintf("Created %s '%s'", typeName(object), object.GetName())
	case controllerutil.OperationResultUpdated:
		return fmt.Sprintf("Updated %s '%s'", typeName(object), object.GetName())
	case controllerutil.OperationResultUpdatedStatus:
		return fmt.Sprintf("Updated resource and status of %s '%s'", typeName(object), object.GetName())
	case controllerutil.OperationResultUpdatedStatusOnly:
		return fmt.Sprintf("Updated status of %s '%s'", typeName(object), object.GetName())
	default:
		return fmt.Sprintf("%s '%s' left unchanged", typeName(object), object.GetName())
	}
}

// RecordCreateOrUpdateOperationEvent records an event for CreateOrUpdate operations on a resource (object) for it's owning
// resource (owner)
//
//   - OperationResultNone: No event is recorded
//   - Other operation results: An event of type Normal is recorded
//   - messageKeyValuePairs are strings expected on the format "myKey=myValue" and are prepended to the generated
//     event message for additional information
func RecordCreateOrUpdateOperationEvent(
	recorder record.EventRecorder, op controllerutil.OperationResult, object client.Object, owner runtime.Object,
	messageKeyValuePairs ...string,
) {
	if op == controllerutil.OperationResultNone {
		return
	}
	message := operationMessage(op, object)
	if len(messageKeyValuePairs) > 0 {
		message = fmt.Sprintf("%s (%s)", message, strings.Join(messageKeyValuePairs, ", "))
	}

	recorder.Eventf(owner, v1.EventTypeNormal, operationReason(op, object), message)
}
