package lookup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	githubTestSHA1 = "0123456789abcdef0123456789abcdef01234567"
	githubTestSHA2 = "89abcdef0123456789abcdef0123456789abcdef"
	githubTestSHA3 = "fedcba9876543210fedcba9876543210fedcba98"
)

func TestParseGitHubAtomFixtures(t *testing.T) {
	body, err := os.ReadFile("../../testdata/upstream/github/commits-vestibule.atom")
	if err != nil {
		t.Fatal(err)
	}
	commits, err := parseGitHubAtom(body, "Zakkaus/vestibule")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 20 {
		t.Fatalf("vestibule fixture entries = %d, want 20", len(commits))
	}
	if commits[0].SHA != "dcb02d0c24b4eec5eff12bcf3263b8953dec878a" || commits[0].ShortSHA != "dcb02d0" {
		t.Fatalf("first fixture commit = %+v", commits[0])
	}
	if commits[0].URL != "https://github.com/Zakkaus/vestibule/commit/"+commits[0].SHA {
		t.Fatalf("first fixture URL = %q", commits[0].URL)
	}

	overlay, err := os.ReadFile("../../testdata/upstream/github/commits-overlay.atom")
	if err != nil {
		t.Fatal(err)
	}
	commits, err = parseGitHubAtom(overlay, "gentoo-zh/overlay")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 20 {
		t.Fatalf("overlay fixture entries = %d, want 20", len(commits))
	}
}

func TestParseGitHubAtomThreeAndZeroEntries(t *testing.T) {
	three, err := parseGitHubAtom(githubAtomBody(
		githubAtomEntry(githubTestSHA1, "one", "alice", ""),
		githubAtomEntry(githubTestSHA2, "two", "bob", "text"),
		githubAtomEntry(githubTestSHA3, "three", "", ""),
	), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(three) != 3 || three[0].SHA != githubTestSHA1 || three[2].Title != "three" {
		t.Fatalf("three-entry page = %+v", three)
	}
	zero, err := parseGitHubAtom([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"></feed>`), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(zero) != 0 {
		t.Fatalf("empty page = %+v, want no commits", zero)
	}
}

func TestParseGitHubAtomFieldAndNamespaceBoundaries(t *testing.T) {
	valid := githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>Document &amp;lt;tag&amp;gt; literally</title><author><name>A &amp;amp; B</name></author><link rel="alternate" href="javascript:alert(1)"/></entry>`)
	commits, err := parseGitHubAtom(valid, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if commits[0].Title != "Document &lt;tag&gt; literally" || commits[0].Author != "A &amp; B" {
		t.Fatalf("XML text was decoded more than once: %+v", commits[0])
	}
	if commits[0].URL != "https://github.com/owner/repo/commit/"+githubTestSHA1 {
		t.Fatalf("commit URL used upstream link: %q", commits[0].URL)
	}

	for name, body := range map[string][]byte{
		"explicit text title":         githubAtomBody(githubAtomEntry(githubTestSHA1, "title", "", "text")),
		"missing author name":         githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>title</title><author/></entry>`),
		"wrong namespace author name": githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom" xmlns:x="urn:foreign"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>title</title><author><x:name>not an author</x:name></author></entry>`),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseGitHubAtom(body, "owner/repo")
			if err != nil {
				t.Fatal(err)
			}
			if name == "explicit text title" && got[0].Title != "title" {
				t.Fatalf("explicit text title = %q", got[0].Title)
			}
			if name == "missing author name" && got[0].Author != "" {
				t.Fatalf("missing author was not omitted: %q", got[0].Author)
			}
			if name == "wrong namespace author name" && got[0].Author != "" {
				t.Fatalf("foreign author name was accepted: %q", got[0].Author)
			}
		})
	}

	for name, body := range map[string][]byte{
		"wrong root namespace":       []byte(`<feed xmlns="urn:foreign"></feed>`),
		"foreign entry ID":           []byte(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:x="urn:foreign"><entry><x:id>x</x:id><title>title</title></entry></feed>`),
		"missing ID":                 githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><title>title</title></entry>`),
		"missing title":              githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id></entry>`),
		"unsupported title type":     githubAtomBody(githubAtomEntry(githubTestSHA1, "title", "", "html")),
		"nested title element":       githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>bad <b>child</b></title></entry>`),
		"nested author name element": githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>title</title><author><name>bad <b>child</b></name></author></entry>`),
		"duplicate ID": githubAtomBody(
			githubAtomEntry(githubTestSHA1, "one", "", ""),
			githubAtomEntry(githubTestSHA1, "two", "", ""),
		),
		"invalid ID":             githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit/` + githubTestSHA1 + `</id><title>title</title></entry>`),
		"uppercase SHA":          githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + strings.ToUpper(githubTestSHA1) + `</id><title>title</title></entry>`),
		"wrong year width":       githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,200:Grit::Commit/` + githubTestSHA1 + `</id><title>title</title></entry>`),
		"ID surrounding space":   githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id> tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title>title</title></entry>`),
		"empty title type":       githubAtomBody(`<entry xmlns="http://www.w3.org/2005/Atom"><id>tag:github.com,2008:Grit::Commit/` + githubTestSHA1 + `</id><title type="">title</title></entry>`),
		"trailing malformed XML": append(githubAtomBody(githubAtomEntry(githubTestSHA1, "title", "", "")), []byte("<broken>")...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseGitHubAtom(body, "owner/repo"); err == nil {
				t.Fatal("invalid Atom page was accepted")
			}
		})
	}
}

