package lookup

import (
	"context"

	neturl "net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type RepologyPkg struct {
	Repo    string `json:"repo"`
	Version string `json:"version"`
}

type distroFamily struct {
	Label    string
	Prefixes []string
	Search   string
	Relabel  func(string) ReleaseInfo
}

// Repo prefixes define displayed families; relabel derives live release roles.
// RHEL rebuilds, CentOS Stream, and EPEL remain separate version channels.
var DistroFamilies = []distroFamily{
	{"Gentoo", []string{"gentoo"}, "https://packages.gentoo.org/packages/search?q=%s", nil},
	{"AUR", []string{"aur"}, "https://aur.archlinux.org/packages?K=%s", nil},
	{"Arch", []string{"arch"}, "https://archlinux.org/packages/?q=%s", nil},
	{"Alpine", []string{"alpine_"}, "https://pkgs.alpinelinux.org/packages?name=%s", nil},
	{"Debian", []string{"debian_"}, "https://tracker.debian.org/pkg/%s", debianRelabel},
	{"Ubuntu", []string{"ubuntu_"}, "https://launchpad.net/ubuntu/+source/%s", ubuntuRelabel},
	{"Nixpkgs", []string{"nix_"}, "https://search.nixos.org/packages?query=%s", nil},
	{"Fedora", []string{"fedora_"}, "https://packages.fedoraproject.org/pkgs/%s/", nil},
	{"RHEL", []string{"almalinux_", "rocky_"}, "https://repology.org/project/%s/versions", nil},
	{"CentOS Stream", []string{"centos_stream_"}, "https://repology.org/project/%s/versions", nil},
	{"EPEL", []string{"epel_"}, "https://packages.fedoraproject.org/pkgs/%s/", nil},
	{"openSUSE Leap", []string{"opensuse_leap"}, "https://software.opensuse.org/search?q=%s", nil},
	{"openSUSE Tumbleweed", []string{"opensuse_tumbleweed"}, "https://software.opensuse.org/search?q=%s", nil},
}

func FamOf(repo string) string {
	for _, f := range DistroFamilies {
		for _, p := range f.Prefixes {
			// Require an exact prefix boundary; "archpower_*" is not Arch.
			if repo == p || strings.HasPrefix(repo, strings.TrimRight(p, "_")+"_") {
				return f.Label
			}
		}
	}
	return ""
}

// Date-like snapshots rank below real releases but still order correctly in CalVer-only families.
func dateSnapshot(v string) bool {
	if bareDate(v) { // bare 8-digit YYYYMMDD, e.g. gcc-snapshot
		return true
	}
	if len(v) < 10 {
		return false
	}
	sep := v[4]
	if (sep != '-' && sep != '.') || v[7] != sep {
		return false
	}
	for i := 0; i < 10; i++ {
		if i == 4 || i == 7 {
			continue
		}
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

// Plausibility bounds keep non-date eight-digit versions out of the snapshot tier.
func bareDate(v string) bool {
	if len(v) != 8 {
		return false
	}
	for i := 0; i < 8; i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	y := int(v[0]-'0')*1000 + int(v[1]-'0')*100 + int(v[2]-'0')*10 + int(v[3]-'0')
	m := int(v[4]-'0')*10 + int(v[5]-'0')
	d := int(v[6]-'0')*10 + int(v[7]-'0')
	return y >= 1990 && y <= 2100 && m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// Gentoo 9999 variants track live source rather than a release.
func allNines(v string) bool {
	nine := false
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] == '9':
			nine = true
		case v[i] == '.':
		default:
			return false
		}
	}
	return nine
}

// Require digits around "snap" to avoid classifying genuine snapshot versions as transition stubs.
var snapTransitionalRe = regexp.MustCompile(`(?i)\d+snap\d+`)

// Snap transitional debs rank below real packages and render as "snap".
func snapVersion(v string) bool { return snapTransitionalRe.MatchString(v) }

// Transitional package versions render as "snap".
func DisplayVer(v string) string {
	if snapVersion(v) {
		return "snap"
	}
	return v
}

// Prefer real releases, then dates, then live or transitional pseudo-versions.
func verTier(v string) int {
	switch {
	case allNines(v), snapVersion(v):
		return 2
	case dateSnapshot(v):
		return 1
	default:
		return 0
	}
}

