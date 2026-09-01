package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

const processSettingsPath = "/api/process/settings"

type apiTestProcessSettingsService struct {
	view  settings.ProcessView
	calls int
}

func (s *apiTestProcessSettingsService) ProcessSettings() settings.ProcessView {
	s.calls++
	return s.view
}

func TestGetProcessSettingsReturnsValuesAndSources(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		source settings.Source
		file   bool
	}{
		{name: "factory defaults", config: map[string]any{}, source: settings.SourceFactory},
		{
			name: "user file",
			config: map[string]any{
				"feeds": []map[string]any{{
					"chat_id": -1009000000201, "lang": "en", "interval_seconds": 600,
					"bugs": false, "news": true, "silent_bugs": true,
				}},
				"news_url":       "https://example.invalid/news-items.xml",
				"overlays":       []map[string]any{{"name": "gentoo", "repo": "gentoo/overlay", "branch": "stable"}},
				"stats_timezone": "Asia/Shanghai",
			},
			source: settings.SourceUserFile,
			file:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := loadProcessSettingsConfig(t, test.config)
			service := &apiTestProcessSettingsService{view: config.ProcessSettings()}
			server, cookies := processSettingsTestServer(t, auth.RoleOperator, service)
			response := processSettingsRequest(server, cookies, http.MethodGet)
			body := decodeProcessSettings(t, response)

			assertProcessSettingsResponse(t, response, service.calls, body, test.source)
			if test.file {
				assertUserFileProcessSettings(t, body)
				return
			}
			assertFactoryProcessSettings(t, body)
		})
	}
}

func assertProcessSettingsResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	calls int,
	body processSettingsResponse,
	source settings.Source,
) {
	t.Helper()
	wantSource := source.String()
	if response.Code != http.StatusOK || calls != 1 || body.Feeds.Source != wantSource ||
		body.NewsURL.Source != wantSource || body.Overlays.Source != wantSource || body.StatsTimezone.Source != wantSource {
		t.Fatalf("status=%d calls=%d sources=%q/%q/%q/%q, want 200, 1, %q",
			response.Code, calls, body.Feeds.Source, body.NewsURL.Source, body.Overlays.Source,
			body.StatsTimezone.Source, wantSource)
	}
}

func assertFactoryProcessSettings(t *testing.T, body processSettingsResponse) {
	t.Helper()
	if len(body.Feeds.Value) != 0 || body.NewsURL.Value != "" || len(body.Overlays.Value) != 0 ||
		body.StatsTimezone.Value != "" {
		t.Fatalf("factory response = %+v", body)
	}
}

func assertUserFileProcessSettings(t *testing.T, body processSettingsResponse) {
	t.Helper()
	if len(body.Feeds.Value) != 1 {
		t.Fatalf("feeds = %+v, want one configured feed", body.Feeds.Value)
	}
	assertConfiguredFeed(t, body.Feeds.Value[0])
	if body.NewsURL.Value != "https://example.invalid/news-items.xml" || len(body.Overlays.Value) != 1 ||
		body.StatsTimezone.Value != "Asia/Shanghai" {
		t.Fatalf("user-file response = %+v", body)
	}
	if overlay := body.Overlays.Value[0]; overlay.Repo != "gentoo/overlay" || overlay.Branch != "stable" {
		t.Fatalf("overlay = %+v, want gentoo/overlay stable", overlay)
	}
}

func assertConfiguredFeed(t *testing.T, feed settings.FeedConfig) {
	t.Helper()
	if feed.ChatID != -1009000000201 || feed.Lang != "en" || feed.IntervalSeconds != 600 ||
		feed.Bugs == nil || *feed.Bugs || feed.News == nil || !*feed.News ||
		feed.SilentBugs == nil || !*feed.SilentBugs {
		t.Fatalf("feed = %+v, want configured feed", feed)
	}
}

func TestGetProcessSettingsRejectsManager(t *testing.T) {
	config := loadProcessSettingsConfig(t, map[string]any{})
	service := &apiTestProcessSettingsService{view: config.ProcessSettings()}
	server, cookies := processSettingsTestServer(t, auth.RoleManager, service)
	response := processSettingsRequest(server, cookies, http.MethodGet)

	if response.Code != http.StatusForbidden || decodeError(response) != "process_access_denied" || service.calls != 0 {
		t.Fatalf("status=%d code=%s calls=%d, want 403, process_access_denied, 0",
			response.Code, decodeError(response), service.calls)
	}
}

func TestProcessSettingsHasNoWriteRoute(t *testing.T) {
	config := loadProcessSettingsConfig(t, map[string]any{})
	service := &apiTestProcessSettingsService{view: config.ProcessSettings()}
	server, cookies := processSettingsTestServer(t, auth.RoleOperator, service)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := processSettingsRequest(server, cookies, method)
		if response.Code != http.StatusNotFound || decodeError(response) != "not_found" {
			t.Fatalf("%s status=%d code=%s, want 404, not_found", method, response.Code, decodeError(response))
		}
	}
	if service.calls != 0 {
		t.Fatalf("write routes called the process settings service %d times", service.calls)
	}
}

func loadProcessSettingsConfig(t *testing.T, value map[string]any) *settings.Config {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := settings.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func processSettingsTestServer(
	t *testing.T,
	role auth.Role,
	service ProcessSettingsService,
) (*Server, []*http.Cookie) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken,
		Now:      func() time.Time { return now },
		OperatorAllowed: func(telegramID int64) bool {
			return telegramID == 9
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var grant auth.Grant
	switch role {
	case auth.RoleManager:
		grant, err = manager.IssueManagerSession(apiSignedInitData(now, 9))
	case auth.RoleOperator:
		link, _, issueErr := manager.IssueOperatorLink(9)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		grant, err = manager.RedeemOperatorLink(link)
	default:
		t.Fatalf("unsupported role %q", role)
	}
	if err != nil {
		t.Fatal(err)
	}
	cookies := httptest.NewRecorder()
	manager.SetCookies(cookies, grant)
	return New(Config{Authenticator: manager, ProcessSettings: service}), cookies.Result().Cookies()
}

func processSettingsRequest(server *Server, cookies []*http.Cookie, method string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, processSettingsPath, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func decodeProcessSettings(t *testing.T, response *httptest.ResponseRecorder) processSettingsResponse {
	t.Helper()
	var body processSettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
