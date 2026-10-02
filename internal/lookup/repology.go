package lookup

import (
	"regexp"
	"sort"
)

// repologyNameRe matches a project name and nothing that could steer the request elsewhere.
var RepologyNameRe = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,80}$`)

// repologyRows caps the reply. Repology tracks hundreds of repositories, most of which nobody in
// a Linux chat runs, so the named families are listed and the rest are counted.
const repologyRows = 16

type RepologyEntry struct {
	Label   string
	Version string
}

// repologyByFamily keeps the newest version each known family ships and counts everything else,
// reusing the family map and version ordering /pkgs already relies on.
func RepologyByFamily(pkgs []RepologyPkg) (entries []RepologyEntry, others int) {
	newest := map[string]string{}
	unnamed := map[string]bool{}
	for _, p := range pkgs {
		family := FamOf(p.Repo)
		if family == "" {
			unnamed[p.Repo] = true
			continue
		}
		if current, ok := newest[family]; !ok || betterVer(current, p.Version) {
			newest[family] = p.Version
		}
	}
	families := make([]string, 0, len(newest))
	for family := range newest {
		families = append(families, family)
	}
	sort.Strings(families)
	for _, family := range families {
		if len(entries) >= repologyRows {
			others++
			continue
		}
		entries = append(entries, RepologyEntry{Label: family, Version: newest[family]})
	}
	return entries, others + len(unnamed)
}