// Better tiers win; equal tiers use Gentoo version ordering.
func betterVer(cur, cand string) bool {
	if ct, nt := verTier(cur), verTier(cand); ct != nt {
		return nt < ct
	}
	return verLess(cur, cand)
}

func RepologyVersionsURL(proj string) string {
	return "https://repology.org/project/" + neturl.PathEscape(proj) + "/versions"
}

func NewestRow(rows []RepologyPkg) (ver, repo string) {
	for _, p := range rows {
		if ver == "" || betterVer(ver, p.Version) {
			ver, repo = p.Version, p.Repo
		}
	}
	return ver, repo
}

// Rolling/development channels are distinct from numbered stable releases.
func rollingRelease(label string) bool {
	switch label {
	case "", "unstable", "testing", "rawhide", "edge", "sid", "devel", "cauldron", "current":
		return true
	}
	return false
}

type channelLine struct{ Ver, Label string }

// Show the newest supported numbered release and a newer rolling channel.
// Choose by release recency, not package version: an old release's higher version must not win.
// isTesting excludes development, unreleased, or EOL numbered series.
func familyChannels(rows []RepologyPkg, prefixes []string, isTesting func(string) bool) []channelLine {
	if len(rows) == 0 {
		return nil
	}
	excluded := func(lbl string) bool { return isTesting != nil && isTesting(lbl) }

	rolling := newestRollingChannel(rows, prefixes, excluded)
	stable := newestStableChannel(rows, prefixes, excluded)
	switch {
	case stable.Ver == "" && rolling.Ver == "": // everything excluded — fall back to the raw newest
		v, r := NewestRow(rows)
		return []channelLine{{v, ReleaseLabel(r, prefixes)}}
	case stable.Ver == "": // a pure rolling distro (Arch, AUR, Tumbleweed) — just the rolling line
		return []channelLine{rolling}
	case rolling.Ver == "" || !betterVer(stable.Ver, rolling.Ver): // no rolling, or it isn't ahead
		return []channelLine{stable}
	default: // a rolling/dev channel is ahead of stable — show it, then stable
		return []channelLine{rolling, stable}
	}
}

func newestRollingChannel(rows []RepologyPkg, prefixes []string, excluded func(string) bool) channelLine {
	var newest channelLine
	for _, p := range rows {
		label := ReleaseLabel(p.Repo, prefixes)
		if !rollingRelease(label) || excluded(label) {
			continue
		}
		if newest.Ver == "" || betterVer(newest.Ver, p.Version) {
			newest = channelLine{p.Version, label}
		}
	}
	return newest
}

func newestStableChannel(rows []RepologyPkg, prefixes []string, excluded func(string) bool) channelLine {
	var newest channelLine
	for _, p := range rows {
		label := ReleaseLabel(p.Repo, prefixes)
		if rollingRelease(label) || excluded(label) {
			continue
		}
		switch {
		case newest.Label == "" || verLess(newest.Label, label): // first, or a newer release
			newest = channelLine{p.Version, label}
		case label == newest.Label && betterVer(newest.Ver, p.Version): // same release, better version
			newest.Ver = p.Version
		}
	}
	return newest
}

// Live distro metadata prevents Debian testing from being mislabeled stable.
func DebianTesting(label string) bool {
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	return relInfo.Debian[label] == "testing"
}

// Known unreleased Debian suites, including sid, are development channels.
func DebianDevSuite(series string) bool {
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	released, known := relInfo.DebianSer[strings.ToLower(series)]
	return known && !released
}

// releaseLabel removes the family prefix; exact rolling repos have no label.
func ReleaseLabel(repo string, prefixes []string) string {
	s := repo
	for _, p := range prefixes {
		if strings.HasPrefix(repo, p) {
			s = strings.TrimPrefix(repo, p)
			break
		}
	}
	s = strings.TrimLeft(s, "_")
	if s == "" || s == repo { // exact-prefix (rolling) repo, or no prefix matched
		return ""
	}
	s = strings.TrimPrefix(s, "stable_") // nix_stable_25_11 -> 25.11, not "stable.25.11"
	return strings.ReplaceAll(s, "_", ".")
}

