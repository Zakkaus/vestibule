package lookup

import (
	"context"
	"fmt"

	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/edition"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type overlay struct {
	Name   string // short display name
	repo   string // GitHub owner/name
	branch string
}

// Identify outbound requests as this product; operators can override it.
var userAgent = edition.Name

var Overlays []overlay

func configurePkg(cfg *settings.Config) {
	if cfg.UserAgent != "" {
		userAgent = cfg.UserAgent
	}
	if len(cfg.Overlays) == 0 {
		Overlays = nil
		return
	}
	Overlays = nil
	for _, o := range cfg.Overlays {
		br := o.Branch
		if br == "" {
			br = "master"
		}
		name := o.Name
		if name == "" {
			name = o.Repo
		}
		Overlays = append(Overlays, overlay{Name: name, repo: o.Repo, branch: br})
	}
}

const pkgCacheTTL = 6 * time.Hour

const verCacheTTL = 6 * time.Hour

const maxHitsPerSource = 8

// Bound caches keyed by user input; exceptional overflow clears them wholesale.
const pkgCacheMax = 2000

const pkgRetryFloor = 3 * time.Minute

type pkgCache struct {
	mu          sync.Mutex
	pkgs        map[string]map[string]string
	available   map[string]bool
	fetched     time.Time
	lastAttempt time.Time
	refreshing  bool
}

var PkgC = &pkgCache{
	pkgs:      map[string]map[string]string{},
	available: map[string]bool{},
}

// Preserve per-source availability so partial answers never become definitive misses.
type PkgLookupAvailability struct {
	Official bool
	Overlays map[string]bool
}

func (a PkgLookupAvailability) AnyUnavailable() bool {
	if !a.Official {
		return true
	}
	for _, ok := range a.Overlays {
		if !ok {
			return true
		}
	}
	return false
}

func isPkgPath(p string) bool {
	i := strings.IndexByte(p, '/')
	if i < 1 || strings.Contains(p[i+1:], "/") {
		return false
	}
	switch p[:i] {
	case "metadata", "profiles", "eclass", "licenses", "scripts", ".github", ".gitlab":
		return false
	}
	cat := p[:i]
	return strings.Contains(cat, "-") || cat == "virtual"
}

func splitVer(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' })
}

type gentooSuffix struct {
	class  int
	number string
	raw    string
}

type gentooVersion struct {
	base     []string
	suffixes []gentooSuffix
	revision string
}

// verLess implements the Gentoo ordering needed to select the latest version.
func verLess(a, b string) bool {
	av, bv := parseGentooVersion(a), parseGentooVersion(b)
	if c := compareVersionTokens(av.base, bv.base); c != 0 {
		return c < 0
	}
	for i := range max(len(av.suffixes), len(bv.suffixes)) {
		as, bs := gentooSuffix{}, gentooSuffix{}
		if i < len(av.suffixes) {
			as = av.suffixes[i]
		}
		if i < len(bv.suffixes) {
			bs = bv.suffixes[i]
		}
		if as.class != bs.class {
			return as.class < bs.class
		}
		if as.raw != "" || bs.raw != "" {
			if c := cmpToken(as.raw, bs.raw); c != 0 {
				return c < 0
			}
		} else if c := cmpNum(as.number, bs.number); c != 0 {
			return c < 0
		}
	}
	return cmpNum(av.revision, bv.revision) < 0
}

func parseGentooVersion(version string) gentooVersion {
	revision := ""
	if index := strings.LastIndex(version, "-r"); index >= 0 && decimalDigits(version[index+2:]) {
		revision = version[index+2:]
		version = version[:index]
	}
	parts := strings.Split(version, "_")
	parsed := gentooVersion{base: splitVer(parts[0]), revision: revision}
	if len(parts) > 1 {
		parsed.suffixes = make([]gentooSuffix, 0, len(parts)-1)
		for _, token := range parts[1:] {
			parsed.suffixes = append(parsed.suffixes, parseGentooSuffix(token))
		}
	}
	return parsed
}

