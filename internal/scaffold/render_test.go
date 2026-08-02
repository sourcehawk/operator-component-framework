package scaffold

import (
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

func goldenCases() map[string]TemplateData {
	return map[string]TemplateData{
		"static": {
			Package: "configmap", ImportPath: "k8s.io/api/core/v1", ImportAlias: "corev1",
			TypeName: "ConfigMap", Group: "", Version: "v1", Kind: "ConfigMap", Variant: VariantStatic,
		},
		"workload": {
			Package: "deployment", ImportPath: "k8s.io/api/apps/v1", ImportAlias: "appsv1",
			TypeName: "Deployment", Group: "apps", Version: "v1", Kind: "Deployment", Variant: VariantWorkload,
		},
		"task": {
			Package: "job", ImportPath: "k8s.io/api/batch/v1", ImportAlias: "batchv1",
			TypeName: "Job", Group: "batch", Version: "v1", Kind: "Job", Variant: VariantTask,
		},
		"integration": {
			Package: "ingress", ImportPath: "k8s.io/api/networking/v1", ImportAlias: "networkingv1",
			TypeName: "Ingress", Group: "networking.k8s.io", Version: "v1", Kind: "Ingress", Variant: VariantIntegration,
		},
		"static-cluster-scoped": {
			Package: "clusterrole", ImportPath: "k8s.io/api/rbac/v1", ImportAlias: "rbacv1",
			TypeName: "ClusterRole", Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole",
			ClusterScoped: true, Variant: VariantStatic,
		},
	}
}

func TestRenderGolden(t *testing.T) {
	for name, data := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			files, err := Render(data)
			require.NoError(t, err)
			require.Len(t, files, len(GeneratedFiles))

			for _, fileName := range GeneratedFiles {
				content, ok := files[fileName]
				require.True(t, ok, "missing rendered file %s", fileName)

				goldenPath := filepath.Join("testdata", "golden", name, fileName+".golden")
				if *update {
					require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
					require.NoError(t, os.WriteFile(goldenPath, content, 0o644))
					continue
				}

				expected, err := os.ReadFile(goldenPath)
				require.NoError(t, err)
				assert.Equal(t, string(expected), string(content))
			}
		})
	}
}

func TestRenderProducesParsableGo(t *testing.T) {
	t.Parallel()

	for name, data := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files, err := Render(data)
			require.NoError(t, err)

			for fileName, content := range files {
				_, err := parser.ParseFile(token.NewFileSet(), fileName, content, parser.AllErrors)
				assert.NoError(t, err, "rendered %s does not parse", fileName)
			}
		})
	}
}

func TestRenderRejectsUnknownVariant(t *testing.T) {
	t.Parallel()

	_, err := Render(TemplateData{
		Package: "thing", ImportPath: "example.io/api/v1", ImportAlias: "examplev1",
		TypeName: "Thing", Version: "v1", Kind: "Thing", Variant: Variant("bogus"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown variant "bogus"`)
}
