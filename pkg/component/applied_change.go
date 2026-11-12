package component

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v6/value"
)

// appliedObjectChanged reports whether a Server-Side Apply by manager changed
// the object, by comparing the object observed before the patch (read through
// the client, usually the informer cache) with the API server's response to it.
//
// Only the fields that manager owns before or after the apply are compared, so
// a write by another manager between the read and the apply, such as a
// finalizer, label or annotation, does not count as a change. A changed owned
// field and an owned field the apply removed both count.
//
// When before carries no managedFields, as when the cache strips them, or the
// response holds no apply entry for manager, the owned fields cannot be known.
// The whole object is then compared instead, apart from status, managedFields,
// resourceVersion, generation and TypeMeta, and a concurrent write by another
// manager does count as a change.
func appliedObjectChanged(before, after client.Object, manager string) (bool, error) {
	afterOwned, found, err := appliedFieldSet(after, manager)
	if err != nil {
		return false, err
	}
	if !found || len(before.GetManagedFields()) == 0 {
		return wholeObjectChanged(before, after)
	}

	// No entry for manager before the apply means it owned nothing yet, as when
	// it adopts an object another client created.
	beforeOwned, _, err := appliedFieldSet(before, manager)
	if err != nil {
		return false, err
	}

	beforeContent, err := runtime.DefaultUnstructuredConverter.ToUnstructured(before)
	if err != nil {
		return false, err
	}
	afterContent, err := runtime.DefaultUnstructuredConverter.ToUnstructured(after)
	if err != nil {
		return false, err
	}

	// A path that has children is compared through them. Comparing the parent
	// would bring back the fields other managers added beside the owned ones.
	// Iterate rather than a range over Set.All: All ignores the result of yield,
	// so leaving the loop early panics.
	changed := false
	afterOwned.Union(beforeOwned).Leaves().Iterate(func(path fieldpath.Path) {
		if changed {
			return
		}
		beforeValue, beforeFound := lookupFieldPath(beforeContent, path)
		afterValue, afterFound := lookupFieldPath(afterContent, path)
		changed = beforeFound != afterFound || !equality.Semantic.DeepEqual(beforeValue, afterValue)
	})
	return changed, nil
}

// appliedFieldSet returns the fields that manager owns on obj through Server-Side
// Apply to the main resource. found is false when obj has no such entry.
func appliedFieldSet(obj client.Object, manager string) (set *fieldpath.Set, found bool, err error) {
	set = &fieldpath.Set{}
	for _, entry := range obj.GetManagedFields() {
		if entry.Manager != manager || entry.Operation != metav1.ManagedFieldsOperationApply || entry.Subresource != "" {
			continue
		}
		if entry.FieldsV1 == nil {
			return set, true, nil
		}
		if err := set.FromJSON(entry.FieldsV1.GetRawReader()); err != nil {
			return nil, false, fmt.Errorf("failed to decode managed fields of %q: %w", manager, err)
		}
		return set, true, nil
	}
	return set, false, nil
}

// lookupFieldPath returns the value at path in the unstructured content, and
// whether the path exists there.
func lookupFieldPath(content map[string]any, path fieldpath.Path) (any, bool) {
	var current any = content
	for _, element := range path {
		var found bool
		switch {
		case element.FieldName != nil:
			fields, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			current, found = fields[*element.FieldName]
		case element.Key != nil:
			current, found = findListItem(current, func(item any) bool {
				fields, ok := item.(map[string]any)
				if !ok {
					return false
				}
				for _, key := range *element.Key {
					field, ok := fields[key.Name]
					if !ok || !value.Equals(value.NewValueInterface(field), key.Value) {
						return false
					}
				}
				return true
			})
		case element.Value != nil:
			current, found = findListItem(current, func(item any) bool {
				return value.Equals(value.NewValueInterface(item), *element.Value)
			})
		case element.Index != nil:
			items, ok := current.([]any)
			if !ok || *element.Index < 0 || *element.Index >= len(items) {
				return nil, false
			}
			current, found = items[*element.Index], true
		}
		if !found {
			return nil, false
		}
	}
	return current, true
}

// findListItem returns the first item of the list for which match is true.
func findListItem(list any, match func(any) bool) (any, bool) {
	items, ok := list.([]any)
	if !ok {
		return nil, false
	}
	for _, item := range items {
		if match(item) {
			return item, true
		}
	}
	return nil, false
}

// wholeObjectChanged compares the two objects on everything apart from status,
// managedFields, resourceVersion, generation and TypeMeta. Those change without
// the desired state changing: other controllers write status, the server bumps
// the bookkeeping fields on any write, fake clients bump resourceVersion even on
// a no-op apply, and TypeMeta depends on how the object was decoded.
func wholeObjectChanged(before, after client.Object) (bool, error) {
	beforeContent, err := comparableContent(before)
	if err != nil {
		return false, err
	}
	afterContent, err := comparableContent(after)
	if err != nil {
		return false, err
	}
	return !equality.Semantic.DeepEqual(beforeContent, afterContent), nil
}

// comparableContent renders an object as an unstructured map with the fields
// wholeObjectChanged ignores removed. It copies the top-level and metadata maps
// before deleting keys so the object itself is never mutated (for
// *unstructured.Unstructured, ToUnstructured returns the object's own content).
func comparableContent(obj client.Object) (map[string]any, error) {
	converted, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	content := make(map[string]any, len(converted))
	for k, v := range converted {
		content[k] = v
	}
	delete(content, "apiVersion")
	delete(content, "kind")
	delete(content, "status")

	if rawMetadata, ok := content["metadata"].(map[string]any); ok {
		metadata := make(map[string]any, len(rawMetadata))
		for k, v := range rawMetadata {
			metadata[k] = v
		}
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "generation")
		content["metadata"] = metadata
	}

	return content, nil
}
