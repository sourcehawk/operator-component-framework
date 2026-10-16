package component

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kjson "sigs.k8s.io/json"
)

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
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

// WithOwnedStatusFields declares the non-condition status fields that this
// flush owns, for an owner whose status is shared with other writers.
//
// Each path names a field under status by its JSON name, with nested fields
// separated by dots, for example "observedGeneration" or "summary.ready".
// Paths are relative to status: write "phase", not "status.phase". A path must
// not name "conditions" or a field below it: condition ownership comes from the
// components passed to [FlushStatus]. For a typed owner every path must resolve
// to exactly one field in the JSON of the owner's Go type, so a misspelled or
// ambiguous path is an error rather than a silent loss of ownership. FlushStatus validates the
// paths before it writes and returns an error for an invalid one.
//
// The declaration takes effect only on a 409 conflict retry. An owned field
// keeps the staged value, and a field the staged owner does not carry is
// removed. Every other non-condition status field takes the server's value, and
// a field the server does not carry is removed. A field the controller staged
// but did not declare is therefore lost on a retry: declare every non-condition
// field the controller writes.
//
// Calling it with no paths declares that the flush owns no non-condition
// status field, as for a controller that writes only conditions. Paths from
// several options are combined.
func WithOwnedStatusFields(paths ...string) FlushOption {
	return func(o *flushOptions) {
		o.fieldsDeclared = true
		o.ownedFields = append(o.ownedFields, paths...)
	}
}

// parseOwnedStatusFields splits each path into its segments and rejects a path
// the owner's type cannot carry.
func parseOwnedStatusFields(owner OperatorCRD, paths []string) ([][]string, error) {
	parsed := make([][]string, 0, len(paths))
	for _, path := range paths {
		segments := strings.Split(path, ".")
		if err := validateStatusFieldPath(owner, segments); err != nil {
			return nil, fmt.Errorf("owned status field %q: %w", path, err)
		}
		parsed = append(parsed, segments)
	}
	return parsed, nil
}

func validateStatusFieldPath(owner OperatorCRD, segments []string) error {
	for _, segment := range segments {
		if segment == "" {
			return errors.New("path has an empty segment")
		}
	}

	if segments[0] == "conditions" {
		return errors.New("conditions are owned through the components passed to FlushStatus")
	}

	// An unstructured owner has no Go type to check the path against.
	if _, ok := owner.(runtime.Unstructured); ok {
		return nil
	}

	current := reflect.TypeOf(owner)
	for i, segment := range append([]string{"status"}, segments...) {
		current = indirectType(current)
		// A type such as metav1.Time encodes itself, often as a string, so its
		// Go fields say nothing about its JSON shape. The owner type itself is
		// exempt: only its status is addressed.
		if i > 0 && encodesItself(current) {
			return fmt.Errorf("cannot address %q below a field of type %s, which encodes itself", segment, current)
		}
		// Below a map or an interface the schema is open, so any key is valid.
		if current.Kind() == reflect.Map || current.Kind() == reflect.Interface {
			return nil
		}
		if current.Kind() != reflect.Struct {
			return fmt.Errorf("cannot address %q below a field of type %s", segment, current)
		}

		candidates := jsonFields(current, segment, nil)
		switch len(candidates) {
		case 0:
			return fmt.Errorf("type %s has no field with JSON name %q", current, segment)
		case 1:
			current = candidates[0]
		default:
			// encoding/json picks one of several such fields by tag and depth
			// rules. Rejecting the path keeps ownership from landing on a
			// field other than the one that is encoded.
			return fmt.Errorf("type %s has %d fields with JSON name %q, directly or through embedded structs",
				current, len(candidates), segment)
		}
	}
	return nil
}

func encodesItself(t reflect.Type) bool {
	pointer := reflect.PointerTo(t)
	return pointer.Implements(jsonMarshaler) || pointer.Implements(textMarshaler)
}

// jsonFields returns the type of every field with the JSON name name, declared
// on t or on a struct that t embeds at any depth. Pass nil for visiting.
func jsonFields(t reflect.Type, name string, visiting map[reflect.Type]bool) []reflect.Type {
	// A struct can embed itself through a pointer. visiting holds the structs
	// on the current embedding chain, so such a chain ends instead of looping.
	// It is copied per chain, so the same struct embedded twice side by side
	// still counts twice toward ambiguity.
	if visiting[t] {
		return nil
	}
	visiting = maps.Clone(visiting)
	if visiting == nil {
		visiting = map[reflect.Type]bool{}
	}
	visiting[t] = true

	var found []reflect.Type
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		// encoding/json skips unexported fields, except that the exported
		// fields of an unexported embedded struct are promoted.
		if !field.IsExported() && !field.Anonymous {
			continue
		}

		fieldName, _, _ := strings.Cut(tag, ",")
		embedded := indirectType(field.Type)
		if fieldName == "" && field.Anonymous && embedded.Kind() == reflect.Struct {
			found = append(found, jsonFields(embedded, name, visiting)...)
			continue
		}
		if fieldName == "" {
			fieldName = field.Name
		}
		if fieldName == name {
			found = append(found, field.Type)
		}
	}
	return found
}

