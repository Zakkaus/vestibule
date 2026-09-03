package lookup

import (
	"fmt"
	"testing"
)

// overlayCacheWith builds a cache holding one overlay with the given atoms, each at version 1.0.
func overlayCacheWith(name string, atoms ...string) *pkgCache {
	pkgs := make(map[string]string, len(atoms))
	for _, atom := range atoms {
		pkgs[atom] = "1.0"
	}
	return &pkgCache{
		pkgs:      map[string]map[string]string{name: pkgs},
		available: map[string]bool{name: true},
	}
}

// An overlay hit whose package name is exactly the query is listed first. Ranked with the
// incidental substring matches instead, the package somebody named is pushed past the eight-hit
// cap and disappears from the answer, so /pkg tells them it is not packaged when it is.
func TestAnExactOverlayMatchOutranksSubstringMatchesAndSurvivesTheCap(t *testing.T) {
	const query = "oh-my-pi-bin"
	const wanted = "zzz-util/oh-my-pi-bin"
	atoms := []string{wanted}
	for i := 1; i <= maxHitsPerSource+1; i++ {
		atoms = append(atoms, fmt.Sprintf("app-misc/oh-my-pi-bin-plugin-%d", i))
	}
	pc := overlayCacheWith("guru", atoms...)

	hits := pc.search(query)["guru"]
	if len(hits) == 0 {
		t.Fatalf("search(%q) returned nothing, want the exact package and its neighbours", query)
	}
	if hits[0] != wanted {
		t.Errorf("first hit = %q, want %q: the package the reader asked for is not at the top", hits[0], wanted)
	}
	found := false
	for _, hit := range hits {
		if hit == wanted {
			found = true
		}
	}
	if !found {
		t.Errorf("hits = %v, want %q among them: the package the reader named was dropped at the cap and the answer says it does not exist",
			hits, wanted)
	}
	if len(hits) > maxHitsPerSource {
		t.Errorf("hits = %d, want at most %d", len(hits), maxHitsPerSource)
	}
}

// A query that carries a category is matched against the whole atom. Matched against the bare
// package name instead, a pasted "dev-util/oh-my-pi-bin" matches nothing at all and the overlay
// that holds the package reports no hit.
func TestASlashedQueryIsMatchedAgainstTheWholeAtom(t *testing.T) {
	pc := overlayCacheWith("guru",
		"dev-util/oh-my-pi-bin",
		"app-misc/other-oh-my-pi-bin",
	)

	hits := pc.search("dev-util/oh-my-pi-bin")["guru"]
	if len(hits) != 1 || hits[0] != "dev-util/oh-my-pi-bin" {
		t.Errorf("search(dev-util/oh-my-pi-bin) = %v, want exactly [dev-util/oh-my-pi-bin]: a categorised query must find its own package and nothing from another category",
			hits)
	}

	// Positive control: the bare name still reaches both packages through the same call.
	if bare := pc.search("oh-my-pi-bin")["guru"]; len(bare) != 2 {
		t.Errorf("search(oh-my-pi-bin) = %v, want both packages", bare)
	}
}
