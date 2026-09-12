package settings

import (
	"strings"
	"testing"
)

func TestGitHubRepoEventSwitchesDefaultOff(t *testing.T) {
	for _, repo := range []GitHubRepo{{}, {Issues: boolPointer(false), Pulls: boolPointer(false)}} {
		if repo.IssuesOn() || repo.PullsOn() {
			t.Fatalf("event switches = issues %v, pulls %v; want both disabled", repo.IssuesOn(), repo.PullsOn())
		}
	}
	enabled := GitHubRepo{Issues: boolPointer(true), Pulls: boolPointer(true)}
	if !enabled.IssuesOn() || !enabled.PullsOn() {
		t.Fatalf("explicit event switches = issues %v, pulls %v; want both enabled", enabled.IssuesOn(), enabled.PullsOn())
	}
}

func boolPointer(value bool) *bool { return &value }

func TestLoadConfigRejectsDuplicateGitHubRepoWithDifferentEventSwitches(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, map[string]any{"feeds": []map[string]any{{
		"chat_id": -1009000002307,
		"github_repos": []map[string]any{
			{"repo": "owner/repo", "branch": "main", "issues": true},
			{"repo": "owner/repo", "branch": "main", "pulls": true},
		},
	}}}))
	if err == nil {
		t.Fatal("duplicate GitHub repository and branch with different switches was accepted")
	}
}

func TestLoadConfigNormalizesGitHubAPIBase(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
	}{
		{name: "empty", want: "https://api.github.com"},
		{name: "trailing slashes", raw: "https://api.example.com/root///", want: "https://api.example.com/root"},
		{name: "local HTTP", raw: "http://127.0.0.1:1234/", want: "http://127.0.0.1:1234"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := LoadConfig(writeConfig(t, map[string]any{"github_api_base": test.raw}))
			requireNoError(t, err)
			if config.GitHubAPIBase != test.want {
				t.Fatalf("github_api_base = %q, want %q", config.GitHubAPIBase, test.want)
			}
		})
	}
	for _, raw := range []string{"api.example.com", "ftp://api.example.com", "https:///api", "https://user:secret@api.example.com", "https://api.example.com?q=1", "https://api.example.com#fragment"} {
		t.Run("reject "+raw, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, map[string]any{"github_api_base": raw})); err == nil {
				t.Fatalf("invalid github_api_base %q was accepted", raw)
			}
		})
	}
}

func TestLoadConfigDoesNotLeakGitHubAPICredentials(t *testing.T) {
	const password = "api-password-that-must-not-leak"
	_, err := LoadConfig(writeConfig(t, map[string]any{"github_api_base": "https://user:" + password + "@api.example.com"}))
	if err == nil || strings.Contains(err.Error(), password) {
		t.Fatalf("credential-bearing github_api_base error = %v", err)
	}
}

func TestLoadConfigUsesFactoryGitHubAPIBase(t *testing.T) {
	old := embeddedDefaults.Resources.GitHubAPIBase
	t.Cleanup(func() { embeddedDefaults.Resources.GitHubAPIBase = old })
	embeddedDefaults.Resources.GitHubAPIBase = "https://api.factory.invalid/root///"
	config, err := LoadConfig(writeConfig(t, map[string]any{}))
	requireNoError(t, err)
	if config.GitHubAPIBase != "https://api.factory.invalid/root" {
		t.Fatalf("factory GitHub API base = %q", config.GitHubAPIBase)
	}
}

func TestLoadConfigRejectsInvalidGitHubReposAndDuplicatePairs(t *testing.T) {
	invalidRepos := []string{"", "noslash", "o/r@x", "../x", "./x", "o/..", "/x", "o/", "o//r"}
	for _, repo := range invalidRepos {
		t.Run("invalid repo "+repo, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, map[string]any{
				"feeds": []map[string]any{{
					"chat_id":      -1009000002302,
					"github_repos": []map[string]any{{"repo": repo}},
				}},
			}))
			if err == nil {
				t.Fatalf("invalid GitHub repository %q was accepted", repo)
			}
		})
	}

	_, err := LoadConfig(writeConfig(t, map[string]any{
		"feeds": []map[string]any{{
			"chat_id": -1009000002303,
			"github_repos": []map[string]any{
				{"repo": "owner/repo", "branch": "main"},
				{"repo": "owner/repo", "branch": "main"},
			},
		}},
	}))
	if err == nil {
		t.Fatal("duplicate GitHub repository and branch pair was accepted")
	}
}

func TestLoadConfigLeavesGitHubBranchValidationToUpstream(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, map[string]any{
		"feeds": []map[string]any{{
			"chat_id": -1009000002304,
			"github_repos": []map[string]any{
				{"repo": "owner/repo", "branch": "bad..ref"},
				{"repo": "other/repo", "branch": "a b"},
			},
		}},
	}))
	requireNoError(t, err)
	if got := config.Feeds[0].GitHubRepos[0].Branch; got != "bad..ref" {
		t.Fatalf("branch %q was changed while loading", got)
	}
	if got := config.Feeds[0].GitHubRepos[1].Branch; got != "a b" {
		t.Fatalf("branch %q was changed while loading", got)
	}
}

