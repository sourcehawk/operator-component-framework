package component

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// FlushOption configures a [FlushStatus] call.
type FlushOption func(*flushOptions)

type flushOptions struct {
	// fieldsDeclared is set by any WithOwnedStatusFields option, including one
	// with no paths: declaring that a flush owns no field differs from not
	// declaring at all.
	fieldsDeclared bool
	ownedFields    []string
}

// conditionsMarker is the condition type staged on the owner while
// conditionsJSONPath looks for it in the owner's JSON.
const conditionsMarker = "ocf.sourcehawk.io/flush-status-conditions-marker"

// WithOwnedStatusFields declares the non-condition status fields that this
// flush owns, for an owner whose status is shared with other writers. It turns
// every write of [FlushStatus] into a JSON merge patch of the status that holds
// only the declared fields and the conditions, so a field that another writer
// owns is never sent, on the first attempt or on a retry. Each attempt first
// reads the owner's status through rec.APIReader when set, and through
// rec.Client otherwise; a cache-backed rec.Client can return a stale status.
//
// Each path names a field under status by its JSON name, with nested fields
// separated by dots, for example "observedGeneration" or "summary.ready".
// Paths are relative to status: write "phase", not "status.phase". A declared
// field takes the staged owner's value, and a declared field that the staged
// owner does not carry is removed on the server. A declared object is owned
// whole: a child that the staged owner does not carry is removed too.
//
// Calling it with no paths declares that the flush owns no non-condition status
// field, as for a controller that writes only conditions. Paths from several
// options are combined.
//
// FlushStatus returns an error without writing when a path has an empty
// segment, names the conditions, contains them, or lies below them; condition
// ownership comes from the components passed to FlushStatus. It also fails
// when a declared path runs below a field that the staged owner holds as a
// non-object value. The framework does not check a path against the CRD
// schema: with a structural schema the API server prunes a misspelled field,
// so that part of the write does nothing.
func WithOwnedStatusFields(paths ...string) FlushOption {
	return func(o *flushOptions) {
		o.fieldsDeclared = true
		o.ownedFields = append(o.ownedFields, paths...)
	}
}

// patchOwnedStatus writes the declared fields and the conditions of rec.Owner
// with a merge patch guarded by the staged resourceVersion, and retries on a
// conflict after refreshing the staged owner's resourceVersion and unowned
// conditions from the server.
func patchOwnedStatus(
	ctx context.Context, rec ReconcileContext, ownedConditions map[string]struct{}, rawPaths []string,
) error {
	invalid := func(err error) error {
		return fmt.Errorf(
			"flushing status of %s %s: %w", rec.Owner.GetKind(), client.ObjectKeyFromObject(rec.Owner), err,
		)
	}

	conditionsPath, err := conditionsJSONPath(rec.Owner)
	if err != nil {
		return invalid(err)
	}

	paths, err := parseOwnedStatusPaths(rawPaths, conditionsPath)
	if err != nil {
		return invalid(err)
	}

	staged, err := statusContent(rec.Owner)
	if err != nil {
		return invalid(err)
	}

	// Read every declared path before any request, so a path that cannot be
	// read from the staged owner fails without touching the server.
	for _, path := range paths {
		if _, err := valueAt(staged, path); err != nil {
			return invalid(fmt.Errorf("owned status field %q: %w", strings.Join(path, "."), err))
		}
	}

	server, err := readServerStatus(ctx, rec)
	if err != nil {
		return err
	}

	body, err := ownedStatusPatch(rec.Owner, server, paths, conditionsPath)
	if err != nil {
		return invalid(err)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		patchErr := rec.Client.Status().Patch(ctx, rec.Owner, client.RawPatch(types.MergePatchType, body))
		if patchErr == nil || !apierrors.IsConflict(patchErr) {
			return patchErr
		}

		if refreshErr := refreshUnownedConditions(ctx, rec, ownedConditions); refreshErr != nil {
			return refreshErr
		}

		// Read the status after the refresh, so it is at least as new as the
		// resourceVersion the patch is guarded by.
		var readErr error
		if server, readErr = readServerStatus(ctx, rec); readErr != nil {
			return readErr
		}

		var buildErr error
		if body, buildErr = ownedStatusPatch(rec.Owner, server, paths, conditionsPath); buildErr != nil {
			return buildErr
		}
		return patchErr
	})
}

