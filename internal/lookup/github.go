package lookup

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	githubAtomNamespace = "http://www.w3.org/2005/Atom"
	githubAtomBaseURL   = "https://github.com"
	maxGitHubAtomBytes  = 4 << 20
	maxGitHubRESTBytes  = 4 << 20
)

var githubCommitIDRe = regexp.MustCompile(`^tag:github\.com,[0-9]{4}:Grit::Commit/([0-9a-f]{40})$`)

// Commit is one validated entry from a GitHub Atom commit feed.
type Commit struct {
	ID       string
	SHA      string
	ShortSHA string
	Title    string
	Author   string
	URL      string
}

var githubAtomBase = githubAtomBaseURL
var githubAPIBase = "https://api.github.com"

// GitHubItem is one validated issue or pull request creation record.
type GitHubItem struct {
	Number   int
	Title    string
	Author   string
	URL      string
	IsPull   bool
	State    string
	MergedAt *string
}

func parseGitHubItems(body []byte, repo string) ([]GitHubItem, bool, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false, fmt.Errorf("GitHub REST root is not an array")
	}
	var rawItems []struct {
		Number    int    `json:"number"`
		HTMLURL   string `json:"html_url"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
		State     string `json:"state"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		PullRequest *struct {
			MergedAt json.RawMessage `json:"merged_at"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &rawItems); err != nil {
		return nil, false, err
	}
	items := make([]GitHubItem, len(rawItems))
	const githubURLPrefix = "https://github.com/"
	repoEnd := len(githubURLPrefix) + len(repo)
	for index, raw := range rawItems {
		if raw.Number <= 0 {
			return nil, false, fmt.Errorf("GitHub REST item %d has invalid number", index)
		}
		if len(raw.HTMLURL) <= repoEnd || !strings.HasPrefix(raw.HTMLURL, githubURLPrefix) ||
			!strings.EqualFold(raw.HTMLURL[len(githubURLPrefix):repoEnd], repo) || raw.HTMLURL[repoEnd] != '/' {
			return nil, false, fmt.Errorf("GitHub REST item %d has invalid html_url", index)
		}
		title := sanitizeGitHubTitle(raw.Title)
		if title == "" {
			return nil, false, fmt.Errorf("GitHub REST item %d has empty title", index)
		}
		if strings.TrimSpace(raw.User.Login) == "" {
			return nil, false, fmt.Errorf("GitHub REST item %d has empty user.login", index)
		}
		if _, err := time.Parse(time.RFC3339, raw.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("GitHub REST item %d has invalid created_at", index)
		}
		if raw.State != "open" && raw.State != "closed" {
			return nil, false, fmt.Errorf("GitHub REST item %d has invalid state", index)
		}
		item := GitHubItem{Number: raw.Number, Title: title, Author: raw.User.Login, URL: raw.HTMLURL, State: raw.State}
		if raw.PullRequest != nil {
			item.IsPull = true
			if raw.PullRequest.MergedAt != nil && !bytes.Equal(raw.PullRequest.MergedAt, []byte("null")) {
				var mergedAt string
				if err := json.Unmarshal(raw.PullRequest.MergedAt, &mergedAt); err != nil {
					return nil, false, fmt.Errorf("GitHub REST item %d has invalid pull_request.merged_at", index)
				}
				if _, err := time.Parse(time.RFC3339, mergedAt); err != nil {
					return nil, false, fmt.Errorf("GitHub REST item %d has invalid pull_request.merged_at", index)
				}
				item.MergedAt = &mergedAt
			}
		}
		items[index] = item
	}
	return items, len(rawItems) == 30, nil
}

func sanitizeGitHubTitle(title string) string {
	runes := make([]rune, 0, min(len(title), 200))
	for _, value := range title {
		if !unicode.IsControl(value) {
			runes = append(runes, value)
		}
	}
	runes = []rune(strings.TrimSpace(string(runes)))
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return string(runes)
}

// configureGitHub resets the base on every New call; direct callers also get slash normalization.
func configureGitHub(cfg *settings.Config) {
	githubAtomBase = githubAtomBaseURL
	githubAPIBase = "https://api.github.com"
	if cfg != nil && cfg.GitHubAtomBase != "" {
		base := strings.TrimRight(cfg.GitHubAtomBase, "/")
		if base != "" {
			githubAtomBase = base
		}
	}
	if cfg != nil && cfg.GitHubAPIBase != "" {
		apiBase := strings.TrimRight(cfg.GitHubAPIBase, "/")
		if apiBase != "" {
			githubAPIBase = apiBase
		}
	}
}

func configureFeedSources(cfg *settings.Config) {
	configureNews(cfg)
	configureGitHub(cfg)
}

// RecentCommits fetches and validates a repository's current Atom commit page.
func RecentCommits(ctx context.Context, repo, branch string) ([]Commit, error) {
	endpoint := githubCommitFeedURL(repo, branch)
	body, err := httpGetBody(ctx, endpoint, maxGitHubAtomBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub Atom: %w", err)
	}
	commits, err := parseGitHubAtom(body, repo)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub Atom: %w", err)
	}
	return commits, nil
}

// RecentGitHubItems fetches and validates one GitHub issue/PR REST page.
func RecentGitHubItems(ctx context.Context, repo string) ([]GitHubItem, bool, error) {
	endpoint := githubAPIBase + "/repos/" + repo + "/issues?state=all&sort=created&direction=desc&per_page=30"
	headers := http.Header{"Accept": {"application/vnd.github+json"}}
	if githubToken != "" {
		headers.Set("Authorization", "Bearer "+githubToken)
	}
	response, err := httpGet(ctx, endpoint, headers)
	if err != nil {
		return nil, false, fmt.Errorf("fetch GitHub REST: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGitHubRESTBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("fetch GitHub REST: %w", err)
	}
	if len(body) > maxGitHubRESTBytes {
		return nil, false, &httpBodyTooLargeError{url: endpoint, limit: maxGitHubRESTBytes}
	}
	items, full, err := parseGitHubItems(body, repo)
	if err != nil {
		return nil, false, fmt.Errorf("parse GitHub REST: %w", err)
	}
	return items, full, nil
}

func githubCommitFeedURL(repo, branch string) string {
	base := githubAtomBase
	path := base + "/" + repo + "/commits"
	if branch == "" {
		return path + ".atom"
	}
	segments := strings.Split(branch, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return path + "/" + strings.Join(segments, "/") + ".atom"
}

func parseGitHubAtom(body []byte, repo string) ([]Commit, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, err := nextXMLRoot(decoder)
	if err != nil {
		return nil, err
	}
	if root.Name != (xml.Name{Space: githubAtomNamespace, Local: "feed"}) {
		return nil, fmt.Errorf("GitHub Atom root is %s{%s}, want {%s}feed", root.Name.Local, root.Name.Space, githubAtomNamespace)
	}

	commits := make([]Commit, 0)
	seen := make(map[string]struct{})
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read GitHub Atom feed: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			if token.Name.Space == githubAtomNamespace && token.Name.Local == "entry" {
				commit, err := parseGitHubEntry(decoder, token, repo)
				if err != nil {
					return nil, err
				}
				if _, duplicate := seen[commit.ID]; duplicate {
					return nil, fmt.Errorf("duplicate GitHub Atom entry ID %q", commit.ID)
				}
				seen[commit.ID] = struct{}{}
				commits = append(commits, commit)
				continue
			}
			if err := skipXMLElement(decoder, token); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if err := ensureXMLTail(decoder); err != nil {
				return nil, err
			}
			return commits, nil
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return nil, fmt.Errorf("unexpected text outside GitHub Atom feed")
			}
		}
	}
}

func nextXMLRoot(decoder *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, fmt.Errorf("read GitHub Atom root: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			return token, nil
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return xml.StartElement{}, fmt.Errorf("unexpected text before GitHub Atom root")
			}
		}
	}
}

func ensureXMLTail(decoder *xml.Decoder) error {
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read trailing GitHub Atom XML: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			return fmt.Errorf("unexpected trailing GitHub Atom element %q", token.Name.Local)
		case xml.EndElement:
			return fmt.Errorf("unexpected trailing GitHub Atom element %q", token.Name.Local)
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return fmt.Errorf("unexpected trailing text after GitHub Atom feed")
			}
		}
	}
}

type githubEntryFields struct {
	id, title, author             string
	idSeen, titleSeen, authorSeen bool
}

func parseGitHubEntry(decoder *xml.Decoder, start xml.StartElement, repo string) (Commit, error) {
	fields, err := readGitHubEntryFields(decoder, start)
	if err != nil {
		return Commit{}, err
	}
	return buildGitHubCommit(fields, repo)
}

func readGitHubEntryFields(decoder *xml.Decoder, start xml.StartElement) (githubEntryFields, error) {
	var fields githubEntryFields
	for {
		tok, err := decoder.Token()
		if err != nil {
			return fields, fmt.Errorf("read GitHub Atom entry: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			if token.Name.Space != githubAtomNamespace {
				if err := skipXMLElement(decoder, token); err != nil {
					return fields, err
				}
				continue
			}
			if err := readGitHubEntryField(decoder, token, &fields); err != nil {
				return fields, err
			}
		case xml.EndElement:
			if token.Name != start.Name {
				return fields, fmt.Errorf("unexpected GitHub Atom entry closing element %q", token.Name.Local)
			}
			return fields, nil
		}
	}
}

func readGitHubEntryField(decoder *xml.Decoder, token xml.StartElement, fields *githubEntryFields) error {
	switch token.Name.Local {
	case "id":
		if fields.idSeen {
			return fmt.Errorf("GitHub Atom entry has multiple id elements")
		}
		value, err := readXMLText(decoder, token)
		if err != nil {
			return fmt.Errorf("read GitHub Atom entry ID: %w", err)
		}
		fields.id, fields.idSeen = value, true
	case "title":
		if fields.titleSeen {
			return fmt.Errorf("GitHub Atom entry has multiple title elements")
		}
		titleType, hasTitleType := xmlAttribute(token, "type")
		if hasTitleType && titleType != "text" {
			return fmt.Errorf("unsupported GitHub Atom title type %q", titleType)
		}
		value, err := readXMLText(decoder, token)
		if err != nil {
			return fmt.Errorf("read GitHub Atom title: %w", err)
		}
		fields.title, fields.titleSeen = strings.TrimSpace(value), true
	case "author":
		if fields.authorSeen {
			return fmt.Errorf("GitHub Atom entry has multiple author elements")
		}
		value, err := parseGitHubAuthor(decoder, token)
		if err != nil {
			return err
		}
		fields.author, fields.authorSeen = value, true
	default:
		return skipXMLElement(decoder, token)
	}
	return nil
}

func buildGitHubCommit(fields githubEntryFields, repo string) (Commit, error) {
	match := githubCommitIDRe.FindStringSubmatch(fields.id)
	if !fields.idSeen || len(match) != 2 {
		return Commit{}, fmt.Errorf("invalid GitHub Atom commit ID %q", fields.id)
	}
	if !fields.titleSeen {
		return Commit{}, fmt.Errorf("GitHub Atom entry is missing title")
	}
	sha := match[1]
	return Commit{
		ID:       fields.id,
		SHA:      sha,
		ShortSHA: sha[:7],
		Title:    fields.title,
		Author:   fields.author,
		URL:      githubAtomBase + "/" + repo + "/commit/" + sha,
	}, nil
}

func parseGitHubAuthor(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var author string
	for {
		tok, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("read GitHub Atom author: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			if token.Name.Space == githubAtomNamespace && token.Name.Local == "name" {
				name, err := readXMLText(decoder, token)
				if err != nil {
					return "", fmt.Errorf("read GitHub Atom author name: %w", err)
				}
				if author == "" {
					author = strings.TrimSpace(name)
				}
				continue
			}
			if err := skipXMLElement(decoder, token); err != nil {
				return "", err
			}
		case xml.EndElement:
			if token.Name != start.Name {
				return "", fmt.Errorf("unexpected GitHub Atom author closing element %q", token.Name.Local)
			}
			return author, nil
		}
	}
}

func readXMLText(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var text strings.Builder
	for {
		tok, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch token := tok.(type) {
		case xml.CharData:
			_, _ = text.Write(token)
		case xml.StartElement:
			return "", fmt.Errorf("element %q is not valid inside plain text", token.Name.Local)
		case xml.EndElement:
			if token.Name != start.Name {
				return "", fmt.Errorf("unexpected closing element %q in plain text", token.Name.Local)
			}
			return text.String(), nil
		}
	}
}

func xmlAttribute(start xml.StartElement, local string) (string, bool) {
	for _, attr := range start.Attr {
		if attr.Name.Space == "" && attr.Name.Local == local {
			return attr.Value, true
		}
	}
	return "", false
}

func skipXMLElement(decoder *xml.Decoder, start xml.StartElement) error {
	if err := decoder.Skip(); err != nil {
		return fmt.Errorf("skip GitHub Atom element %q: %w", start.Name.Local, err)
	}
	return nil
}
