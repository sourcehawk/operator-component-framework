package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSerializeMatchesGoldenPath(t *testing.T) {
	dep := testDeployment()
	out, err := Serialize(dep, testScheme())
	require.NoError(t, err)

	// Byte-identical to what CompareYAML writes through the same serializer.
	path := filepath.Join(t.TempDir(), "dep.yaml")
	require.NoError(t, CompareYAML(path, &fakePreviewer{obj: dep}, WithScheme(testScheme()), Update(true)))
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(out))
}

func TestSerializeComponentJoinsDocuments(t *testing.T) {
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
	objs := []client.Object{testDeployment(), cm}
	out, err := SerializeComponent(objs, testScheme())
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(out), "kind:"))
	assert.Contains(t, string(out), "---\n")
}

func TestSerializeComponentMatchesCompareComponentYAML(t *testing.T) {
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
	objs := []client.Object{testDeployment(), cm}

	out, err := SerializeComponent(objs, testScheme())
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "component.yaml")
	require.NoError(t, CompareComponentYAML(path, &fakeComponentPreviewer{objs: objs}, WithScheme(testScheme()), Update(true)))
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(out))
}
