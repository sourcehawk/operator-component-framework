package goldengen

import "fmt"

// firesAt reports whether name is in the firing-set at version v.
func firesAt(firing map[string][]string, v, name string) bool {
	for _, n := range firing[v] {
		if n == name {
			return true
		}
	}
	return false
}

// firesSomewhere reports whether name is in the firing-set at any swept version.
func firesSomewhere(firing map[string][]string, versions []string, name string) bool {
	for _, v := range versions {
		if firesAt(firing, v, name) {
			return true
		}
	}
	return false
}

// CheckGating verifies a fixture's Requires/Forbids expectations against its
// per-version firing-sets. The lattice is:
//
//   - Requires with empty For: the name fires at some swept version.
//   - Requires with For=v: the name fires at v.
//   - Forbids with empty For: the name fires at no swept version.
//   - Forbids with For=v: the name does not fire at v.
//
// It returns a descriptive error on the first violation.
func CheckGating[T any](f Fixture[T], versions []string, firing map[string][]string) error {
	for _, e := range f.Requires {
		if e.For == "" {
			if !firesSomewhere(firing, versions, e.Name) {
				return fmt.Errorf("fixture %q: required mutation %q never fires across the version sweep", f.Name, e.Name)
			}
			continue
		}
		if !firesAt(firing, e.For, e.Name) {
			return fmt.Errorf("fixture %q: required mutation %q does not fire at %s", f.Name, e.Name, e.For)
		}
	}
	for _, e := range f.Forbids {
		if e.For == "" {
			if firesSomewhere(firing, versions, e.Name) {
				return fmt.Errorf("fixture %q: forbidden mutation %q fires somewhere across the version sweep", f.Name, e.Name)
			}
			continue
		}
		if firesAt(firing, e.For, e.Name) {
			return fmt.Errorf("fixture %q: forbidden mutation %q fires at %s", f.Name, e.Name, e.For)
		}
	}
	return nil
}
