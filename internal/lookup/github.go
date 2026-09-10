package lookup

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	githubAtomNamespace = "http://www.w3.org/2005/Atom"
	githubAtomBaseURL   = "https://github.com"
	maxGitHubAtomBytes  = 4 << 20
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

// configureGitHub resets the base on every New call; direct callers also get slash normalization.
func configureGitHub(cfg *settings.Config) {
	githubAtomBase = githubAtomBaseURL
	if cfg != nil && cfg.GitHubAtomBase != "" {
		base := strings.TrimRight(cfg.GitHubAtomBase, "/")
		if base != "" {
			githubAtomBase = base
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
