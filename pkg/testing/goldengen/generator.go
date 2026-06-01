package goldengen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Generator runs a version matrix: per-fixture gating assertions, minimal goldens,
// and a coverage manifest, all derived from one Config.
type Generator[T any] struct {
	cfg    Config[T]
	update bool
}

// New creates a Generator from a Config.
func New[T any](cfg Config[T]) *Generator[T] {
	return &Generator[T]{cfg: cfg}
}

// WithUpdate controls whether Run overwrites goldens and the manifest. Wire it to
// a -update test flag: gen.WithUpdate(*update).
func (g *Generator[T]) WithUpdate(enabled bool) *Generator[T] {
	g.update = enabled
	return g
}

// AssertComplete proves that every registered mutation is consciously accounted
// for: each one is either exercised by at least one fixture, by being named in
// that fixture's Requires (which makes Run assert it actually fires), or it is
// explicitly listed in Config.Exclude as deliberately not covered. Nothing
// registered may slip through both untested and unexcluded. It is the guard that a
// newly added mutation forces exactly one decision: cover it or exclude it.
//
// It is called from the consumer's TestMain so its result gates the package's exit
// code:
//
//	func TestMain(m *testing.M) { os.Exit(gen.AssertComplete(m.Run())) }
//
// Concretely, accounting holds when union(Requires names across all fixtures) plus
// Config.Exclude equals the set of registered mutation Names. The registered
// universe is gathered by building each fixture once (registration is
// version-independent). Forbids does not count toward coverage: forbidding a
// mutation asserts where it must not fire, which is not evidence that it works, so
// a mutation that only ever appears in Forbids is still unaccounted.
//
// It returns code unchanged when code is already nonzero (the tests failed, so
// accounting noise would only obscure that) or when accounting holds. Otherwise it
// prints the violations to stderr and returns a nonzero code. Violations are: a
// registered mutation neither required by any fixture nor excluded; a stale Exclude
// or Requires name not registered by any fixture; and an empty registered mutation
// Name.
func (g *Generator[T]) AssertComplete(code int) int {
	if code != 0 {
		return code
	}
	if err := g.cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "goldengen: invalid config: %v\n", err)
		return 1
	}

	registered := map[string]struct{}{}
	for _, f := range g.cfg.Fixtures {
		unit, err := g.cfg.Build(g.cfg.Versions[0], f.Spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goldengen: build fixture %q for accounting: %v\n", f.Name, err)
			return 1
		}
		for _, name := range unit.RegisteredMutations() {
			if name == "" {
				fmt.Fprintf(os.Stderr, "goldengen: fixture %q registers a mutation with an empty Name\n", f.Name)
				return 1
			}
			registered[name] = struct{}{}
		}
	}

	accounted := map[string]struct{}{}
	for _, f := range g.cfg.Fixtures {
		for _, e := range f.Requires {
			accounted[e.Name] = struct{}{}
		}
	}
	for _, name := range g.cfg.Exclude {
		accounted[name] = struct{}{}
	}

	var violations []string
	for name := range registered {
		if _, ok := accounted[name]; !ok {
			violations = append(violations, fmt.Sprintf("unaccounted mutation %q (require it in a fixture or add it to Exclude)", name))
		}
	}
	for name := range accounted {
		if _, ok := registered[name]; !ok {
			violations = append(violations, fmt.Sprintf("stale name %q is required or excluded but not registered by any fixture", name))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "goldengen: %s\n", v)
		}
		return 1
	}
	return code
}

// fixtureSweep holds, per fixture, the per-version firing-sets and the built units
// used to render representatives.
type fixtureSweep struct {
	firing map[string][]string // version -> firing-set
	units  map[string]Unit     // version -> built unit
}

// sweepFixture builds the fixture at every version, collecting its per-version
// firing-set and the built unit.
func (g *Generator[T]) sweepFixture(f Fixture[T]) (fixtureSweep, error) {
	sw := fixtureSweep{firing: map[string][]string{}, units: map[string]Unit{}}
	for _, v := range g.cfg.Versions {
		unit, err := g.cfg.Build(v, f.Spec)
		if err != nil {
			return sw, fmt.Errorf("build fixture %q at %s: %w", f.Name, v, err)
		}
		firing, err := unit.FiringSet()
		if err != nil {
			return sw, fmt.Errorf("firing set for fixture %q at %s: %w", f.Name, v, err)
		}
		sw.firing[v] = firing
		sw.units[v] = unit
	}
	return sw, nil
}

// Run validates the config, asserts per-fixture gating, and writes (or compares)
// one golden per regime at <Dir>/<fixture>/<representative>.yaml plus the coverage
// manifest at <Dir>/manifest.yaml. It honors WithUpdate.
func (g *Generator[T]) Run(t *testing.T) {
	t.Helper()
	if err := g.cfg.Validate(); err != nil {
		t.Fatalf("goldengen: invalid config: %v", err)
	}

	manifest := Manifest{}
	for _, f := range g.cfg.Fixtures {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			sw, err := g.sweepFixture(f)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if err := CheckGating(f, g.cfg.Versions, sw.firing); err != nil {
				t.Errorf("%v", err)
			}

			regimes := ClassifyRegimes(g.cfg.Versions, sw.firing)
			fm := FixtureManifest{Name: f.Name}
			for _, r := range regimes {
				unit := sw.units[r.Representative]
				y, err := unit.RenderYAML()
				if err != nil {
					t.Fatalf("render fixture %q regime %s: %v", f.Name, r.Representative, err)
				}
				path := filepath.Join(g.cfg.Dir, f.Name, r.Representative+".yaml")
				if err := writeOrCompareGolden(path, y, g.update); err != nil {
					t.Errorf("%v", err)
				}
				fm.Regimes = append(fm.Regimes, RegimeManifest(r))
			}
			manifest.Fixtures = append(manifest.Fixtures, fm)
		})
	}

	manifestYAML, err := manifest.YAML()
	if err != nil {
		t.Fatalf("goldengen: marshal manifest: %v", err)
	}
	if err := writeOrCompareGolden(filepath.Join(g.cfg.Dir, "manifest.yaml"), manifestYAML, g.update); err != nil {
		t.Errorf("%v", err)
	}
}

// writeOrCompareGolden writes the bytes when update is set, otherwise compares
// against the file at path, returning a descriptive error on mismatch.
func writeOrCompareGolden(path string, actual []byte, update bool) error {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create golden dir: %w", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			return fmt.Errorf("write golden %s: %w", path, err)
		}
		return nil
	}
	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("golden %s does not exist; run with -update", path)
	}
	if err != nil {
		return fmt.Errorf("read golden %s: %w", path, err)
	}
	if string(expected) != string(actual) {
		return fmt.Errorf("golden mismatch at %s", path)
	}
	return nil
}
