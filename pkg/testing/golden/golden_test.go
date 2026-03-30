package golden

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

// fakePreviewer implements Previewer for testing without pulling in the full
// primitive builder machinery.
type fakePreviewer struct {
	obj *appsv1.Deployment
	err error
}

func (f *fakePreviewer) PreviewObject() (*appsv1.Deployment, error) {
	return f.obj, f.err
}

func testDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-server",
			Namespace: "default",
			Labels:    map[string]string{"app": "web-server"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web-server"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "web-server"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "my-app:2.0.0"},
					},
				},
			},
		},
	}
}

func TestCompareYAML(t *testing.T) {
	t.Run("should return nil when output matches golden file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deployment.yaml")

		p := &fakePreviewer{obj: testDeployment()}

		// Create the golden file first.
		require.NoError(t, CompareYAML(path, p, Update(true)))

		// Compare against it.
		require.NoError(t, CompareYAML(path, p))
	})

	t.Run("should return MismatchError when output differs from golden file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deployment.yaml")

		// Write the golden file with replicas=3.
		original := &fakePreviewer{obj: testDeployment()}
		require.NoError(t, CompareYAML(path, original, Update(true)))

		// Compare with replicas=5.
		modified := testDeployment()
		modified.Spec.Replicas = ptr.To[int32](5)

		err := CompareYAML(path, &fakePreviewer{obj: modified})
		require.Error(t, err)

		var mismatch *MismatchError
		require.True(t, errors.As(err, &mismatch), "error should be *MismatchError")
		assert.Equal(t, path, mismatch.Path)
		assert.Contains(t, mismatch.Diff, "-  replicas: 3")
		assert.Contains(t, mismatch.Diff, "+  replicas: 5")
	})

	t.Run("should create intermediate directories when updating", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deeply", "deployment.yaml")

		p := &fakePreviewer{obj: testDeployment()}
		require.NoError(t, CompareYAML(path, p, Update(true)))

		_, err := os.Stat(path)
		require.NoError(t, err, "golden file should have been created")
	})

	t.Run("should overwrite existing golden file when updating", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deployment.yaml")

		// Write initial golden file.
		original := &fakePreviewer{obj: testDeployment()}
		require.NoError(t, CompareYAML(path, original, Update(true)))
		before, err := os.ReadFile(path)
		require.NoError(t, err)

		// Update with a modified object.
		modified := testDeployment()
		modified.Spec.Replicas = ptr.To[int32](5)
		require.NoError(t, CompareYAML(path, &fakePreviewer{obj: modified}, Update(true)))
		after, err := os.ReadFile(path)
		require.NoError(t, err)

		assert.NotEqual(t, string(before), string(after))
	})

	t.Run("should error when golden file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.yaml")

		err := CompareYAML(path, &fakePreviewer{obj: testDeployment()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("should error when PreviewObject fails", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deployment.yaml")

		err := CompareYAML(path, &fakePreviewer{err: assert.AnError})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PreviewObject failed")
	})
}

func TestAssertYAML(t *testing.T) {
	t.Run("should pass when output matches golden file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deployment.yaml")

		p := &fakePreviewer{obj: testDeployment()}

		// Create the golden file first.
		AssertYAML(t, path, p, Update(true))

		// Now assert against it.
		AssertYAML(t, path, p)
	})
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	return s
}

func TestSerializeObject(t *testing.T) {
	t.Run("should strip creationTimestamp from metadata", func(t *testing.T) {
		dep := testDeployment()
		out, err := serializeObject(dep, nil)
		require.NoError(t, err)
		assert.NotContains(t, string(out), "creationTimestamp")
	})

	t.Run("should strip empty status", func(t *testing.T) {
		dep := testDeployment()
		out, err := serializeObject(dep, nil)
		require.NoError(t, err)
		assert.NotContains(t, string(out), "status:")
	})

	t.Run("should produce stable output across calls", func(t *testing.T) {
		dep := testDeployment()
		out1, err := serializeObject(dep, nil)
		require.NoError(t, err)
		out2, err := serializeObject(dep, nil)
		require.NoError(t, err)
		assert.Equal(t, string(out1), string(out2))
	})

	t.Run("should populate apiVersion and kind from scheme when TypeMeta is empty", func(t *testing.T) {
		dep := testDeployment()
		dep.TypeMeta = metav1.TypeMeta{} // Clear TypeMeta

		out, err := serializeObject(dep, testScheme())
		require.NoError(t, err)
		assert.Contains(t, string(out), "apiVersion: apps/v1")
		assert.Contains(t, string(out), "kind: Deployment")
	})

	t.Run("should not overwrite existing TypeMeta", func(t *testing.T) {
		dep := testDeployment()
		dep.TypeMeta = metav1.TypeMeta{APIVersion: "custom/v1", Kind: "Custom"}

		out, err := serializeObject(dep, testScheme())
		require.NoError(t, err)
		assert.Contains(t, string(out), "apiVersion: custom/v1")
		assert.Contains(t, string(out), "kind: Custom")
	})

	t.Run("should use scheme when Kind is set but apiVersion is missing", func(t *testing.T) {
		dep := testDeployment()
		dep.TypeMeta = metav1.TypeMeta{Kind: "Deployment"} // Kind set, apiVersion missing

		out, err := serializeObject(dep, testScheme())
		require.NoError(t, err)
		assert.Contains(t, string(out), "apiVersion: apps/v1")
		assert.Contains(t, string(out), "kind: Deployment")
	})

	t.Run("should error when TypeMeta is incomplete and no scheme is provided", func(t *testing.T) {
		dep := testDeployment()
		dep.TypeMeta = metav1.TypeMeta{Kind: "Deployment"} // Kind set, apiVersion missing

		_, err := serializeObject(dep, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incomplete TypeMeta")
	})

	t.Run("should error when TypeMeta is empty and no scheme is provided", func(t *testing.T) {
		dep := testDeployment()
		dep.TypeMeta = metav1.TypeMeta{}

		_, err := serializeObject(dep, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incomplete TypeMeta")
		assert.Contains(t, err.Error(), "WithScheme(scheme)")
	})
}

func TestFormatDiff(t *testing.T) {
	t.Run("should produce a unified diff with context", func(t *testing.T) {
		diff := formatDiff("a\nb\nc\n", "a\nB\nc\n")

		assert.Contains(t, diff, "--- expected")
		assert.Contains(t, diff, "+++ actual")
		assert.Contains(t, diff, "-b")
		assert.Contains(t, diff, "+B")
	})

	t.Run("should show added lines", func(t *testing.T) {
		diff := formatDiff("a\n", "a\nb\n")
		assert.Contains(t, diff, "+b")
	})

	t.Run("should show removed lines", func(t *testing.T) {
		diff := formatDiff("a\nb\n", "a\n")
		assert.Contains(t, diff, "-b")
	})
}
