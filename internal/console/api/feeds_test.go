package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestGetFeedsReturnsEffectiveValuesAndSources(t *testing.T) {
	server, cookies, _, service, _ := apiSettingsTestServer(t, true)
	group, _ := service.store.Settings(apiSettingsGroupID)
	lang := "zh-Hant"
	news := true
	repos := []settings.GitHubRepo{{Repo: "owner/repo", Branch: "main"}}
	next := group.Overrides()
	next.Feed = &settings.FeedOverride{Lang: &lang, News: &news, GitHubRepos: &repos}
	if _, err := service.store.Update(apiSettingsGroupID, group.Revision(), next); err != nil {
		t.Fatal(err)
	}

	response := feedsRequest(server, cookies, "", http.MethodGet, nil)
	body := decodeFeeds(t, response)
	if response.Code != http.StatusOK || body.Revision != 1 ||
		body.Feed.Lang.Value != lang || body.Feed.Lang.Source != settings.SourceChatOverride.String() ||
		!body.Feed.News.Value || body.Feed.Bugs.Value ||
		body.Feed.IntervalSeconds.Value != 300 ||
		body.Feed.IntervalSeconds.Source != settings.SourceFactory.String() ||
		len(body.GitHubRepos.Value) != 1 || body.GitHubRepos.Value[0].Repo != "owner/repo" ||
		body.GitHubRepos.Value[0].Branch != "main" || body.GitHubRepos.Value[0].Issues || body.GitHubRepos.Value[0].Pulls {
		t.Fatalf("GET feeds status=%d body=%+v", response.Code, body)
	}
}

func TestPutFeedsReplacesOnlyFeedOverride(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	group, _ := service.store.Settings(apiSettingsGroupID)
	enabled := false
	oldNews := true
	next := group.Overrides()
	next.Enabled = &enabled
	next.Feed = &settings.FeedOverride{News: &oldNews}
	if _, err := service.store.Update(apiSettingsGroupID, group.Revision(), next); err != nil {
		t.Fatal(err)
	}

	response := feedsRequest(server, cookies, csrf, http.MethodPut, strings.NewReader(
		`{"expected_revision":1,"lang":"","interval_seconds":300,"bugs":true,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[]}`))
	body := decodeFeeds(t, response)
	group, _ = service.store.Settings(apiSettingsGroupID)
	if response.Code != http.StatusOK || body.Revision != 2 || !body.Feed.Bugs.Value || body.Feed.News.Value ||
		body.GitHubRepos.Source != settings.SourceChatOverride.String() || body.GitHubRepos.Value == nil ||
		group.Enabled().Value || group.Enabled().Source != settings.SourceChatOverride || service.updateCalls != 1 {
		t.Fatalf("PUT feeds status=%d body=%+v enabled=%+v updates=%d",
			response.Code, body, group.Enabled(), service.updateCalls)
	}
}

func TestPutFeedsRejectsConflictAndCSRF(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	group, _ := service.store.Settings(apiSettingsGroupID)
	next := group.Overrides()
	spoiler := false
	next.NameSpoiler = &spoiler
	if _, err := service.store.Update(apiSettingsGroupID, group.Revision(), next); err != nil {
		t.Fatal(err)
	}

	conflict := feedsRequest(server, cookies, csrf, http.MethodPut, strings.NewReader(`{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":true,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[]}`))
	if conflict.Code != http.StatusConflict || decodeError(conflict) != "settings_conflict" {
		t.Fatalf("conflict status=%d code=%q", conflict.Code, decodeError(conflict))
	}
	csrfFailure := feedsRequest(server, cookies, "", http.MethodPut, strings.NewReader(`{"expected_revision":1,"lang":"","interval_seconds":300,"bugs":false,"news":true,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[]}`))
	if csrfFailure.Code != http.StatusForbidden || decodeError(csrfFailure) != "csrf_invalid" {
		t.Fatalf("CSRF status=%d code=%q", csrfFailure.Code, decodeError(csrfFailure))
	}
	group, _ = service.store.Settings(apiSettingsGroupID)
	if group.Revision() != 1 || group.Feed().News.Value {
		t.Fatalf("failed writes changed group: revision=%d news=%v", group.Revision(), group.Feed().News.Value)
	}
}