// A Repology 404 or empty direct result may fall back to search; other failures remain unavailable.
func FetchRepology(ctx context.Context, name string) (proj string, pkgs []RepologyPkg, alts []string, exact, available bool) {
	return fetchRepologyWith(ctx, name, func(ctx context.Context, url string, dst any) error {
		return GetJSON(ctx, url, nil, dst)
	})
}

func fetchRepologyWith(
	ctx context.Context,
	name string,
	getJSON func(context.Context, string, any) error,
) (proj string, pkgs []RepologyPkg, alts []string, exact, available bool) {
	q := strings.ToLower(strings.TrimSpace(name))
	if q == "" {
		return "", nil, nil, false, true
	}
	err := getJSON(ctx, "https://repology.org/api/v1/project/"+neturl.PathEscape(q), &pkgs)
	if err == nil && len(pkgs) > 0 {
		return q, pkgs, nil, true, true
	}
	if err != nil && httpStatusCode(err) != 404 {
		return "", nil, nil, false, false
	}
	var found map[string][]RepologyPkg
	if err := getJSON(ctx, "https://repology.org/api/v1/projects/?search="+neturl.QueryEscape(q), &found); err != nil {
		return "", nil, nil, false, false
	}
	if p, ok := found[q]; ok { // exact name surfaced by the search
		return q, p, nil, true, true
	}
	type cand struct {
		name string
		fams int
	}
	cands := make([]cand, 0, len(found))
	for n, ps := range found {
		if strings.Contains(n, ":") {
			continue // skip Repology's language-namespaced projects (go:…, haskell:…)
		}
		fset := map[string]bool{}
		for _, p := range ps {
			if f := FamOf(p.Repo); f != "" {
				fset[f] = true
			}
		}
		if len(fset) > 0 { // only consider packages that exist in distros we show
			cands = append(cands, cand{n, len(fset)})
		}
	}
	if len(cands) == 0 {
		return "", nil, nil, false, true
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].fams != cands[j].fams {
			return cands[i].fams > cands[j].fams
		}
		return cands[i].name < cands[j].name
	})
	for i := 1; i < len(cands) && i <= 5; i++ {
		alts = append(alts, cands[i].name)
	}
	return cands[0].name, found[cands[0].name], alts, false, true
}

type DistroLine struct{ Label, Ver, Rel, Url string }

// Show stable and newer ~amd64 separately; equal versions remain one stable line.
// Without a stable keyword, show only ~amd64.
func GentooDistroLines(stable, latest, url string) []DistroLine {
	switch {
	case stable != "" && latest != "" && stable != latest:
		return []DistroLine{{"Gentoo amd64", stable, "", url}, {"Gentoo ~amd64", latest, "", url}}
	case stable != "":
		return []DistroLine{{"Gentoo amd64", stable, "", url}}
	case latest != "":
		return []DistroLine{{"Gentoo ~amd64", latest, "", url}}
	}
	return nil
}

type madEntry struct{ Suite, ver string }

// Madison output is oldest-first; pocket variants are excluded and suites deduplicated.
func parseMadison(body string) []madEntry {
	var ordered []madEntry
	idx := map[string]int{}
	for _, ln := range strings.Split(body, "\n") {
		parts := strings.Split(ln, "|")
		if len(parts) < 4 {
			continue
		}
		ver := strings.TrimSpace(parts[1])
		suite := strings.SplitN(strings.TrimSpace(parts[2]), "/", 2)[0] // drop "/universe" etc.
		if ver == "" || suite == "" || strings.Contains(suite, "-") {   // skip -updates/-security/-backports
			continue
		}
		if i, ok := idx[suite]; ok {
			ordered[i].ver = ver // newer line for the same suite wins
			continue
		}
		idx[suite] = len(ordered)
		ordered = append(ordered, madEntry{suite, ver})
	}
	return ordered
}

// pickMadison prefers the newest released suite, flagging a development-only fallback.
func pickMadison(entries []madEntry, devSuite func(string) bool) (suite, ver string, dev bool) {
	pick := entries[len(entries)-1] // madison lists oldest-first, so the last is the newest suite
	dev = devSuite != nil && devSuite(pick.Suite)
	if dev {
		for i := len(entries) - 2; i >= 0; i-- {
			if !devSuite(entries[i].Suite) {
				return entries[i].Suite, entries[i].ver, false
			}
		}
	}
	return pick.Suite, pick.ver, dev
}

