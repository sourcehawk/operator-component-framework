package component

import (
	"fmt"
	"maps"
	"slices"

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
// finalizer, label or annotation, does not count as a change. A write that sets
// a field inside an empty object that manager owns, such as emptyDir: {}, does
// count.
//
// When the owned fields cannot be known, the whole object is compared instead,
// apart from status, managedFields, resourceVersion, generation and TypeMeta,
// and a concurrent write by another manager does count as a change. That is the
// case when before carries no managedFields, as when the cache strips them, when
// the response holds no apply entry for manager, and when the two entries were
// recorded at different API versions.
func appliedObjectChanged(before, after client.Object, manager string) (bool, error) {
	afterEntry, found := appliedEntry(after, manager)
	if !found || len(before.GetManagedFields()) == 0 {
		return wholeObjectChanged(before, after)
	}

	// No entry for manager before the apply means it owned nothing yet, as when
	// it adopts an object another client created.
	beforeOwned := &fieldpath.Set{}
	if beforeEntry, found := appliedEntry(before, manager); found {
		// Paths are expressed in the version of their entry. A conversion
		// between versions can rename a field, and a path from the other version
		// would then find nothing in either object.
		if beforeEntry.APIVersion != afterEntry.APIVersion {
			return wholeObjectChanged(before, after)
		}
		var err error
		if beforeOwned, err = decodeFieldSet(beforeEntry); err != nil {
			return false, err
		}
	}
	afterOwned, err := decodeFieldSet(afterEntry)
	if err != nil {
		return false, err
	}
	owned := afterOwned.Union(beforeOwned)

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
	owned.Leaves().Iterate(func(path fieldpath.Path) {
		if changed {
			return
		}
		beforeValue, beforeFound := lookupFieldPath(beforeContent, path)
		afterValue, afterFound := lookupFieldPath(afterContent, path)
		changed = beforeFound != afterFound || !equality.Semantic.DeepEqual(beforeValue, afterValue)
	})
	if changed {
		return true, nil
	}

	// A lookup by key finds a list item wherever it sits, so the comparison of
	// values above does not see a reorder. The order of some lists, such as
	// init containers, changes what the object does.
	return ownedListOrderChanged(owned, nil, beforeContent, afterContent), nil
}

// appliedEntry returns the managedFields entry of manager's Server-Side Apply
// to the main resource of obj.
func appliedEntry(obj client.Object, manager string) (metav1.ManagedFieldsEntry, bool) {
	for _, entry := range obj.GetManagedFields() {
		if entry.Manager == manager && entry.Operation == metav1.ManagedFieldsOperationApply && entry.Subresource == "" {
			return entry, true
		}
	}
	return metav1.ManagedFieldsEntry{}, false
}

func decodeFieldSet(entry metav1.ManagedFieldsEntry) (*fieldpath.Set, error) {
	set := &fieldpath.Set{}
	if entry.FieldsV1 == nil {
		return set, nil
	}
	if err := set.FromJSON(entry.FieldsV1.GetRawReader()); err != nil {
		return nil, fmt.Errorf("failed to decode managed fields of %q: %w", entry.Manager, err)
	}
	return set, nil
}

// lookupFieldPath returns the value at path in the unstructured content, and
// whether the path exists there.
func lookupFieldPath(content map[string]any, path fieldpath.Path) (any, bool) {
	var current any = content
	for _, element := range path {
		switch {
		case element.FieldName != nil:
			fields, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			if current, ok = fields[*element.FieldName]; !ok {
				return nil, false
			}
		case element.Index != nil:
			items, ok := current.([]any)
			if !ok || *element.Index < 0 || *element.Index >= len(items) {
				return nil, false
			}
			current = items[*element.Index]
		default:
			items, _ := current.([]any)
			index, ok := listItemIndex(items, element)
			if !ok {
				return nil, false
			}
			current = items[index]
		}
	}
	return current, true
}

// listItemIndex returns the position in items of the item that a Key or Value
// path element selects.
func listItemIndex(items []any, element fieldpath.PathElement) (int, bool) {
	index := slices.IndexFunc(items, func(item any) bool {
		if element.Value != nil {
			return value.Equals(value.NewValueInterface(item), *element.Value)
		}
		if element.Key == nil {
			return false
		}
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
	return index, index >= 0
}

// ownedListOrderChanged reports whether the owned items of any list in owned,
// below prefix, are in a different relative order before and after. Items that
// only other managers own are left out, so another manager inserting an item
// does not count.
func ownedListOrderChanged(owned *fieldpath.Set, prefix fieldpath.Path, before, after map[string]any) bool {
	var items fieldpath.PathElementSet
	collect := func(element fieldpath.PathElement) {
		if element.Key != nil || element.Value != nil {
			items.Insert(element)
		}
	}
	owned.Members.Iterate(collect)
	owned.Children.Iterate(collect)
	if items.Size() > 1 && !slices.Equal(ownedItemOrder(&items, prefix, before), ownedItemOrder(&items, prefix, after)) {
		return true
	}

	changed := false
	owned.Children.Iterate(func(element fieldpath.PathElement) {
		if changed {
			return
		}
		child, _ := owned.Children.Get(element)
		changed = ownedListOrderChanged(child, append(prefix.Copy(), element), before, after)
	})
	return changed
}

// ownedItemOrder returns the items of the list at prefix that are in owned, in
// the order the list holds them. Owned items the list does not hold are left
// out: the comparison of values already reports them.
func ownedItemOrder(owned *fieldpath.PathElementSet, prefix fieldpath.Path, content map[string]any) []string {
	list, _ := lookupFieldPath(content, prefix)
	items, _ := list.([]any)
	positions := map[int]string{}
	owned.Iterate(func(element fieldpath.PathElement) {
		if index, ok := listItemIndex(items, element); ok {
			positions[index] = element.String()
		}
	})
	order := make([]string, 0, len(positions))
	for _, index := range slices.Sorted(maps.Keys(positions)) {
		order = append(order, positions[index])
	}
	return order
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
