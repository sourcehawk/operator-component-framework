// Package golden provides snapshot testing utilities for verifying resource
// primitive output across versions.
package golden

import (
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
// post-mutation state. All built-in primitives implement this through
// [generic.BaseResource].
type Previewer[T client.Object] interface {
	PreviewObject() (T, error)
}

type config struct {
	update bool
	scheme *runtime.Scheme
}

// Option configures assertion behavior.
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

// AssertYAML calls PreviewObject on p, serializes the result as YAML, and
// compares it against the golden file at path. The test fails with a diff if
// they differ.
//
// When [Update] is enabled, the golden file is written (creating intermediate
// directories as needed) and comparison is skipped.
//
// If the golden file does not exist and Update is not enabled, the test fails.
func AssertYAML[T client.Object](t *testing.T, path string, p Previewer[T], opts ...Option) {
	t.Helper()

	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	obj, err := p.PreviewObject()
	require.NoError(t, err, "PreviewObject failed")

	actual, err := serializeObject(obj, cfg.scheme)
	require.NoError(t, err, "failed to serialize object to YAML")

	if cfg.update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755), "failed to create golden file directory")
		require.NoError(t, os.WriteFile(path, actual, 0o644), "failed to write golden file")
		return
	}

	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		require.Fail(t, fmt.Sprintf("golden file %s does not exist; run with -update to create it", path))
		return
	}
	require.NoError(t, err, "failed to read golden file")

	if string(expected) != string(actual) {
		require.Fail(t, fmt.Sprintf("golden file mismatch: %s\n\n%s", path, formatDiff(string(expected), string(actual))))
	}
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