var aurArchRe = regexp.MustCompile(`(?i)arch=\(([^)]*)\)`)

const relInfoTTL = 24 * time.Hour

// Failed refreshes retry quickly instead of retaining degraded data for 24 hours.
const relInfoRetryTTL = 10 * time.Minute

var (
	fetchDebianStatusFn = fetchDebianStatus
	fetchUbuntuFn       = fetchUbuntu
)

var relInfo = struct {
	Mu         sync.Mutex
	Debian     map[string]string // Debian version ("13") -> status ("stable"/"testing"/...)
	DebianSer  map[string]bool   // Debian series codename ("trixie") -> already released?
	ubuntu     map[string]bool   // Ubuntu version ("24.04") -> is it an LTS?
	ubuntuRel  map[string]bool   // Ubuntu version ("24.04") -> already released (date in the past)?
	ubuntuEOL  map[string]bool   // Ubuntu version ("18.04") -> past the standard-support end date?
	UbuntuSer  map[string]bool   // Ubuntu series codename ("resolute") -> already released?
	Fetched    time.Time
	Refreshing bool // a fetch is in flight (so concurrent /pkgs don't all hit upstream)
}{}

// Refresh is optional enrichment: failures retain old data and raw labels still work.
// The in-flight guard coalesces concurrent cold lookups.
func EnsureReleaseInfo(ctx context.Context, now time.Time) {
	relInfo.Mu.Lock()
	fresh := relInfo.Debian != nil && now.Sub(relInfo.Fetched) < relInfoTTL
	if fresh || relInfo.Refreshing {
		relInfo.Mu.Unlock()
		return // already fresh, or someone else is fetching — fall back to current data
	}
	relInfo.Refreshing = true
	relInfo.Mu.Unlock()
	// Always clear the in-flight flag, including during panic unwinding.
	defer func() {
		relInfo.Mu.Lock()
		relInfo.Refreshing = false
		relInfo.Mu.Unlock()
	}()

	deb := fetchDebianStatusFn(ctx, now)
	ubu, ubuRel, ubuEOL, ubuSer := fetchUbuntuFn(ctx, now)

	// Empty HTTP-200 parses indicate upstream errors or schema drift; never replace good data.
	debOK, ubuOK := len(deb.Roles) > 0, len(ubu) > 0
	relInfo.Mu.Lock()
	if debOK {
		relInfo.Debian, relInfo.DebianSer = deb.Roles, deb.Series
	}
	if ubuOK {
		relInfo.ubuntu, relInfo.ubuntuRel, relInfo.ubuntuEOL, relInfo.UbuntuSer = ubu, ubuRel, ubuEOL, ubuSer
	}
	if relInfo.Debian == nil {
		relInfo.Debian = map[string]string{} // mark attempted so the freshness gate can hold (no per-call refetch)
	}
	// Full TTL requires both sources; partial refreshes use the short retry window.
	relInfo.Fetched = relInfoNextFetched(now, debOK && ubuOK)
	relInfo.Mu.Unlock()
}

// Backdate failed refreshes to leave only relInfoRetryTTL freshness.
func relInfoNextFetched(now time.Time, bothOK bool) time.Time {
	if bothOK {
		return now
	}
	return now.Add(relInfoRetryTTL - relInfoTTL)
}

// A release date at or before now marks a distro-info row released.
func parseDistroInfo(body string) (rows [][]string) {
	for i, line := range strings.Split(body, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" { // skip header + blanks
			continue
		}
		rows = append(rows, strings.Split(line, ","))
	}
	return rows
}

type debianReleaseData struct {
	Roles  map[string]string
	Series map[string]bool
}

func fetchDebianStatus(ctx context.Context, now time.Time) debianReleaseData {
	body, err := httpGetBody(ctx, "https://debian.pages.debian.net/distro-info-data/debian.csv", 1<<20)
	if err != nil {
		return debianReleaseData{}
	}
	return deriveDebianReleaseData(string(body), now)
}

func deriveDebianStatus(body string, now time.Time) map[string]string {
	return deriveDebianReleaseData(body, now).Roles
}