func TestFeedsRejectsUnauthorizedChat(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, false)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		var body *strings.Reader
		if method == http.MethodPut {
			body = strings.NewReader(`{"expected_revision":0}`)
		} else {
			body = strings.NewReader("")
		}
		response := feedsRequest(server, cookies, csrf, method, body)
		if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" {
			t.Fatalf("%s status=%d code=%q", method, response.Code, decodeError(response))
		}
	}
	if service.updateCalls != 0 {
		t.Fatalf("unauthorized requests made %d updates", service.updateCalls)
	}
}

func TestPutFeedsValidatesCompleteRequestSchema(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		field     string
		fieldCode string
	}{
		{name: "missing revision", body: `{}`, field: "expected_revision", fieldCode: "invalid_revision"},
		{name: "null revision", body: `{"expected_revision":null}`, field: "expected_revision", fieldCode: "invalid_revision"},
		{name: "unsafe revision", body: `{"expected_revision":9007199254740992}`, field: "expected_revision", fieldCode: "invalid_revision"},
		{name: "fractional revision", body: `{"expected_revision":0.5}`, field: "expected_revision", fieldCode: "invalid_revision"},
		{name: "missing language", body: `{"expected_revision":0,"interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[]}`, field: "lang", fieldCode: "required_field"},
		{name: "null repositories", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":null}`, field: "github_repos", fieldCode: "required_field"},
		{name: "short interval", body: `{"expected_revision":0,"lang":"","interval_seconds":59,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[]}`, field: "interval_seconds", fieldCode: "invalid_interval"},
		{name: "missing issue switch", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[{"repo":"owner/repo","pulls":false}]}`, field: "github_repos[0].issues", fieldCode: "required_field"},
		{name: "null pull switch", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[{"repo":"owner/repo","issues":false,"pulls":null}]}`, field: "github_repos[0].pulls", fieldCode: "required_field"},
		{name: "invalid repository", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[{"repo":"owner","issues":false,"pulls":false}]}`, field: "github_repos[0].repo", fieldCode: "invalid_repository"},
		{name: "dot repository segment", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[{"repo":"owner/..","issues":false,"pulls":false}]}`, field: "github_repos[0].repo", fieldCode: "invalid_repository"},
		{name: "duplicate repository", body: `{"expected_revision":0,"lang":"","interval_seconds":300,"bugs":false,"news":false,"bug_product":"","bug_component":"","silent_bugs":false,"github_repos":[{"repo":"owner/repo","branch":"main","issues":false,"pulls":false},{"repo":"owner/repo","branch":"main","issues":true,"pulls":true}]}`, field: "github_repos[1]", fieldCode: "duplicate_repository"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
			response := feedsRequest(server, cookies, csrf, http.MethodPut, strings.NewReader(test.body))
			var body feedsInvalidResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusBadRequest || body.Error.Code != "invalid_request" || len(body.Fields) != 1 ||
				body.Fields[0].Name != test.field || body.Fields[0].Code != test.fieldCode || service.updateCalls > 1 {
				t.Fatalf("status=%d body=%+v updates=%d", response.Code, body, service.updateCalls)
			}
		})
	}

	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	unknown := feedsRequest(server, cookies, csrf, http.MethodPut, strings.NewReader(`{"expected_revision":0,"unknown":true}`))
	if unknown.Code != http.StatusBadRequest || decodeError(unknown) != "invalid_json" || service.updateCalls != 0 {
		t.Fatalf("unknown field status=%d code=%q updates=%d", unknown.Code, decodeError(unknown), service.updateCalls)
	}
}

func feedsRequest(
	server *Server,
	cookies []*http.Cookie,
	csrf string,
	method string,
	body *strings.Reader,
) *httptest.ResponseRecorder {
	if body == nil {
		body = strings.NewReader("")
	}
	path := "/api/chats/" + strconv.FormatInt(apiSettingsGroupID, 10) + "/feeds"
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func decodeFeeds(t *testing.T, response *httptest.ResponseRecorder) feedsResponse {
	t.Helper()
	var body feedsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
