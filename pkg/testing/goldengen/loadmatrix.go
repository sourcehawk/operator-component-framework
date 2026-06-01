package goldengen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// matrixFile is the on-disk shape of a version matrix. It mirrors Config without
// the Go-only Build, which LoadMatrix takes as an argument. The struct tags are
// JSON because sigs.k8s.io/yaml converts YAML to JSON before unmarshalling.
type matrixFile struct {
	Dir      string              `json:"dir"`
	Versions []string            `json:"versions"`
	Exclude  []string            `json:"exclude"`
	Fixtures []matrixFixtureFile `json:"fixtures"`
}

// matrixFixtureFile is the on-disk shape of one fixture. Exactly one of Spec or
// SpecFile must be set. Spec is captured as a raw JSON message (sigs.k8s.io/yaml
// has already converted the inline YAML to JSON by this point) and unmarshalled
// into the typed spec later, which preserves nested fields cleanly.
type matrixFixtureFile struct {
	Name     string          `json:"name"`
	Spec     json.RawMessage `json:"spec"`
	SpecFile string          `json:"specFile"`
	Requires []matrixExpect  `json:"requires"`
	Forbids  []matrixExpect  `json:"forbids"`
}

// matrixExpect is the on-disk shape of an Expect. It carries lowercase JSON tags
// so the YAML keys read naturally (name, for) rather than the Go field names.
type matrixExpect struct {
	Name string `json:"name"`
	For  string `json:"for"`
}

func (e matrixExpect) toExpect() Expect {
	return Expect(e)
}

func toExpects(in []matrixExpect) []Expect {
	if len(in) == 0 {
		return nil
	}
	out := make([]Expect, 0, len(in))
	for _, e := range in {
		out = append(out, e.toExpect())
	}
	return out
}

// LoadMatrix reads a YAML matrix file at path and returns a Config ready to run.
// The file mirrors Config without its Go-only Build, which is supplied here as an
// argument. Each fixture's CR comes from either an inline spec or an external
// specFile (exactly one of the two); the CR is unmarshalled into a fresh T from
// newSpec. A specFile path is resolved relative to the matrix file's directory.
// Supplying both spec and specFile, or neither, is a load error. The resulting
// Config is validated via Config.Validate before it is returned, so invariants
// such as every Expect.For being a member of versions are enforced.
//
// The scheme used to serialize rendered objects is supplied by the build function,
// which passes it to goldengen.Resource or goldengen.Component when it constructs
// each Unit.
func LoadMatrix[T any](
	path string,
	newSpec func() T,
	build func(version string, spec T) (Unit, error),
) (Config[T], error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config[T]{}, fmt.Errorf("read matrix %q: %w", path, err)
	}

	var mf matrixFile
	if err := yaml.Unmarshal(raw, &mf); err != nil {
		return Config[T]{}, fmt.Errorf("parse matrix %q: %w", path, err)
	}

	baseDir := filepath.Dir(path)
	cfg := Config[T]{
		Dir:      mf.Dir,
		Versions: mf.Versions,
		Exclude:  mf.Exclude,
		Build:    build,
	}

	for _, ff := range mf.Fixtures {
		spec, err := loadFixtureSpec(ff, baseDir, newSpec)
		if err != nil {
			return Config[T]{}, err
		}
		cfg.Fixtures = append(cfg.Fixtures, Fixture[T]{
			Name:     ff.Name,
			Spec:     spec,
			Requires: toExpects(ff.Requires),
			Forbids:  toExpects(ff.Forbids),
		})
	}

	if err := cfg.Validate(); err != nil {
		return Config[T]{}, err
	}
	return cfg, nil
}

// loadFixtureSpec resolves a fixture's CR into a fresh T. It enforces the
// exactly-one-of(spec, specFile) invariant and resolves specFile relative to
// baseDir.
func loadFixtureSpec[T any](ff matrixFixtureFile, baseDir string, newSpec func() T) (T, error) {
	var zero T
	hasInline := len(ff.Spec) > 0
	hasFile := ff.SpecFile != ""
	if hasInline == hasFile {
		return zero, fmt.Errorf("fixture %q must set exactly one of spec or specFile", ff.Name)
	}

	data := []byte(ff.Spec)
	if hasFile {
		b, err := os.ReadFile(filepath.Join(baseDir, ff.SpecFile))
		if err != nil {
			return zero, fmt.Errorf("fixture %q specFile %q: %w", ff.Name, ff.SpecFile, err)
		}
		data = b
	}

	spec := newSpec()
	// Unmarshal into &spec rather than spec so a value-type T works too: a pointer
	// T (the common case) is populated in place, and a value T becomes addressable.
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return zero, fmt.Errorf("fixture %q spec: %w", ff.Name, err)
	}
	return spec, nil
}
