package lookup

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"
)

const lsManPage = `LS(1)                            User Commands                           LS(1)

NAME
       ls - list directory contents

SYNOPSIS
       ls [OPTION]... [FILE]...

DESCRIPTION
       List information about the FILEs.
`

// The two sections worth quoting are read out of the plain-text page, and the description that
// follows them is not.
func TestManPageParsing(t *testing.T) {
	title, synopsis := parseManPage(lsManPage)
	if title != "ls - list directory contents" {
		t.Errorf("title = %q", title)
	}
	if synopsis != "ls [OPTION]... [FILE]..." {
		t.Errorf("synopsis = %q", synopsis)
	}
	if strings.Contains(synopsis, "List information") {
		t.Error("the synopsis must stop at the next section")
	}
}

// A page without the sections still resolves; the caller falls back to the page name.
func TestManPageWithoutSections(t *testing.T) {
	title, synopsis := parseManPage("SOMEPAGE(9)\n\n       free text only\n")
	if title != "" || synopsis != "" {
		t.Errorf("title = %q synopsis = %q, want both empty", title, synopsis)
	}
}

// Only a page name reaches the network; anything that could steer the request is refused before
// a request is made.
func TestManNameAccepts(t *testing.T) {
	// Real page names carry dots, so a trailing number that is not a section is just part of
	// the name; it simply will not be found.
	for _, name := range []string{"ls", "fstab.5", "ssh_config", "git-rebase", "a.out", "open.2",
		"systemd.service", "tmpfiles.d", "ls.99"} {
		if !manNameRe.MatchString(name) {
			t.Errorf("%q is a legitimate page name", name)
		}
	}
	for _, name := range []string{"", "../etc/passwd", "ls;rm", "a b", "http://x", "ls\nrm", strings.Repeat("x", 90)} {
		if manNameRe.MatchString(name) {
			t.Errorf("%q must not reach the network", name)
		}
	}
}

// A section that has no page is not an outage; only an unreachable site is.
func TestManMissingPageIsNotAnOutage(t *testing.T) {
	withStatusFixture(t, 404, func() {
		_, found, failed := fetchManPage(context.Background(), "nosuchpage", "1")
		if found {
			t.Error("a 404 means the page does not exist")
		}
		if failed {
			t.Error("a site that answered 404 is reachable; do not report an outage")
		}
	})
}

// printfManPages carries the two pages a bare "printf" is ambiguous between: the shell utility in
// section 1 and the C library function in section 3.
var printfManPages = map[string]string{
	"1": "PRINTF(1)\n\nNAME\n       printf - format and print data\n\nSYNOPSIS\n       printf FORMAT [ARGUMENT]...\n",
	"3": "PRINTF(3)\n\nNAME\n       printf - formatted output conversion\n\nSYNOPSIS\n       int printf(const char *restrict format, ...);\n",
}

// withManSectionFixture serves a page for each section named in pages and 404s every other one.
func withManSectionFixture(t *testing.T, pages map[string]string, requested *[]string) {
	t.Helper()
	old := httpClient
	httpClient = &http.Client{Transport: fixtureRoundTrip(func(req *http.Request) (*http.Response, error) {
		id := strings.TrimSuffix(path.Base(req.URL.Path), ".txt")
		section := id[strings.LastIndexByte(id, '.')+1:]
		*requested = append(*requested, section)
		body, ok := pages[section]
		status := http.StatusOK
		if !ok {
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	t.Cleanup(func() { httpClient = old })
}

// When the asker names a section, that section is the answer. Probing the default order instead
// answers "printf.3" with printf(1) — the shell builtin rather than the C function they asked
// about — and the reply gives no sign that it is a different page.
func TestANamedSectionIsTheOnlyOneProbed(t *testing.T) {
	var requested []string
	withManSectionFixture(t, printfManPages, &requested)

	page, found, failed := fetchManPage(context.Background(), "printf", "3")
	if !found || failed {
		t.Fatalf("fetchManPage(printf, 3) = found %v failed %v, want a page", found, failed)
	}
	if page.title != "printf - formatted output conversion" {
		t.Errorf("title = %q, want the section 3 page: the asker named a section and got another one", page.title)
	}
	if !strings.HasSuffix(page.url, "/printf.3") {
		t.Errorf("url = %q, want the section 3 page", page.url)
	}
	if len(requested) != 1 || requested[0] != "3" {
		t.Errorf("probed sections = %v, want only [3]", requested)
	}

	// Positive control: a bare name still walks the default order and lands on section 1.
	requested = nil
	page, found, failed = fetchManPage(context.Background(), "printf", "")
	if !found || failed || !strings.HasSuffix(page.url, "/printf.1") {
		t.Fatalf("fetchManPage(printf, \"\") = %+v found %v failed %v, want the section 1 page", page, found, failed)
	}
}
