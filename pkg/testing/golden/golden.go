// Package golden provides snapshot testing utilities for verifying resource
// primitive output across versions.
package golden

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Previewer is satisfied by any resource primitive that can render its
// post-mutation state as a client.Object. All built-in primitives implement
// this through generic.BaseResource.
type Previewer interface {
	Preview() (client.Object, error)
}

// ComponentPreviewer is satisfied by a built component that can render the
// desired state of all the resources it would apply. The component package's
// *Component implements this through its Preview method.
type ComponentPreviewer interface {
	Preview() ([]client.Object, error)
}

type config struct {
	update bool
	scheme *runtime.Scheme
}

// Option configures behavior.
type Option func(*config)

// Update returns an Option that overwrites the golden file with the actual
// output when enabled. Typically wired to a -update test flag:
//
//	var update = flag.Bool("update", false, "update golden files")
//
//	golden.AssertYAML(t, "testdata/foo.yaml", res, golden.Update(*update))
func Update(enabled bool) Option {
	return func(c *config) {
		c.update = enabled
	}
}

// WithScheme returns an Option that sets apiVersion and kind on the serialized
// object when TypeMeta is not populated. The scheme is used to look up the
// object's GroupVersionKind.
//
// Without this option, objects that omit TypeMeta cause serialization to fail.
// Callers must either populate TypeMeta on the object or provide a scheme via
// WithScheme so apiVersion and kind can be resolved.
func WithScheme(s *runtime.Scheme) Option {
	return func(c *config) {
		c.scheme = s
	}
}

// MismatchError is returned by [CompareYAML] when the serialized output does
// not match the golden file. The Diff field contains a unified diff.
type MismatchError struct {
	Path string
	Diff string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("golden file mismatch: %s\n\n%s", e.Path, e.Diff)
}

// CompareYAML calls Preview on p, serializes the result as YAML, and compares
// it against the golden file at path. Returns a [*MismatchError] if they
// differ, or nil if they match.
//
// When [Update] is enabled, the golden file is written (creating intermediate
// directories as needed) and comparison is skipped.
//
// Returns an error if the golden file does not exist and Update is not enabled.
func CompareYAML(path string, p Previewer, opts ...Option) error {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	obj, err := p.Preview()
	if err != nil {
		return fmt.Errorf("Preview failed: %w", err)
	}

	actual, err := serializeObject(obj, cfg.scheme)
	if err != nil {
		return fmt.Errorf("failed to serialize object to YAML: %w", err)
	}

	return compareOrUpdate(path, actual, cfg.update)
}

// AssertYAML is a test helper that calls [CompareYAML] and fails the test if
// the result does not match the golden file. See [CompareYAML] for details.
func AssertYAML(t *testing.T, path string, p Previewer, opts ...Option) {
	t.Helper()
	require.NoError(t, CompareYAML(path, p, opts...))
}

// CompareComponentYAML calls Preview on c, serializes every rendered object
// into a single multi-document YAML stream (--- separated, in the order
// Preview returns them), and compares it against the golden file at path.
// Returns a [*MismatchError] if they differ, or nil if they match.
//
// When [Update] is enabled, the golden file is written and comparison is
// skipped. [WithScheme] is applied to each object as in [CompareYAML]. An
// empty preview produces empty output.
func CompareComponentYAML(path string, c ComponentPreviewer, opts ...Option) error {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	objs, err := c.Preview()
	if err != nil {
		return fmt.Errorf("Preview failed: %w", err)
	}

	docs := make([][]byte, 0, len(objs))
	for i, obj := range objs {
		doc, err := serializeObject(obj, cfg.scheme)
		if err != nil {
			return fmt.Errorf("failed to serialize object %d to YAML: %w", i, err)
		}
		docs = append(docs, doc)
	}

	actual := bytes.Join(docs, []byte("---\n"))

	return compareOrUpdate(path, actual, cfg.update)
}

// AssertComponentYAML is a test helper that calls [CompareComponentYAML] and
// fails the test if the result does not match the golden file.
func AssertComponentYAML(t *testing.T, path string, c ComponentPreviewer, opts ...Option) {
	t.Helper()
	require.NoError(t, CompareComponentYAML(path, c, opts...))
}

// compareOrUpdate writes actual to path when update is set, otherwise compares
// actual against the existing golden file at path.
func compareOrUpdate(path string, actual []byte, update bool) error {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create golden file directory: %w", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			return fmt.Errorf("failed to write golden file: %w", err)
		}
		return nil
	}

	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("golden file %s does not exist; run with -update to create it", path)
	}
	if err != nil {
		return fmt.Errorf("failed to read golden file: %w", err)
	}

	if string(expected) != string(actual) {
		return &MismatchError{Path: path, Diff: formatDiff(string(expected), string(actual))}
	}
	return nil
}

// serializeObject marshals a client.Object to YAML, removing zero-value noise
// fields that Kubernetes types emit (e.g. creationTimestamp: null).
//
// When the object has no TypeMeta set, the scheme is used to look up and inject
// apiVersion and kind. Returns an error if neither TypeMeta nor a scheme is
// available to determine the object's type.
func serializeObject(obj client.Object, scheme *runtime.Scheme) ([]byte, error) {
	if err := ensureTypeMeta(obj, scheme); err != nil {
		return nil, err
	}

	raw, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}

	// Round-trip through an unstructured map to strip zero-value noise fields.
	var m map[string]interface{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	cleanMetadataNoise(m)

	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// ensureTypeMeta verifies the object has both apiVersion and kind set. If
// TypeMeta is incomplete or empty, it attempts to populate it from the scheme.
// Returns an error if the type cannot be determined.
func ensureTypeMeta(obj client.Object, scheme *runtime.Scheme) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind != "" && gvk.Version != "" {
		return nil
	}

	if scheme == nil {
		return fmt.Errorf(
			"object %T has incomplete TypeMeta (kind=%q, apiVersion=%q) and no scheme was provided; "+
				"either set TypeMeta on the object or pass golden.WithScheme(scheme)",
			obj,
			gvk.Kind,
			gvk.GroupVersion().String(),
		)
	}

	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return fmt.Errorf("failed to look up GVK for %T in scheme: %w", obj, err)
	}

	if len(gvks) == 0 {
		return fmt.Errorf("scheme has no registered GVK for %T", obj)
	}

	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}

// cleanMetadataNoise removes known zero-value fields that Kubernetes types
// always emit but carry no information in a desired-state preview.
func cleanMetadataNoise(m map[string]interface{}) {
	if meta, ok := m["metadata"].(map[string]interface{}); ok {
		delete(meta, "creationTimestamp")
	}

	if status, ok := m["status"]; ok {
		switch v := status.(type) {
		case map[string]interface{}:
			if len(v) == 0 {
				delete(m, "status")
			}
		case nil:
			delete(m, "status")
		}
	}
}

// formatDiff produces a unified diff between expected and actual content
// with surrounding context lines.
func formatDiff(expected, actual string) string {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(expected),
		B:        difflib.SplitLines(actual),
		FromFile: "expected",
		ToFile:   "actual",
		Context:  3,
	})
	return diff
}
