package goldengen

import (
	"sort"
	"strings"
)

// Regime is a group of swept versions sharing an identical firing-set.
type Regime struct {
	// Representative is the first version in supplied order within the group.
	Representative string
	// Versions are all versions in this regime, in supplied order.
	Versions []string
	// Firing is the shared firing-set, sorted.
	Firing []string
}

// ClassifyRegimes groups versions (in supplied order) by identical firing-set.
// Two versions belong to the same regime when their firing-sets are equal as sets,
// independent of order. Regimes are returned in order of first appearance; the
// representative of each is the first version in supplied order belonging to it.
func ClassifyRegimes(versions []string, firing map[string][]string) []Regime {
	index := make(map[string]int) // signature -> regime index
	regimes := make([]Regime, 0)
	for _, v := range versions {
		sorted := append([]string(nil), firing[v]...)
		sort.Strings(sorted)
		sig := strings.Join(sorted, "\x00")
		if i, ok := index[sig]; ok {
			regimes[i].Versions = append(regimes[i].Versions, v)
			continue
		}
		index[sig] = len(regimes)
		regimes = append(regimes, Regime{
			Representative: v,
			Versions:       []string{v},
			Firing:         sorted,
		})
	}
	return regimes
}
