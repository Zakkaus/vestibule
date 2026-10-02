package lookup

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

var BugIDRe = regexp.MustCompile(`^[0-9]{1,9}$`)

type BugInfo struct {
	Summary, Status, Resolution, Product, Component, Severity string
}

type BugLookupState uint8

const (
	bugLookupUnavailable BugLookupState = iota
	BugLookupFound
	BugLookupNotFound
)

type bugResponse struct {
	Error bool `json:"error"`
	Bugs  []struct {
		Summary    string `json:"summary"`
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
		Product    string `json:"product"`
		Component  string `json:"component"`
		Severity   string `json:"severity"`
	} `json:"bugs"`
}

// Only an HTTP 404 is authoritative; malformed, restricted, and failed responses are retryable.
func FetchBug(ctx context.Context, id string) (BugInfo, BugLookupState) {
	u := "https://bugs.gentoo.org/rest/bug/" + id +
		"?include_fields=summary,status,resolution,product,component,severity"
	var br bugResponse
	if err := GetJSON(ctx, u, nil, &br); err != nil {
		if httpStatusCode(err) == http.StatusNotFound {
			return BugInfo{}, BugLookupNotFound
		}
		return BugInfo{}, bugLookupUnavailable
	}
	if br.Error || len(br.Bugs) == 0 {
		return BugInfo{}, bugLookupUnavailable
	}
	b := br.Bugs[0]
	return BugInfo{b.Summary, b.Status, b.Resolution, b.Product, b.Component, b.Severity}, BugLookupFound
}

// NewsItem is one parsed Gentoo news index entry.
type NewsItem struct {
	// Date is the publication date shown by the news index.
	Date string
	// Title is the upstream news title.
	Title string
	// URL is the absolute upstream news URL.
	URL string
}

const newsTTL = 30 * time.Minute

var newsURL = "https://www.gentoo.org/support/news-items/"

var newsBase = "https://www.gentoo.org"

func configureNews(cfg *settings.Config) {
	if cfg.NewsURL != "" {
		newsURL = cfg.NewsURL
	}
	if u, err := url.Parse(newsURL); err == nil && u.Scheme != "" && u.Host != "" {
		newsBase = u.Scheme + "://" + u.Host
	}
}

var (
	// The index date is authoritative: a few historical URL slugs encode a different date.
	newsRowRe = regexp.MustCompile(`(?s)<tr>\s*<td>\s*(\d{4}-\d{2}-\d{2})\s*</td>\s*<td>\s*<a href="(/support/news-items/\d{4}-\d{2}-\d{2}-[^"]+\.html)"[^>]*>([^<]+)</a>`)
	// Keep configurable simple link indexes working when they do not provide table dates.
	newsRe = regexp.MustCompile(`href="(/support/news-items/(\d{4}-\d{2}-\d{2})-[^"]+\.html)"[^>]*>([^<]+)<`)
)

var newsC = struct {
	mu      sync.Mutex
	items   []NewsItem
	fetched time.Time
	loading bool
}{}

// FetchNews fetches and parses the current Gentoo news index without using the command cache.
func FetchNews(c context.Context) ([]NewsItem, error) {
	body, err := httpGetBody(c, newsURL, 2<<20)
	if err != nil {
		return nil, err
	}
	items := parseNews(body)
	if len(items) == 0 && len(body) > 0 {
		// Treat markup drift as unavailable so it cannot become an authoritative empty index.
		return nil, fmt.Errorf("parsed 0 items from %d bytes of %s; the news page layout may have changed", len(body), newsURL)
	}
	return items, nil
}

func parseNews(body []byte) []NewsItem {
	seen := map[string]bool{}
	var items []NewsItem
	add := func(path, date, title string) {
		title = strings.TrimSpace(title)
		if seen[path] || title == "" {
			return
		}
		seen[path] = true
		items = append(items, NewsItem{Date: date, Title: title, URL: newsBase + path})
	}
	text := string(body)
	for _, m := range newsRowRe.FindAllStringSubmatch(text, -1) {
		add(m[2], m[1], m[3])
	}
	for _, m := range newsRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2], m[3])
	}
	return items
}

func GetNews(c context.Context) ([]NewsItem, bool) {
	newsC.mu.Lock()
	fresh := !newsC.fetched.IsZero() && time.Since(newsC.fetched) < newsTTL
	if fresh || newsC.loading {
		items := newsC.items
		newsC.mu.Unlock()
		return items, fresh
	}
	newsC.loading = true
	newsC.mu.Unlock()
	defer func() {
		newsC.mu.Lock()
		newsC.loading = false
		newsC.mu.Unlock()
	}()
	items, err := FetchNews(c)
	if err != nil {
		log.Printf("news fetch: %v", err)
		newsC.mu.Lock()
		old := newsC.items
		newsC.mu.Unlock()
		return old, false
	}
	newsC.mu.Lock()
	newsC.items, newsC.fetched = items, time.Now()
	newsC.mu.Unlock()
	return items, true
}

// classify groups translation titles by base topic and supported language.
type WikiSource struct {
	Name      string
	api       string
	titleBase string
	classify  func(title string) (base, lang string)
}

// Gentoo translation subpages use short language suffixes; longer subpages are content.
var gentooLangRe = regexp.MustCompile(`/([a-z]{2}(?:-[a-z]{2,4})?)$`)

func classifyGentoo(title string) (string, string) {
	m := gentooLangRe.FindStringSubmatch(title)
	if m == nil {
		return title, "en"
	}
	base := title[:len(title)-len(m[0])]
	switch strings.ToLower(m[1]) {
	case "zh-cn", "zh-hans":
		return base, "zh"
	case "zh-tw", "zh-hant":
		return base, "zh-Hant"
	default:
		return base, "other"
	}
}

