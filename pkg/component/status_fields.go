package component

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
// to a field of the owner's Go type, so a misspelled path is an error rather
// than a silent loss of ownership. FlushStatus validates the paths before it
// writes and returns an error for an invalid one.
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

	current, ok := jsonField(reflect.TypeOf(owner), "status")
	if !ok {
		return fmt.Errorf("owner type %T has no status field", owner)
	}

	for _, segment := range segments {
		current = indirectType(current)
		// Below a map or an interface the schema is open, so any key is valid.
		if current.Kind() == reflect.Map || current.Kind() == reflect.Interface {
			return nil
		}
		if current.Kind() != reflect.Struct {
			return fmt.Errorf("cannot address %q below a field of type %s", segment, current)
		}

		next, found := jsonField(current, segment)
		if !found {
			return fmt.Errorf("type %s has no field with JSON name %q", current, segment)
		}
		current = next
	}
	return nil
}

// jsonField returns the type of the struct field that encodes under name,
// following inlined and embedded structs the way encoding/json does.
func jsonField(t reflect.Type, name string) (reflect.Type, bool) {
	t = indirectType(t)
	if t.Kind() != reflect.Struct {
		return nil, false
	}

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		fieldName, _, _ := strings.Cut(tag, ",")
		if fieldName == "" && field.Anonymous {
			if found, ok := jsonField(field.Type, name); ok {
				return found, true
			}
			continue
		}
		if fieldName == "" {
			fieldName = field.Name
		}
		if fieldName == name {
			return field.Type, true
		}
	}
	return nil, false
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
	stagedContent, err := toUnstructuredContent(staged)
	if err != nil {
		return err
	}

	serverContent, err := toUnstructuredContent(server)
	if err != nil {
		return err
	}

	stagedStatus, _, err := unstructured.NestedMap(stagedContent, "status")
	if err != nil {
		return fmt.Errorf("reading staged status: %w", err)
	}

	merged, _, err := unstructured.NestedMap(serverContent, "status")
	if err != nil {
		return fmt.Errorf("reading server status: %w", err)
	}
	if merged == nil {
		merged = map[string]any{}
	}
	delete(merged, "conditions")

	for _, path := range owned {
		value, found, err := unstructured.NestedFieldNoCopy(stagedStatus, path...)
		if err != nil {
			return fmt.Errorf("reading staged status field %q: %w", strings.Join(path, "."), err)
		}
		if !found {
			unstructured.RemoveNestedField(merged, path...)
			continue
		}
		if err := unstructured.SetNestedField(merged, value, path...); err != nil {
			return fmt.Errorf("setting status field %q: %w", strings.Join(path, "."), err)
		}
	}

	if conditions, ok := stagedStatus["conditions"]; ok {
		merged["conditions"] = conditions
	}
	stagedContent["status"] = merged
	return fromUnstructuredContent(stagedContent, staged)
}

// toUnstructuredContent returns a copy of obj's content that the caller may
// modify. The converter hands back an unstructured object's own map.
func toUnstructuredContent(obj OperatorCRD) (map[string]any, error) {
	if u, ok := obj.(runtime.Unstructured); ok {
		return runtime.DeepCopyJSON(u.UnstructuredContent()), nil
	}

	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("converting %T to unstructured: %w", obj, err)
	}
	return content, nil
}

// fromUnstructuredContent replaces obj's content. Fields the content does not
// carry are zeroed.
func fromUnstructuredContent(content map[string]any, obj OperatorCRD) error {
	if u, ok := obj.(runtime.Unstructured); ok {
		u.SetUnstructuredContent(content)
		return nil
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(content, obj); err != nil {
		return fmt.Errorf("converting unstructured to %T: %w", obj, err)
	}
	return nil
}
