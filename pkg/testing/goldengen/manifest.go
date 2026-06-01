package goldengen

import "sigs.k8s.io/yaml"

// RegimeManifest records one behaviorally-distinct regime of a fixture: its
// representative version, the versions it covers, and the shared firing-set.
type RegimeManifest struct {
	Representative string   `json:"representative"`
	Versions       []string `json:"versions"`
	Firing         []string `json:"firing"`
}

// FixtureManifest records all regimes derived for one fixture.
type FixtureManifest struct {
	Name    string           `json:"name"`
	Regimes []RegimeManifest `json:"regimes"`
}

// Manifest is the reviewable coverage map: per fixture, the distinct gating
// regimes with their representative version and firing-set.
type Manifest struct {
	Fixtures []FixtureManifest `json:"fixtures"`
}

// YAML renders the manifest as deterministic YAML.
func (m Manifest) YAML() ([]byte, error) {
	return yaml.Marshal(m)
}