// readServerStatus returns the status that the server stores for the owner.
// It reads into an unstructured object, because a typed read drops a stored
// zero value that omitempty skips, and the patch must still remove it.
func readServerStatus(ctx context.Context, rec ReconcileContext) (map[string]any, error) {
	gvk, err := apiutil.GVKForObject(rec.Owner, rec.Client.Scheme())
	if err != nil {
		return nil, err
	}

	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(gvk)
	if err := liveReader(rec).Get(ctx, client.ObjectKeyFromObject(rec.Owner), live); err != nil {
		return nil, err
	}

	// Status().Update sends the uid and the API server rejects a mismatch. A
	// merge patch does not, so a retry would patch an owner that was deleted
	// and recreated under the same name, taking its resourceVersion.
	if staged := rec.Owner.GetUID(); staged != "" && live.GetUID() != staged {
		return nil, fmt.Errorf(
			"owner %s was replaced: the server holds uid %s, the staged owner has uid %s",
			client.ObjectKeyFromObject(rec.Owner), live.GetUID(), staged,
		)
	}

	status, _ := live.Object["status"].(map[string]any)
	return status, nil
}

// conditionsJSONPath returns the JSON path under status of the list that
// GetStatusConditions returns. OperatorCRD does not fix that list's JSON name
// or depth, so a marker condition is staged for a moment and found in the
// owner's JSON.
func conditionsJSONPath(owner OperatorCRD) ([]string, error) {
	conditions := owner.GetStatusConditions()
	staged := *conditions
	*conditions = []metav1.Condition{{Type: conditionsMarker}}
	data, err := json.Marshal(owner)
	*conditions = staged
	if err != nil {
		return nil, fmt.Errorf("encoding %T: %w", owner, err)
	}

	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("decoding %T: %w", owner, err)
	}

	status, _ := content["status"].(map[string]any)
	path, found := findConditionsMarker(status)
	if !found {
		return nil, fmt.Errorf("%T does not encode its status conditions under status", owner)
	}
	return path, nil
}

func findConditionsMarker(object map[string]any) ([]string, bool) {
	for key, value := range object {
		switch typed := value.(type) {
		case []any:
			if len(typed) != 1 {
				continue
			}
			if item, ok := typed[0].(map[string]any); ok && item["type"] == conditionsMarker {
				return []string{key}, true
			}
		case map[string]any:
			if path, found := findConditionsMarker(typed); found {
				return append([]string{key}, path...), true
			}
		}
	}
	return nil, false
}

func parseOwnedStatusPaths(paths []string, conditionsPath []string) ([][]string, error) {
	parsed := make([][]string, 0, len(paths))
	for _, path := range paths {
		segments := strings.Split(path, ".")
		if slices.Contains(segments, "") {
			return nil, fmt.Errorf("owned status field %q: path has an empty segment", path)
		}

		// The patch writes the conditions under their own path, so a declared
		// path that overlaps it would collide with that write.
		if isPrefix(segments, conditionsPath) || isPrefix(conditionsPath, segments) {
			return nil, fmt.Errorf(
				"owned status field %q: overlaps the conditions at %q, which are owned through the components passed to FlushStatus",
				path, strings.Join(conditionsPath, "."),
			)
		}
		parsed = append(parsed, segments)
	}

	// A declared ancestor writes its whole subtree. A descendant written into
	// it as well would make the patch depend on the order of the paths.
	var kept [][]string
	for i, path := range parsed {
		covered := slices.ContainsFunc(parsed[:i], func(other []string) bool { return isPrefix(other, path) }) ||
			slices.ContainsFunc(parsed[i+1:], func(other []string) bool {
				return len(other) < len(path) && isPrefix(other, path)
			})
		if !covered {
			kept = append(kept, path)
		}
	}
	return kept, nil
}

