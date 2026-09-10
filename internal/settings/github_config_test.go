package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

type githubRepoProcessSettingsDTO struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type githubFeedProcessSettingsDTO struct {
	ChatID      int64                          `json:"chat_id"`
	GitHubRepos []githubRepoProcessSettingsDTO `json:"github_repos"`
}

func TestLoadConfigRetainsGitHubReposInProcessSettings(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, map[string]any{
		"feeds": []map[string]any{{
			"chat_id":       -1009000002301,
			"bugs":          false,
			"news":          false,
			"bug_product":   "Gentoo",
			"bug_component": "Distribution",
			"silent_bugs":   true,
			"github_repos": []map[string]any{
				{"repo": "gentoo-zh/overlay"},
				{"repo": "Zakkaus/vestibule", "branch": "feature/keep#x"},
				{"repo": "example/repo", "branch": "a@b"},
			},
		}},
	}))
	requireNoError(t, err)

	data, err := json.Marshal(config.ProcessSettings().Feeds().Value)
	requireNoError(t, err)
	var got []githubFeedProcessSettingsDTO
	requireNoError(t, json.Unmarshal(data, &got))
	if len(got) != 1 || got[0].ChatID != -1009000002301 || len(got[0].GitHubRepos) != 3 {
		t.Fatalf("process settings feeds = %+v, want one destination with three GitHub repositories", got)
	}
	want := []githubRepoProcessSettingsDTO{
		{Repo: "gentoo-zh/overlay"},
		{Repo: "Zakkaus/vestibule", Branch: "feature/keep#x"},
		{Repo: "example/repo", Branch: "a@b"},
	}
	for i := range want {
		if got[0].GitHubRepos[i] != want[i] {
			t.Fatalf("process settings github_repos[%d] = %+v, want %+v", i, got[0].GitHubRepos[i], want[i])
		}
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

func TestGitHubFeedProcessSettingsSources(t *testing.T) {
	factory, err := LoadConfig(writeConfig(t, map[string]any{}))
	requireNoError(t, err)
	if got := factory.ProcessSettings().Feeds().Source; got != SourceFactory {
		t.Fatalf("factory feed source = %q, want %q", got, SourceFactory)
	}

	userFile, err := LoadConfig(writeConfig(t, map[string]any{
		"feeds": []any{},
	}))
	requireNoError(t, err)
	if got := userFile.ProcessSettings().Feeds().Source; got != SourceUserFile {
		t.Fatalf("user-file feed source = %q, want %q", got, SourceUserFile)
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