func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func zeroStatus(obj OperatorCRD) error {
	content, err := toUnstructuredContent(obj)
	if err != nil {
		return err
	}
	delete(content, "status")
	return fromUnstructuredContent(content, obj)
}

// applyOwnedStatusFields sets on staged the non-condition status of server,
// except for the owned paths, which keep the staged value. Conditions are left
// as staged.
func applyOwnedStatusFields(staged, server OperatorCRD, owned [][]string) error {
	// GetStatusConditions decides where the conditions live, and their JSON
	// name can be anything, so they are carried across the conversion through
	// the accessor rather than by key.
	conditions := slices.Clone(*staged.GetStatusConditions())

	stagedContent, err := toUnstructuredContent(staged)
	if err != nil {
		return err
	}

	serverContent, err := toUnstructuredContent(server)
	if err != nil {
		return err
	}

	stagedStatus, err := statusOf(stagedContent)
	if err != nil {
		return fmt.Errorf("reading staged status: %w", err)
	}

	merged, err := statusOf(serverContent)
	if err != nil {
		return fmt.Errorf("reading server status: %w", err)
	}

	for _, path := range owned {
		value, found, err := lookupStatusField(stagedStatus, path)
		if err != nil {
			return fmt.Errorf("reading staged status field %q: %w", strings.Join(path, "."), err)
		}
		if !found {
			unstructured.RemoveNestedField(merged, path...)
			continue
		}

		// A nullable parent without omitempty encodes as null, and
		// SetNestedField cannot descend through a null.
		removeNullParent(merged, path)
		if err := unstructured.SetNestedField(merged, value, path...); err != nil {
			return fmt.Errorf("setting status field %q: %w", strings.Join(path, "."), err)
		}
	}

	stagedContent["status"] = merged
	if err := fromUnstructuredContent(stagedContent, staged); err != nil {
		return err
	}

	*staged.GetStatusConditions() = conditions
	return nil
}

// lookupStatusField returns the value at path. A null parent counts as
// absent, the same as a missing one.
func lookupStatusField(status map[string]any, path []string) (any, bool, error) {
	var current any = status
	for i, segment := range path {
		if current == nil {
			return nil, false, nil
		}

		object, ok := current.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("%q is not an object", strings.Join(path[:i], "."))
		}

		current, ok = object[segment]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func removeNullParent(status map[string]any, path []string) {
	for i := 1; i < len(path); i++ {
		value, found, _ := lookupStatusField(status, path[:i])
		if found && value == nil {
			unstructured.RemoveNestedField(status, path[:i]...)
			return
		}
	}
}

// statusOf returns the status map of content, which the caller may modify. A
// status that is absent or null, as a nullable status without omitempty
// encodes, is an empty map.
func statusOf(content map[string]any) (map[string]any, error) {
	status, ok := content["status"]
	if !ok || status == nil {
		return map[string]any{}, nil
	}

	object, ok := status.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("status is a %T, not an object", status)
	}
	return runtime.DeepCopyJSON(object), nil
}

// toUnstructuredContent returns a copy of obj's content that the caller may
// modify. UnstructuredContent returns the object's own map, not a copy.
func toUnstructuredContent(obj OperatorCRD) (map[string]any, error) {
	if u, ok := obj.(runtime.Unstructured); ok {
		return runtime.DeepCopyJSON(u.UnstructuredContent()), nil
	}

	// JSON, not runtime.DefaultUnstructuredConverter: the converter panics on
	// a struct with an unexported field when it converts back. The decoder is
	// the API machinery's own, which keeps integers as exact int64 values.
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encoding %T: %w", obj, err)
	}

	var content map[string]any
	if err := kjson.UnmarshalCaseSensitivePreserveInts(data, &content); err != nil {
		return nil, fmt.Errorf("decoding %T: %w", obj, err)
	}
	return content, nil
}

// fromUnstructuredContent replaces obj's content. Fields the content does not
// carry are zeroed, unexported fields included.
func fromUnstructuredContent(content map[string]any, obj OperatorCRD) error {
	if u, ok := obj.(runtime.Unstructured); ok {
		u.SetUnstructuredContent(content)
		return nil
	}

	data, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("encoding content for %T: %w", obj, err)
	}

	// Decoding into a non-empty object keeps the fields the content omits.
	reflect.ValueOf(obj).Elem().SetZero()
	if err := kjson.UnmarshalCaseSensitivePreserveInts(data, obj); err != nil {
		return fmt.Errorf("decoding into %T: %w", obj, err)
	}
	return nil
}
