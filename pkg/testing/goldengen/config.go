package goldengen

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

// Expect is an assertion that a named mutation fires (Requires) or does not fire
// (Forbids) over a fixture's version sweep. For is optional; when set it must be a
// concrete version drawn from Config.Versions and pins the assertion to that
// version instead of quantifying over the whole sweep.
type Expect struct {
	// Name is the registered mutation Name the expectation applies to.
	Name string
	// For optionally pins the expectation to a single version from Config.Versions.
	For string
}

// Fixture is one spec and the gating expectations the author asserts for it.
type Fixture[T any] struct {
	// Name identifies the fixture and names its golden subdirectory.
	Name string
	// Spec is the input passed to Config.Build for every version.
	Spec T
	// Requires lists mutations that must fire, existentially or pinned via For.
	Requires []Expect
	// Forbids lists mutations that must not fire, existentially or pinned via For.
	Forbids []Expect
}

// Config declares the whole version matrix: the versions to sweep, the fixtures to
// build, the scheme to serialize with, and the Build function that materializes a
// Unit from a fixture spec at a version.
type Config[T any] struct {
	// Dir is the root directory for generated goldens and the manifest.
	Dir string
	// Versions is the version universe to sweep, in supplied order. Representative
	// selection uses this order.
	Versions []string
	// Exclude lists registered mutation Names deliberately left unaccounted by the
	// completeness check. It does not affect gating or golden generation.
	Exclude []string
	// Scheme resolves TypeMeta when serializing rendered objects.
	Scheme *runtime.Scheme
	// Fixtures are the specs to build and assert across the version sweep.
	Fixtures []Fixture[T]
	// Build materializes a Unit from a fixture spec at a version.
	Build func(version string, spec T) (Unit, error)
}

// Validate checks the static invariants of the config before any sweep runs:
// non-empty Versions, a non-nil Build, a non-empty Dir, unique non-empty fixture
// names, non-empty expectation names, and every Expect.For being a member of
// Versions.
func (c Config[T]) Validate() error {
	if len(c.Versions) == 0 {
		return fmt.Errorf("goldengen: Versions must not be empty")
	}
	if c.Build == nil {
		return fmt.Errorf("goldengen: Build must not be nil")
	}
	if c.Dir == "" {
		return fmt.Errorf("goldengen: Dir must not be empty")
	}

	known := make(map[string]struct{}, len(c.Versions))
	for _, v := range c.Versions {
		known[v] = struct{}{}
	}

	seenFixture := make(map[string]struct{}, len(c.Fixtures))
	for _, f := range c.Fixtures {
		if f.Name == "" {
			return fmt.Errorf("goldengen: fixture name must not be empty")
		}
		if _, dup := seenFixture[f.Name]; dup {
			return fmt.Errorf("goldengen: duplicate fixture name %q", f.Name)
		}
		seenFixture[f.Name] = struct{}{}

		for _, e := range append(append([]Expect{}, f.Requires...), f.Forbids...) {
			if e.Name == "" {
				return fmt.Errorf("goldengen: fixture %q has an expectation with an empty Name", f.Name)
			}
			if e.For != "" {
				if _, ok := known[e.For]; !ok {
					return fmt.Errorf("goldengen: fixture %q expectation %q has For %q not in Versions", f.Name, e.Name, e.For)
				}
			}
		}
	}
	return nil
}