func TestRecentCommitsUsesConfiguredURLAndHeaders(t *testing.T) {
	oldBase, oldAgent, oldToken := githubAtomBase, userAgent, githubToken
	t.Cleanup(func() { githubAtomBase, userAgent, githubToken = oldBase, oldAgent, oldToken })
	userAgent = "github-test-agent"
	githubToken = "must-not-be-sent"
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("unexpected credentials header %q", got)
		}
		_, _ = w.Write(githubAtomBody(githubAtomEntry(githubTestSHA1, "title", "", "")))
	}))
	defer server.Close()
	configureGitHub(&settings.Config{GitHubAtomBase: server.URL + "/git///"})

	branches := []struct {
		branch string
		path   string
	}{
		{"", "/git/owner/repo/commits.atom"},
		{"v5/next", "/git/owner/repo/commits/v5/next.atom"},
		{"feature/keep#x", "/git/owner/repo/commits/feature/keep%23x.atom"},
		{"a@b", "/git/owner/repo/commits/a@b.atom"},
		{"literal&amp;branch", "/git/owner/repo/commits/literal&amp%3Bbranch.atom"},
	}
	for _, test := range branches {
		if _, err := RecentCommits(context.Background(), "owner/repo", test.branch); err != nil {
			t.Fatal(err)
		}
		if got := paths[len(paths)-1]; got != test.path {
			t.Errorf("branch %q requested %q, want %q", test.branch, got, test.path)
		}
	}
}

func TestRecentCommitsHTTPFailuresAndSizeLimit(t *testing.T) {
	oldBase := githubAtomBase
	t.Cleanup(func() { githubAtomBase = oldBase })
	for _, test := range []struct {
		name string
		code int
		body []byte
		want int
	}{
		{name: "status", code: http.StatusTooManyRequests, want: http.StatusTooManyRequests},
		{name: "oversized", code: http.StatusOK, body: bytes.Repeat([]byte("x"), maxGitHubAtomBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.code != http.StatusOK {
					w.WriteHeader(test.code)
					return
				}
				_, _ = w.Write(test.body)
			}))
			defer server.Close()
			configureGitHub(&settings.Config{GitHubAtomBase: server.URL})
			_, err := RecentCommits(context.Background(), "owner/repo", "")
			if test.want != 0 {
				if got := httpStatusCode(err); got != test.want {
					t.Fatalf("httpStatusCode() = %d, want %d (err %v)", got, test.want, err)
				}
				return
			}
			var tooLarge *httpBodyTooLargeError
			if !errors.As(err, &tooLarge) {
				t.Fatalf("RecentCommits() error = %v, want body-too-large", err)
			}
		})
	}
}

func githubAtomBody(entries ...string) []byte {
	return []byte(`<feed xmlns="http://www.w3.org/2005/Atom">` + strings.Join(entries, "") + `</feed>`)
}

func githubAtomEntry(sha, title, author, titleType string) string {
	attr := ""
	if titleType != "" {
		attr = fmt.Sprintf(` type="%s"`, titleType)
	}
	name := ""
	if author != "" {
		name = `<author><name>` + author + `</name></author>`
	}
	return `<entry><id>tag:github.com,2008:Grit::Commit/` + sha + `</id><title` + attr + `>` + title + `</title>` + name + `</entry>`
}