func parseGentooSuffix(token string) gentooSuffix {
	for _, suffix := range []struct {
		name  string
		class int
	}{
		{name: "alpha", class: -4},
		{name: "beta", class: -3},
		{name: "pre", class: -2},
		{name: "rc", class: -1},
		{name: "p", class: 1},
	} {
		if number, ok := strings.CutPrefix(token, suffix.name); ok && (number == "" || decimalDigits(number)) {
			return gentooSuffix{class: suffix.class, number: number}
		}
	}
	// Unknown underscore components retain the old behavior of sorting after a release.
	return gentooSuffix{class: 2, raw: token}
}

func compareVersionTokens(a, b []string) int {
	for i := range min(len(a), len(b)) {
		if c := cmpToken(a[i], b[i]); c != 0 {
			return c
		}
	}
	return len(a) - len(b)
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

// Compare digit runs numerically without changing byte-wise ordering for other runs.
func cmpToken(a, b string) int {
	ai, bi := 0, 0
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	for ai < len(a) && bi < len(b) {
		if isDigit(a[ai]) && isDigit(b[bi]) {
			aj, bj := ai, bi
			for aj < len(a) && isDigit(a[aj]) {
				aj++
			}
			for bj < len(b) && isDigit(b[bj]) {
				bj++
			}
			if c := cmpNum(a[ai:aj], b[bi:bj]); c != 0 {
				return c
			}
			ai, bi = aj, bj
		} else {
			if a[ai] != b[bi] {
				if a[ai] < b[bi] {
					return -1
				}
				return 1
			}
			ai++
			bi++
		}
	}
	switch { // the token with more left is "greater" (e.g. "r" < "r2")
	case len(a)-ai < len(b)-bi:
		return -1
	case len(a)-ai > len(b)-bi:
		return 1
	default:
		return 0
	}
}

// cmpNum compares arbitrarily large digit strings without integer overflow.
func cmpNum(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	switch {
	case len(a) != len(b):
		if len(a) < len(b) {
			return -1
		}
		return 1
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// ebuildAtomVer extracts ("cat/pkg", "version") from an ebuild blob path "cat/pkg/pkg-VER.ebuild".
func ebuildAtomVer(path string) (string, string, bool) {
	if !strings.HasSuffix(path, ".ebuild") {
		return "", "", false
	}
	slash := strings.LastIndexByte(path, '/')
	if slash < 0 {
		return "", "", false
	}
	dir := path[:slash]    // cat/pkg
	file := path[slash+1:] // pkg-VER.ebuild
	pkg := dir[strings.LastIndexByte(dir, '/')+1:]
	ver := strings.TrimSuffix(file, ".ebuild")
	ver = strings.TrimPrefix(ver, pkg+"-")
	if ver == "" || strings.Contains(ver, "/") {
		return "", "", false
	}
	return dir, ver, true
}

func (o overlay) TreeURL(atom string) string {
	return "https://github.com/" + o.repo + "/tree/" + o.branch + "/" + atom
}

// A real overlay release outranks 9999; a 9999-only package still reports 9999.
func overlayPickVer(cur string, seen bool, ver string) string {
	if !seen || betterVer(cur, ver) {
		return ver
	}
	return cur
}

// fetchOverlay selects the newest version from a recursive GitHub tree.
func fetchOverlay(ctx context.Context, o overlay) (map[string]string, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", o.repo, o.branch)
	hdr := http.Header{"Accept": {"application/vnd.github+json"}}
	if githubToken != "" {
		hdr.Set("Authorization", "Bearer "+githubToken)
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := GetJSON(ctx, u, hdr, &tree); err != nil {
		return nil, err
	}
	if tree.Truncated {
		return nil, fmt.Errorf("%s tree is truncated (%d entries)", o.repo, len(tree.Tree))
	}
	pkgs := map[string]string{}
	for _, e := range tree.Tree {
		if e.Type != "blob" {
			continue
		}
		atom, ver, ok := ebuildAtomVer(e.Path)
		if !ok || !isPkgPath(atom) {
			continue
		}
		cur, seen := pkgs[atom]
		pkgs[atom] = overlayPickVer(cur, seen, ver)
	}
	return pkgs, nil
}

func (pc *pkgCache) Refresh(ctx context.Context) map[string]bool {
	return pc.refreshWith(ctx, Overlays, fetchOverlay)
}

func (pc *pkgCache) refreshWith(
	ctx context.Context,
	sources []overlay,
	fetch func(context.Context, overlay) (map[string]string, error),
) map[string]bool {
	pc.mu.Lock()
	if pc.available == nil {
		pc.available = map[string]bool{}
	}
	fresh := len(pc.pkgs) > 0 && time.Since(pc.fetched) < pkgCacheTTL
	// Retry throttling prevents GitHub rate-limit storms during outages.
	throttled := time.Since(pc.lastAttempt) < pkgRetryFloor
	if fresh || pc.refreshing || throttled {
		status := pc.availabilityLocked(sources)
		pc.mu.Unlock()
		return status
	}
	pc.refreshing = true
	pc.lastAttempt = time.Now()
	pc.mu.Unlock()
	defer func() {
		pc.mu.Lock()
		pc.refreshing = false
		pc.mu.Unlock()
	}()

	allOK := true
	for _, o := range sources {
		m, err := fetch(ctx, o)
		if err != nil {
			log.Printf("pkg cache: %v", err)
			pc.mu.Lock()
			pc.available[o.Name] = false
			pc.mu.Unlock()
			allOK = false
			continue
		}
		pc.mu.Lock()
		pc.pkgs[o.Name] = m
		pc.available[o.Name] = true
		pc.mu.Unlock()
		log.Printf("pkg cache: %s -> %d packages", o.Name, len(m))
	}
	// Partial refreshes retry after the floor, not the full TTL.
	if allOK {
		pc.mu.Lock()
		pc.fetched = time.Now()
		pc.mu.Unlock()
	}
	return pc.availability(sources)
}

func (pc *pkgCache) availability(sources []overlay) map[string]bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.availabilityLocked(sources)
}

func (pc *pkgCache) availabilityLocked(sources []overlay) map[string]bool {
	status := make(map[string]bool, len(sources))
	for _, o := range sources {
		ok, known := pc.available[o.Name]
		if !known {
			_, ok = pc.pkgs[o.Name]
		}
		status[o.Name] = ok
	}
	return status
}

func pn(atom string) string { return atom[strings.IndexByte(atom, '/')+1:] }

func (pc *pkgCache) Search(name string) map[string][]string {
	low := strings.ToLower(name)
	full := strings.Contains(low, "/") // query includes a category -> match the whole atom
	res := map[string][]string{}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for ov, atoms := range pc.pkgs {
		var exact, sub []string
		for atom := range atoms {
			p := strings.ToLower(pn(atom))
			if full {
				p = strings.ToLower(atom)
			}
			if p == low {
				exact = append(exact, atom)
			} else if strings.Contains(p, low) {
				sub = append(sub, atom)
			}
		}
		sort.Strings(exact)
		sort.Strings(sub)
		hits := append(exact, sub...)
		if len(hits) > maxHitsPerSource {
			hits = hits[:maxHitsPerSource]
		}
		if len(hits) > 0 {
			res[ov] = hits
		}
	}
	return res
}

func (pc *pkgCache) OverlayVer(ov, atom string) string {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if m, ok := pc.pkgs[ov]; ok {
		return m[atom]
	}
	return ""
}

type verInfo struct {
	stable, latest string
	fetched        time.Time
}

var verC = struct {
	mu sync.Mutex
	m  map[string]verInfo
}{m: map[string]verInfo{}}

// pkgVersionJSON is one entry of packages.gentoo.org's package "versions" array.
type pkgVersionJSON struct {
	Version  string        `json:"version"`
	Keywords []string      `json:"keywords"`
	Masks    []pkgMaskJSON `json:"masks"`
}

type pkgMaskJSON struct {
	Arches []string `json:"arches"`
}

func (v pkgVersionJSON) maskedOn(arch string) bool {
	for _, mask := range v.Masks {
		if len(mask.Arches) == 0 {
			return true
		}
		for _, maskedArch := range mask.Arches {
			if maskedArch == "*" || maskedArch == arch {
				return true
			}
		}
	}
	return false
}

// Input is newest-first; 9999 is excluded from stable/latest releases.
func pickStableLatest(versions []pkgVersionJSON) (stable, latest string) {
	for _, vv := range versions {
		if strings.HasPrefix(vv.Version, "9999") { // skip live ebuilds
			continue
		}
		if latest == "" {
			latest = vv.Version
		}
		if stable == "" && !vv.maskedOn("amd64") {
			for _, kw := range vv.Keywords {
				if kw == "amd64" {
					stable = vv.Version
					break
				}
			}
		}
		if latest != "" && stable != "" {
			break
		}
	}
	return stable, latest
}

// A 404 is an answered miss; transport, overload, and parse failures are unavailable.
func PkgVersion(ctx context.Context, atom string) (stable, latest string, available bool) {
	verC.mu.Lock()
	if v, ok := verC.m[atom]; ok && time.Since(v.fetched) < verCacheTTL {
		verC.mu.Unlock()
		return v.stable, v.latest, true
	}
	verC.mu.Unlock()

	var pj struct {
		Versions []pkgVersionJSON `json:"versions"`
	}
	err := GetJSON(ctx, "https://packages.gentoo.org/packages/"+atom+".json", nil, &pj)
	if err != nil {
		return "", "", httpStatusCode(err) == http.StatusNotFound
	}
	if len(pj.Versions) == 0 {
		return "", "", true
	}
	stable, latest = pickStableLatest(pj.Versions)
	verC.mu.Lock()
	if len(verC.m) >= pkgCacheMax {
		verC.m = map[string]verInfo{}
	}
	verC.m[atom] = verInfo{stable: stable, latest: latest, fetched: time.Now()}
	verC.mu.Unlock()
	return stable, latest, true
}

var (
	pkgHrefRe        = regexp.MustCompile(`/packages/([a-z][a-z0-9-]+/[A-Za-z0-9][A-Za-z0-9+_.\-]*)`)
	pkgDetailTitleRe = regexp.MustCompile(`<title>([a-z][a-z0-9-]+/[A-Za-z0-9][A-Za-z0-9+_.\-]*)\s+–\s+Gentoo Packages</title>`)
)

// Availability prevents official-tree outages from rendering as "not found".
func SearchMainTree(ctx context.Context, name string) ([]string, bool) {
	return searchMainTreeWith(ctx, name, PkgVersion, httpGetBody)
}

func searchMainTreeWith(
	ctx context.Context,
	name string,
	version func(context.Context, string) (string, string, bool),
	getBody func(context.Context, string, int64) ([]byte, error),
) ([]string, bool) {
	// Slashed atoms use authoritative JSON because the HTML search handles them poorly.
	if strings.Contains(name, "/") && isPkgPath(strings.ToLower(name)) {
		stable, latest, ok := version(ctx, name)
		if !ok {
			return nil, false
		}
		if stable != "" || latest != "" {
			return []string{name}, true
		}
		return nil, true
	}
	body, err := getBody(ctx, "https://packages.gentoo.org/packages/search?q="+url.QueryEscape(name), 2<<20)
	if err != nil {
		log.Printf("main tree search: %v", err)
		return nil, false
	}
	return rankSearchHits(body, name), true
}

// Re-rank deduplicated server hits while preserving its fuzzy matches.
// Exact package names and matching categories outrank incidental substrings.
func rankSearchHits(body []byte, name string) []string {
	seen := map[string]bool{}
	low := strings.ToLower(name)
	// A one-result fuzzy search redirects to a package page. Accept that page only
	// when its canonical atom still matches the query; otherwise it is a false hit.
	if m := pkgDetailTitleRe.FindStringSubmatch(string(body)); m != nil {
		if atom := m[1]; isPkgPath(atom) && pkgRelevance(atom, low) > 0 {
			return []string{atom}
		}
		return nil
	}
	type scored struct {
		atom  string
		score int
	}
	var items []scored
	for _, m := range pkgHrefRe.FindAllStringSubmatch(string(body), -1) {
		atom := m[1]
		if seen[atom] || !isPkgPath(atom) {
			continue
		}
		seen[atom] = true
		items = append(items, scored{atom, pkgRelevance(atom, low)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	hits := make([]string, 0, len(items))
	for _, it := range items {
		hits = append(hits, it.atom)
	}
	if len(hits) > maxHitsPerSource {
		hits = hits[:maxHitsPerSource]
	}
	return hits
}

// pkgRelevance ranks bare package queries.
func pkgRelevance(atom, q string) int {
	cat := ""
	if i := strings.IndexByte(atom, '/'); i > 0 {
		cat = strings.ToLower(atom[:i])
	}
	p := strings.ToLower(pn(atom))
	var score int
	switch {
	case p == q:
		score = 100
	case strings.Contains(cat, q):
		score = 50
	case strings.HasPrefix(p, q):
		score = 30
	case strings.Contains(p, q):
		score = 10
	}
	// Real packages outrank same-name metadata packages, which remain valid fallback hits.
	switch cat {
	case "virtual", "acct-group", "acct-user":
		score -= 5
	}
	return score
}

// Repology indexes bare names, while the Gentoo lookup retains the full atom.
func RepologyQuery(name string) string {
	if strings.Contains(name, "/") && isPkgPath(strings.ToLower(name)) {
		return name[strings.LastIndexByte(name, '/')+1:]
	}
	return name
}

func CommandArg(text string) string {
	// Fields accepts tabs and newlines from pasted commands.
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(fields[1:], " "))
}

type UseFlag struct {
	Name string
	Desc string
	Def  bool // default-enabled (+ prefix)
}

// Preserve USE_EXPAND groups so large sets such as l10n do not flood local flags.
type UseExpandGroup struct {
	Name  string
	Flags []UseFlag
}

type PkgFullInfo struct {
	Atom        string
	Description string
	Homepage    string
	Stable      string
	Latest      string
	Local       []UseFlag
	Global      []UseFlag
	Expand      []UseExpandGroup
	fetched     time.Time
}

var infoC = struct {
	mu sync.Mutex
	m  map[string]PkgFullInfo
}{m: map[string]PkgFullInfo{}}

type useEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func toUseFlags(in []useEntry) []UseFlag {
	out := make([]UseFlag, 0, len(in))
	for _, f := range in {
		out = append(out, UseFlag{
			Name: strings.TrimLeft(f.Name, "+-"),
			Desc: f.Description,
			Def:  strings.HasPrefix(f.Name, "+"),
		})
	}
	return out
}

// Only an authoritative 404 proves absence; other failures leave it unknown.
func OfficialInfo(ctx context.Context, atom string) (info PkgFullInfo, found, available bool) {
	infoC.mu.Lock()
	if v, ok := infoC.m[atom]; ok && time.Since(v.fetched) < verCacheTTL {
		infoC.mu.Unlock()
		return v, true, true
	}
	infoC.mu.Unlock()

	var pj struct {
		Description string           `json:"description"`
		Versions    []pkgVersionJSON `json:"versions"`
		Use         struct {
			Local  []useEntry `json:"local"`
			Global []useEntry `json:"global"`
		} `json:"use"`
		// USE_EXPAND is a sibling of use in the upstream schema.
		UseExpand []struct {
			Name  string     `json:"name"`
			Flags []useEntry `json:"flags"`
		} `json:"use_expand"`
	}
	err := GetJSON(ctx, "https://packages.gentoo.org/packages/"+atom+".json", nil, &pj)
	if err != nil {
		return PkgFullInfo{}, false, httpStatusCode(err) == http.StatusNotFound
	}
	info = PkgFullInfo{Atom: atom, Description: pj.Description, fetched: time.Now()}
	info.Stable, info.Latest = pickStableLatest(pj.Versions)
	info.Local = toUseFlags(pj.Use.Local)
	info.Global = toUseFlags(pj.Use.Global)
	for _, g := range pj.UseExpand {
		if fl := toUseFlags(g.Flags); len(fl) > 0 {
			info.Expand = append(info.Expand, UseExpandGroup{Name: g.Name, Flags: fl})
		}
	}
	infoC.mu.Lock()
	if len(infoC.m) >= pkgCacheMax {
		infoC.m = map[string]PkgFullInfo{}
	}
	infoC.m[atom] = info
	infoC.mu.Unlock()
	return info, true, true
}

func fetchRaw(ctx context.Context, url string) []byte {
	b, _ := httpGetBody(ctx, url, 1<<20)
	return b
}

// Parse multiline IUSE assignments while dropping shell expressions.
func parseIUSE(eb []byte) []string {
	lines := strings.Split(string(eb), "\n")
	var toks []string
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "IUSE=") && !strings.HasPrefix(t, "IUSE+=") {
			continue
		}
		q := strings.IndexByte(t, '"')
		if q < 0 {
			continue
		}
		content := t[q+1:]
		for {
			if end := strings.IndexByte(content, '"'); end >= 0 {
				toks = append(toks, strings.Fields(content[:end])...)
				break
			}
			toks = append(toks, strings.Fields(content)...)
			i++
			if i >= len(lines) {
				break
			}
			content = lines[i]
		}
	}
	out := make([]string, 0, len(toks))
	for _, tk := range toks {
		if tk == "" || strings.ContainsAny(tk, "${}()") {
			continue
		}
		out = append(out, tk)
	}
	return out
}

var ebuildFieldRe = map[string]*regexp.Regexp{}

var ebuildFieldMu sync.Mutex

func ebuildField(eb []byte, key string) string {
	ebuildFieldMu.Lock()
	re := ebuildFieldRe[key]
	if re == nil {
		re = regexp.MustCompile(`(?m)^` + key + `="?([^"\n]*)"?`)
		ebuildFieldRe[key] = re
	}
	ebuildFieldMu.Unlock()
	m := re.FindSubmatch(eb)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

var mdFlagRe = regexp.MustCompile(`(?s)<flag name="([^"]+)">(.*?)</flag>`)

var tagRe = regexp.MustCompile(`<[^>]+>`)

var wsRe = regexp.MustCompile(`\s+`)

func parseMetadataUse(md []byte) map[string]string {
	out := map[string]string{}
	for _, m := range mdFlagRe.FindAllSubmatch(md, -1) {
		desc := tagRe.ReplaceAllString(string(m[2]), "")
		desc = strings.TrimSpace(wsRe.ReplaceAllString(desc, " "))
		out[string(m[1])] = desc
	}
	return out
}

// Overlay metadata comes from the latest ebuild and metadata.xml.
func OverlayInfo(ctx context.Context, o overlay, atom, version string) (PkgFullInfo, bool) {
	if version == "" {
		return PkgFullInfo{}, false
	}
	pkg := pn(atom)
	base := "https://raw.githubusercontent.com/" + o.repo + "/" + o.branch + "/" + atom + "/"
	eb := fetchRaw(ctx, base+pkg+"-"+version+".ebuild")
	if eb == nil {
		return PkgFullInfo{}, false
	}
	descs := map[string]string{}
	if md := fetchRaw(ctx, base+"metadata.xml"); md != nil {
		descs = parseMetadataUse(md)
	}
	info := PkgFullInfo{
		Atom:        atom,
		Description: ebuildField(eb, "DESCRIPTION"),
		Latest:      version,
	}
	if hp := ebuildField(eb, "HOMEPAGE"); hp != "" {
		info.Homepage = strings.Fields(hp)[0]
	}
	for _, n := range parseIUSE(eb) {
		clean := strings.TrimLeft(n, "+-")
		info.Local = append(info.Local, UseFlag{Name: clean, Desc: descs[clean], Def: strings.HasPrefix(n, "+")})
	}
	return info, true
}

// Bound compact USE_EXPAND output; l10n commonly exceeds 100 values.
const ExpandCap = 16

func OverlayByName(name string) (overlay, bool) {
	for _, o := range Overlays {
		if o.Name == name {
			return o, true
		}
	}
	return overlay{}, false
}

// Normalize supported package and overlay URLs to category/package atoms.
func NormalizeQuery(q string) string {
	q = strings.TrimSpace(q)
	q = strings.SplitN(q, "?", 2)[0]
	q = strings.SplitN(q, "#", 2)[0]
	if i := strings.Index(q, "packages.gentoo.org/packages/"); i >= 0 {
		rest := strings.TrimRight(q[i+len("packages.gentoo.org/packages/"):], "/")
		if parts := strings.SplitN(rest, "/", 3); len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			atom := parts[0] + "/" + strings.TrimSuffix(parts[1], ".json")
			if isPkgPath(strings.ToLower(atom)) {
				return atom
			}
		}
	}
	if strings.Contains(q, "github.com/") {
		for _, marker := range []string{"/tree/", "/blob/"} {
			if i := strings.Index(q, marker); i >= 0 {
				// layout after the marker is <branch>/<category>/<package>[/...]
				if segs := strings.Split(strings.TrimRight(q[i+len(marker):], "/"), "/"); len(segs) >= 3 {
					atom := segs[1] + "/" + segs[2]
					if isPkgPath(strings.ToLower(atom)) {
						return atom
					}
				}
			}
		}
	}
	return q
}

type UseSrc struct {
	Official bool
	Ovs      []string
}

// Empty matches are definitive only when every source answered.
func ResolveUseSources(ctx context.Context, q string, overlayOK map[string]bool) (map[string]*UseSrc, PkgLookupAvailability) {
	return resolveUseSourcesWith(ctx, q, overlayOK, OfficialInfo, SearchMainTree)
}

func resolveUseSourcesWith(
	ctx context.Context,
	q string,
	overlayOK map[string]bool,
	info func(context.Context, string) (PkgFullInfo, bool, bool),
	search func(context.Context, string) ([]string, bool),
) (map[string]*UseSrc, PkgLookupAvailability) {
	srcs := map[string]*UseSrc{}
	availability := PkgLookupAvailability{Overlays: overlayOK}
	get := func(a string) *UseSrc {
		s := srcs[a]
		if s == nil {
			s = &UseSrc{}
			srcs[a] = s
		}
		return s
	}

	low := strings.ToLower(q)
	if strings.Contains(low, "/") && isPkgPath(low) {
		_, found, ok := info(ctx, q)
		availability.Official = ok
		if found {
			get(q).Official = true
		}
		for _, o := range Overlays {
			if PkgC.OverlayVer(o.Name, q) != "" {
				s := get(q)
				s.Ovs = append(s.Ovs, o.Name)
			}
		}
		return srcs, availability
	}
	atoms, ok := search(ctx, q)
	availability.Official = ok
	for _, a := range atoms {
		if strings.EqualFold(pn(a), q) {
			get(a).Official = true
		}
	}
	for ov, list := range PkgC.Search(q) {
		for _, a := range list {
			if strings.EqualFold(pn(a), q) {
				s := get(a)
				s.Ovs = append(s.Ovs, ov)
			}
		}
	}
	return srcs, availability
}

// ok distinguishes lookup failure from a package with no arm64 keyword.
func ArmStatus(ctx context.Context, atom string) (stable, testing string, ok bool) {
	var pj struct {
		Versions []pkgVersionJSON `json:"versions"`
	}
	if err := GetJSON(ctx, "https://packages.gentoo.org/packages/"+atom+".json", nil, &pj); err != nil {
		return "", "", false
	}
	stable, testing = arm64Keywords(pj.Versions)
	return stable, testing, true
}

// packages.gentoo.org returns newest first; live ebuilds are skipped.
func arm64Keywords(versions []pkgVersionJSON) (stable, testing string) {
	for _, vv := range versions {
		if strings.HasPrefix(vv.Version, "9999") || vv.maskedOn("arm64") {
			continue
		}
		for _, kw := range vv.Keywords {
			switch kw {
			case "arm64":
				if stable == "" {
					stable = vv.Version
				}
			case "~arm64":
				if testing == "" {
					testing = vv.Version
				}
			}
		}
	}
	return stable, testing
}
