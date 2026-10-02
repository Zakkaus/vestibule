package lookup

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ReleaseInfo struct {
	Label        string
	SupportEnded bool
}
type DistroRow struct {
	Label, Version, URL string
	Release             ReleaseInfo
}
type DistroResult struct {
	Project                 string
	Rows                    []DistroRow
	Alternatives            []string
	Exact, Available, Found bool
}

func QueryDistros(ctx context.Context, name string) DistroResult {
	EnsureReleaseInfo(ctx, time.Now())
	proj, pkgs, alts, exact, available := FetchRepology(ctx, RepologyQuery(name))
	result := DistroResult{Project: proj, Alternatives: alts, Exact: exact, Available: available, Found: len(pkgs) > 0}
	if len(pkgs) == 0 {
		return result
	}
	families := map[string][]RepologyPkg{}
	for _, p := range pkgs {
		if family := FamOf(p.Repo); family != "" && p.Version != "" {
			families[family] = append(families[family], p)
		}
	}
	result.Rows = queryGentooDistroRows(ctx, proj)
	qproj := url.QueryEscape(proj)
	for i := range DistroFamilies {
		family := &DistroFamilies[i]
		result.Rows = appendDistroFamilyRows(result.Rows, family, families[family.Label], qproj)
	}
	return result
}

func appendDistroFamilyRows(dst []DistroRow, family *distroFamily, rows []RepologyPkg, projectQuery string) []DistroRow {
	if len(rows) == 0 {
		return dst
	}
	if family.Label == "Gentoo" {
		if len(dst) == 0 {
			version, repo := NewestRow(rows)
			dst = append(dst, DistroRow{Label: "Gentoo", Version: version, URL: fmt.Sprintf(family.Search, projectQuery), Release: ReleaseInfo{Label: ReleaseLabel(repo, family.Prefixes)}})
		}
		return dst
	}
	var excluded func(string) bool
	switch family.Label {
	case "Debian":
		excluded = DebianTesting
	case "Ubuntu":
		excluded = UbuntuExcluded
	}
	for _, channel := range familyChannels(rows, family.Prefixes, excluded) {
		release := ReleaseInfo{Label: channel.Label}
		if family.Relabel != nil {
			release = family.Relabel(channel.Label)
		}
		dst = append(dst, DistroRow{Label: family.Label, Version: channel.Ver, URL: fmt.Sprintf(family.Search, projectQuery), Release: release})
	}
	return dst
}

func queryGentooDistroRows(ctx context.Context, project string) []DistroRow {
	atoms, _ := SearchMainTree(ctx, project)
	if len(atoms) == 0 {
		return nil
	}
	atom := atoms[0]
	if pkgName := atom[strings.LastIndexByte(atom, '/')+1:]; !strings.EqualFold(pkgName, project) {
		return nil
	}
	stable, latest, _ := PkgVersion(ctx, atom)
	lines := GentooDistroLines(stable, latest, "https://packages.gentoo.org/packages/"+atom)
	rows := make([]DistroRow, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, DistroRow{Label: line.Label, Version: line.Ver, URL: line.Url, Release: ReleaseInfo{Label: line.Rel}})
	}
	return rows
}
