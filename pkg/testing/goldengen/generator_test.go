package goldengen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/goldengen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

// configForStatefulSet returns a Config whose two versions produce two regimes:
// "v2only" fires only at 2.0.0, so 1.0.0 and 2.0.0 land in distinct regimes.
func configForStatefulSet(dir string) goldengen.Config[*appsv1.StatefulSet] {
	return goldengen.Config[*appsv1.StatefulSet]{
		Dir:      dir,
		Versions: []string{"1.0.0", "2.0.0"},
		Fixtures: []goldengen.Fixture[*appsv1.StatefulSet]{{
			Name: "default",
			Spec: baseStatefulSet(),
			Requires: []goldengen.Expect{
				{Name: "Always"},
				{Name: "V2Only", For: "2.0.0"},
			},
			Forbids: []goldengen.Expect{
				{Name: "V2Only", For: "1.0.0"},
			},
		}},
		Build: func(v string, spec *appsv1.StatefulSet) (goldengen.Unit, error) {
			res, err := statefulset.NewBuilder(spec.DeepCopy()).
				WithMutation(
					statefulset.Mutation{
						Name:   "Always",
						Mutate: func(*statefulset.Mutator) error { return nil },
					},
					statefulset.Mutation{
						Name:    "V2Only",
						Feature: staticGate{enabled: v == "2.0.0"},
						Mutate:  func(*statefulset.Mutator) error { return nil },
					},
				).
				Build()
			if err != nil {
				return nil, err
			}
			return goldengen.Resource(res, testScheme()), nil
		},
	}
}

func TestGeneratorRunWritesGoldensAndManifest(t *testing.T) {
	dir := t.TempDir()
	gen := goldengen.New(configForStatefulSet(dir))

	// First run with update writes goldens + manifest.
	gen.WithUpdate(true).Run(t)

	assert.FileExists(t, filepath.Join(dir, "default", "1.0.0.yaml"))
	assert.FileExists(t, filepath.Join(dir, "default", "2.0.0.yaml"))
	assert.FileExists(t, filepath.Join(dir, "manifest.yaml"))

	// Exactly one golden per regime: the sweep has two versions (1.0.0 and 2.0.0)
	// with distinct firing-sets, so each is its own regime with its own golden.
	gold200, err := os.ReadFile(filepath.Join(dir, "default", "2.0.0.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(gold200), "kind: StatefulSet")

	// The manifest records both regimes for the fixture.
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "name: default")
	assert.Contains(t, string(manifest), "representative: 1.0.0")
	assert.Contains(t, string(manifest), "representative: 2.0.0")
	assert.Contains(t, string(manifest), "V2Only")

	// Second run without update compares clean.
	gen.WithUpdate(false).Run(t)
}