// Arch translation titles use a parenthesized language label.
var archLangRe = regexp.MustCompile(` \(([^)]+)\)$`)

func classifyArch(title string) (string, string) {
	m := archLangRe.FindStringSubmatch(title)
	if m == nil {
		return title, "en"
	}
	base := title[:len(title)-len(m[0])]
	switch m[1] {
	case "\u7b80\u4f53\u4e2d\u6587":
		return base, "zh"
	case "\u7e41\u9ad4\u4e2d\u6587", "\u6b63\u9ad4\u4e2d\u6587":
		return base, "zh-Hant"
	default:
		return base, "other"
	}
}

var WikiSources = []WikiSource{
	{Name: "Gentoo", api: "https://wiki.gentoo.org/api.php", titleBase: "https://wiki.gentoo.org/wiki/", classify: classifyGentoo},
	{Name: "Arch", api: "https://wiki.archlinux.org/api.php", titleBase: "https://wiki.archlinux.org/title/", classify: classifyArch},
}

// Escape each path segment separately so MediaWiki subpages retain their slashes.
func wikiTitlePath(title string) string {
	parts := strings.Split(strings.ReplaceAll(title, " ", "_"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (w WikiSource) PageURL(title string) string { return w.titleBase + wikiTitlePath(title) }

// Drop untagged foreign-language pages from the English fallback.
func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

func CleanDisplayTitle(s string) string {
	return html.UnescapeString(strings.TrimSpace(tagRe.ReplaceAllString(s, "")))
}

// ok distinguishes a failed wiki fetch from an authoritative empty search.
func SearchTitles(ctx context.Context, w WikiSource, query string, limit int) (titles []string, ok bool) {
	u := fmt.Sprintf("%s?action=query&list=search&srsearch=%s&srlimit=%d&srprop=&format=json",
		w.api, url.QueryEscape(query), limit)
	var resp struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := GetJSON(ctx, u, nil, &resp); err != nil {
		return nil, false // transient fetch failure — NOT a genuine "no results"
	}
	out := make([]string, 0, len(resp.Query.Search))
	for _, s := range resp.Query.Search {
		out = append(out, s.Title)
	}
	return out, true
}

// Display titles supply localized headings for canonical page names.
func DisplayTitles(ctx context.Context, w WikiSource, titles []string) map[string]string {
	out := map[string]string{}
	if len(titles) == 0 {
		return out
	}
	u := fmt.Sprintf("%s?action=query&prop=info&inprop=displaytitle&format=json&titles=%s",
		w.api, url.QueryEscape(strings.Join(titles, "|")))
	var resp struct {
		Query struct {
			Pages map[string]struct {
				Title        string `json:"title"`
				Displaytitle string `json:"displaytitle"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := GetJSON(ctx, u, nil, &resp); err != nil {
		return out
	}
	for _, p := range resp.Query.Pages {
		if p.Displaytitle != "" {
			out[p.Title] = p.Displaytitle
		}
	}
	return out
}

// Dedupe topics case-insensitively, preferring the requester's language.
func (w WikiSource) PickWikiTitles(l i18n.Lang, titles []string, max int) []string {
	type entry struct {
		title string
		lang  string
	}
	preferred := l.String()
	rank := func(language string) int {
		if language == preferred {
			return 0
		}
		if language == "en" {
			return 1
		}
		return 2
	}
	chosen := map[string]entry{}
	var order []string
	for _, title := range titles {
		base, language := w.classify(title)
		if language == "other" || (language == "en" && hasNonASCII(base)) {
			continue
		}
		key := strings.ToLower(base)
		if current, ok := chosen[key]; ok {
			if rank(language) < rank(current.lang) {
				chosen[key] = entry{title: title, lang: language}
			}
			continue
		}
		chosen[key] = entry{title: title, lang: language}
		order = append(order, key)
	}
	var primary, fallback []string
	for _, key := range order {
		if chosen[key].lang == preferred {
			primary = append(primary, chosen[key].title)
		} else {
			fallback = append(fallback, chosen[key].title)
		}
	}
	out := append(primary, fallback...)
	if len(out) > max {
		out = out[:max]
	}
	return out
}

const archcnForum = "https://forum.archlinuxcn.org"

type ForumTopic struct{ Title, Url string }

// ok distinguishes a fetch failure from an authoritative empty search.
func SearchArchcn(ctx context.Context, query string, limit int) (topics []ForumTopic, ok bool) {
	u := archcnForum + "/search.json?q=" + url.QueryEscape(query)
	var resp struct {
		Topics []struct {
			ID    int    `json:"id"`
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"topics"`
	}
	if err := GetJSON(ctx, u, nil, &resp); err != nil {
		return nil, false // transient fetch failure — NOT a genuine "no results"
	}
	out := make([]ForumTopic, 0, limit)
	for _, t := range resp.Topics {
		out = append(out, ForumTopic{t.Title, fmt.Sprintf("%s/t/%s/%d", archcnForum, t.Slug, t.ID)})
		if len(out) >= limit {
			break
		}
	}
	return out, true
}

var ForumLinks = []struct {
	Label i18n.Text
	Site  string
}{
	{i18n.Messages.LookupContent.BBS.GentooForum, "forums.gentoo.org"},
	{i18n.Messages.LookupContent.BBS.ArchBBS, "bbs.archlinux.org"},
	{i18n.Messages.LookupContent.BBS.UbuntuForum, "ubuntuforums.org"},
	{i18n.Messages.LookupContent.BBS.DebianForum, "forums.debian.net"},
}

func DdgSiteSearch(site, query string) string {
	return "https://duckduckgo.com/?q=" + url.QueryEscape("site:"+site+" "+query)
}
