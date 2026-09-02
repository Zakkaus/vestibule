package status

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type releaseRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip releaseRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestReleaseCheckerLooksUpOnlyOnDemandAndAssessesRollback(t *testing.T) {
	calls := 0
	checker := releaseTestChecker(t, &calls)
	if calls != 0 {
		t.Fatalf("constructor performed %d requests, want none", calls)
	}

	info, err := checker.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("Latest performed %d requests, want release plus manifest", calls)
	}
	assertReleaseInfo(t, info)
}

func releaseTestChecker(t *testing.T, calls *int) *ReleaseChecker {
	t.Helper()
	checker := NewReleaseChecker("v5.1.0", "github-token")
	checker.client = &http.Client{Transport: releaseRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		*calls++
		switch request.URL.Host + request.URL.Path {
		case "api.github.com/repos/Zakkaus/vestibule/releases/latest":
			if request.Header.Get("Authorization") != "Bearer github-token" {
				t.Fatalf("API authorization = %q, want bearer token", request.Header.Get("Authorization"))
			}
			if request.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
				t.Fatalf("API version = %q", request.Header.Get("X-GitHub-Api-Version"))
			}
			return releaseResponse(http.StatusOK, `{
				"tag_name":"v5.2.0",
				"body":"Safer replacement\n\nSee details.",
				"published_at":"2026-09-01T00:00:00Z",
				"assets":[{"name":"vestibule-schema-manifest"}]
			}`), nil
		case "github.com/Zakkaus/vestibule/releases/download/v5.2.0/vestibule-schema-manifest":
			if request.Header.Get("Authorization") != "" {
				t.Fatal("release asset request forwarded the API token")
			}
			return releaseResponse(http.StatusOK,
				"target_schema_version=3\nminimum_rollback_schema_version=3\n"), nil
		default:
			t.Fatalf("unexpected release request %s", request.URL)
			return nil, nil
		}
	})}
	return checker
}

func assertReleaseInfo(t *testing.T, info ReleaseInfo) {
	t.Helper()
	if info.Version != "v5.2.0" {
		t.Fatalf("release version = %q", info.Version)
	}
	if !info.UpdateAvailable {
		t.Fatal("release was not marked available")
	}
	if info.URL != "https://github.com/Zakkaus/vestibule/releases/tag/v5.2.0" {
		t.Fatalf("release URL = %q", info.URL)
	}
	if info.Notes != "Safer replacement\n\nSee details." {
		t.Fatalf("release notes = %q", info.Notes)
	}
	if info.Rollback == nil {
		t.Fatal("rollback assessment is missing")
	}
	if info.Rollback.Available {
		t.Fatalf("rollback assessment = %#v, want blocked", info.Rollback)
	}
	if info.Rollback.TargetSchemaVersion != 3 {
		t.Fatalf("rollback target = v%d, want v3", info.Rollback.TargetSchemaVersion)
	}
	if info.Rollback.RetainedSchemaVersion != 2 {
		t.Fatalf("retained schema = v%d, want v2", info.Rollback.RetainedSchemaVersion)
	}
	if info.Rollback.MinimumRollbackSchemaVersion != 3 {
		t.Fatalf("rollback floor = v%d, want v3", info.Rollback.MinimumRollbackSchemaVersion)
	}
}

func TestReleaseCheckerRejectsIncompleteMetadata(t *testing.T) {
	checker := NewReleaseChecker("v5.1.0", "")
	checker.client = &http.Client{Transport: releaseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return releaseResponse(http.StatusOK, `{
			"tag_name":"v5.2.0",
			"body":"notes",
			"published_at":"2026-09-01T00:00:00Z",
			"assets":[]
		}`), nil
	})}
	if _, err := checker.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no schema manifest") {
		t.Fatalf("Latest error = %v, want missing manifest refusal", err)
	}
}

func TestNewerReleaseUsesSemanticPrecedence(t *testing.T) {
	for name, test := range map[string]struct {
		current string
		latest  string
		want    bool
	}{
		"minor update":       {current: "v5.1.9", latest: "v5.2.0", want: true},
		"numeric precedence": {current: "v5.10.0", latest: "v5.2.0", want: false},
		"same release":       {current: "v5.2.0", latest: "v5.2.0", want: false},
		"prerelease":         {current: "v5.2.0-rc.2", latest: "v5.2.0", want: true},
		"development build":  {current: "dev", latest: "v5.2.0", want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := newerRelease(test.current, test.latest); got != test.want {
				t.Fatalf("newerRelease(%q, %q) = %t, want %t", test.current, test.latest, got, test.want)
			}
		})
	}
}

func releaseResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