func TestLoadConfigNormalizesGitHubAtomBase(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{name: "empty", raw: "", want: "https://github.com"},
		{name: "default null", raw: nil, want: "https://github.com"},
		{name: "trailing slashes", raw: "https://git.example.com/git///", want: "https://git.example.com/git"},
		{name: "http local server", raw: "http://127.0.0.1:1234/", want: "http://127.0.0.1:1234"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := LoadConfig(writeConfig(t, map[string]any{"github_atom_base": test.raw}))
			requireNoError(t, err)
			if config.GitHubAtomBase != test.want {
				t.Fatalf("github_atom_base = %q, want %q", config.GitHubAtomBase, test.want)
			}
		})
	}

	invalid := []string{
		"git.example.com",
		"//git.example.com",
		"ftp://git.example.com",
		"https:///git",
		"https://user:password@git.example.com",
		"https://git.example.com/git?query=1",
		"https://git.example.com/git#fragment",
	}
	for _, raw := range invalid {
		t.Run("reject "+raw, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, map[string]any{"github_atom_base": raw}))
			if err == nil {
				t.Fatalf("invalid github_atom_base %q was accepted", raw)
			}
		})
	}
}

func TestLoadConfigDoesNotLeakGitHubAtomCredentials(t *testing.T) {
	const password = "password-that-must-not-leak"
	_, err := LoadConfig(writeConfig(t, map[string]any{
		"github_atom_base": "https://user:" + password + "@git.example.com",
	}))
	if err == nil {
		t.Fatal("github_atom_base with credentials was accepted")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("github_atom_base error leaked embedded credentials: %v", err)
	}
}

func TestGitHubRepoEmptyFormsPreserveLegacyFeedSemantics(t *testing.T) {
	for _, test := range []struct {
		name    string
		present bool
		value   any
	}{
		{name: "missing"},
		{name: "null", present: true},
		{name: "empty", present: true, value: []any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			feed := map[string]any{
				"chat_id":       -1009000002305,
				"bugs":          false,
				"news":          true,
				"bug_product":   "Gentoo",
				"bug_component": "Distribution",
				"silent_bugs":   true,
			}
			if test.present {
				feed["github_repos"] = test.value
			}
			config, err := LoadConfig(writeConfig(t, map[string]any{
				"feeds": []map[string]any{feed},
			}))
			requireNoError(t, err)
			if len(config.Feeds) != 1 {
				t.Fatalf("feeds = %+v, want one destination", config.Feeds)
			}
			got := config.Feeds[0]
			if got.BugsOn() || !got.NewsOn() || got.BugProduct != "Gentoo" ||
				got.BugComponent != "Distribution" || got.SilentBugs == nil || !*got.SilentBugs ||
				len(got.GitHubRepos) != 0 {
				t.Fatalf("legacy feed fields changed for github_repos=%s: %+v", test.name, got)
			}
		})
	}
}

func TestGitHubConfigPreservesLegacyBooleanForms(t *testing.T) {
	for _, field := range []string{"bugs", "news", "silent_bugs"} {
		for _, form := range []struct {
			name    string
			present bool
			value   any
		}{
			{name: "missing"},
			{name: "null", present: true},
			{name: "false", present: true, value: false},
			{name: "true", present: true, value: true},
		} {
			t.Run(field+"/"+form.name, func(t *testing.T) {
				feed := map[string]any{
					"chat_id":       -1009000002306,
					"bug_product":   "Gentoo",
					"bug_component": "Distribution",
					"github_repos":  []map[string]any{{"repo": "owner/repo"}},
				}
				if form.present {
					feed[field] = form.value
				}
				config, err := LoadConfig(writeConfig(t, map[string]any{
					"feeds": []map[string]any{feed},
				}))
				requireNoError(t, err)
				got := config.Feeds[0]
				if got.BugProduct != "Gentoo" || got.BugComponent != "Distribution" {
					t.Fatalf("legacy feed fields changed for %s=%s: %+v", field, form.name, got)
				}
				switch field {
				case "bugs":
					if got.BugsOn() != (form.name != "false") {
						t.Fatalf("BugsOn() = %v for %s, want %v", got.BugsOn(), form.name, form.name != "false")
					}
				case "news":
					if got.NewsOn() != (form.name != "false") {
						t.Fatalf("NewsOn() = %v for %s, want %v", got.NewsOn(), form.name, form.name != "false")
					}
				case "silent_bugs":
					want := form.name == "false" || form.name == "true"
					if (got.SilentBugs != nil) != want {
						t.Fatalf("SilentBugs pointer presence = %v for %s, want %v", got.SilentBugs != nil, form.name, want)
					}
					if want && *got.SilentBugs != (form.name == "true") {
						t.Fatalf("SilentBugs = %v for %s", *got.SilentBugs, form.name)
					}
				}
			})
		}
	}
}

func TestLoadConfigUsesFactoryGitHubAtomBase(t *testing.T) {
	old := embeddedDefaults.Resources.GitHubAtomBase
	t.Cleanup(func() { embeddedDefaults.Resources.GitHubAtomBase = old })
	embeddedDefaults.Resources.GitHubAtomBase = "https://git.factory.invalid/prefix///"
	config, err := LoadConfig(writeConfig(t, map[string]any{}))
	requireNoError(t, err)
	if config.GitHubAtomBase != "https://git.factory.invalid/prefix" {
		t.Fatalf("factory GitHub base = %q", config.GitHubAtomBase)
	}
	if config.Feeds != nil {
		t.Fatalf("absent factory feeds changed from null to an empty list: %#v", config.Feeds)
	}
}