func isPrefix(prefix, path []string) bool {
	return len(prefix) <= len(path) && slices.Equal(prefix, path[:len(prefix)])
}

// ownedStatusPatch returns the merge patch body that takes each declared path
// and the conditions from the server's status to the staged owner's, with the
// staged resourceVersion as a precondition.
func ownedStatusPatch(owner OperatorCRD, server map[string]any, paths [][]string, conditionsPath []string) ([]byte, error) {
	staged, err := statusContent(owner)
	if err != nil {
		return nil, err
	}

	status := map[string]any{}
	for _, path := range append(slices.Clone(paths), conditionsPath) {
		value, err := valueAt(staged, path)
		if err != nil {
			return nil, fmt.Errorf("owned status field %q: %w", strings.Join(path, "."), err)
		}

		// A server value below a non-object parent counts as absent.
		current, _ := valueAt(server, path)
		switch typed := value.(type) {
		case nil:
			// A null below a parent the server does not hold as an object
			// would create that parent, so remove only what is there.
			if !holds(server, path) {
				continue
			}
		case map[string]any:
			value = withRemovals(typed, current)
		}
		setPatchValue(status, path, value)
	}

	patch := map[string]any{"status": status}
	if resourceVersion := owner.GetResourceVersion(); resourceVersion != "" {
		patch["metadata"] = map[string]any{"resourceVersion": resourceVersion}
	}
	return json.Marshal(patch)
}

// holds reports whether status has a key at path, below parents that are all
// objects.
func holds(status map[string]any, path []string) bool {
	parent, err := valueAt(status, path[:len(path)-1])
	if err != nil {
		return false
	}

	object, ok := parent.(map[string]any)
	if !ok {
		return false
	}
	_, present := object[path[len(path)-1]]
	return present
}

// statusContent returns the status of obj as JSON values, or nil when obj has
// no status.
func statusContent(obj OperatorCRD) (map[string]any, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encoding %T: %w", obj, err)
	}

	// json.Number re-encodes a number exactly as it was, so an int64 is not
	// rounded through float64.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var content map[string]any
	if err := decoder.Decode(&content); err != nil {
		return nil, fmt.Errorf("decoding %T: %w", obj, err)
	}

	status, _ := content["status"].(map[string]any)
	return status, nil
}

// valueAt returns the value at path in status, or nil when status does not
// carry it, which the patch sends as null to remove the field. An absent or
// null parent counts as absent.
func valueAt(status map[string]any, path []string) (any, error) {
	var current any = status
	for i, segment := range path {
		if current == nil {
			return nil, nil
		}

		object, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New(strings.Join(path[:i], ".") + " is not an object in the staged status")
		}
		current = object[segment]
	}
	return current, nil
}

// withRemovals returns staged with null for each key that server holds and
// staged does not, at any depth, so the merge patch removes it.
func withRemovals(staged map[string]any, server any) map[string]any {
	serverObject, ok := server.(map[string]any)
	if !ok {
		return staged
	}

	result := maps.Clone(staged)
	for key, serverValue := range serverObject {
		value, present := result[key]
		if !present {
			result[key] = nil
			continue
		}
		if child, ok := value.(map[string]any); ok {
			result[key] = withRemovals(child, serverValue)
		}
	}
	return result
}

// setPatchValue sets value at path in the patch, creating parent objects. A
// parent already set to a non-object value by a shorter declared path covers
// the longer one, so the longer path is skipped.
func setPatchValue(patch map[string]any, path []string, value any) {
	current := patch
	for _, segment := range path[:len(path)-1] {
		next, exists := current[segment]
		if !exists {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			return
		}
		current = child
	}
	current[path[len(path)-1]] = value
}