// Derive stable generations, the next testing release, and suite release state from dates.
func deriveDebianReleaseData(body string, now time.Time) debianReleaseData {
	type rel struct {
		ver      string
		released bool
	}
	var rels []rel
	series := map[string]bool{}
	for _, c := range parseDistroInfo(body) {
		if len(c) < 4 {
			continue
		}
		released := false
		if len(c) >= 5 {
			if t, perr := time.Parse("2006-01-02", c[4]); perr == nil && !t.After(now) {
				released = true
			}
		}
		if name := strings.ToLower(strings.TrimSpace(c[2])); name != "" {
			series[name] = released
		}
		if c[0] == "" {
			continue // sid/experimental have no numbered release role
		}
		rels = append(rels, rel{c[0], released})
	}
	roles := map[string]string{}
	// Released versions, newest first: stable, oldstable, oldoldstable.
	var releasedVersions []string
	for _, r := range rels {
		if r.released {
			releasedVersions = append(releasedVersions, r.ver)
		}
	}
	sort.Slice(releasedVersions, func(i, j int) bool {
		return verLess(releasedVersions[j], releasedVersions[i])
	})
	for i, status := range []string{"stable", "oldstable", "oldoldstable"} {
		if i < len(releasedVersions) {
			roles[releasedVersions[i]] = status
		}
	}
	// The lowest not-yet-released version above stable is "testing".
	if len(releasedVersions) > 0 {
		stable := releasedVersions[0]
		testing := ""
		for _, r := range rels {
			if !r.released && verLess(stable, r.ver) && (testing == "" || verLess(r.ver, testing)) {
				testing = r.ver
			}
		}
		if testing != "" {
			roles[testing] = "testing"
		}
	}
	return debianReleaseData{Roles: roles, Series: series}
}

// Ubuntu maps track LTS, release, standard-support end, and codename release state.
func fetchUbuntu(ctx context.Context, now time.Time) (lts, released, eol, series map[string]bool) {
	body, err := httpGetBody(ctx, "https://debian.pages.debian.net/distro-info-data/ubuntu.csv", 1<<20)
	if err != nil {
		return nil, nil, nil, nil
	}
	lts, released, eol, series = map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range parseDistroInfo(string(body)) {
		if len(c) < 1 || c[0] == "" {
			continue
		}
		ver := strings.TrimSpace(strings.TrimSuffix(c[0], "LTS"))
		lts[ver] = strings.Contains(c[0], "LTS")
		// Store unreleased series as known false, not unknown.
		rel := false
		if len(c) >= 5 {
			if t, perr := time.Parse("2006-01-02", c[4]); perr == nil && !t.After(now) {
				rel = true
			}
		}
		released[ver] = rel
		// Exclude releases past standard support that would mask newer releases shipping only a Snap.
		if len(c) >= 6 {
			if t, perr := time.Parse("2006-01-02", c[5]); perr == nil && !t.After(now) {
				eol[ver] = true
			}
		}
		// Madison uses codenames, so retain their release state too.
		if len(c) >= 3 {
			if s := strings.ToLower(strings.TrimSpace(c[2])); s != "" {
				series[s] = rel
			}
		}
	}
	return lts, released, eol, series
}

// Known unreleased Ubuntu suites are development; unknown suites remain displayable.
func UbuntuDevSuite(series string) bool {
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	released, known := relInfo.UbuntuSer[strings.ToLower(series)]
	return known && !released
}

// Unknown Debian labels pass through before metadata loads.
func debianRelabel(raw string) ReleaseInfo {
	if raw == "unstable" {
		return ReleaseInfo{Label: "unstable/sid"}
	}
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	if s, ok := relInfo.Debian[raw]; ok {
		return ReleaseInfo{Label: raw + " " + s}
	}
	return ReleaseInfo{Label: raw}
}

func ubuntuRelabel(raw string) ReleaseInfo {
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	out := raw
	if relInfo.ubuntu[raw] {
		out += " LTS"
	}
	return ReleaseInfo{Label: out, SupportEnded: relInfo.ubuntuEOL[raw]}
}

// Exclude proposed, backports, unreleased, and post-standard-support Ubuntu series from the current line.
// Unknown series remain eligible so lookups still work before metadata loads.
func UbuntuExcluded(label string) bool {
	if strings.Contains(label, "proposed") || strings.Contains(label, "backport") {
		return true
	}
	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	if relInfo.ubuntuEOL[label] {
		return true
	}
	released, known := relInfo.ubuntuRel[label]
	return known && !released
}
