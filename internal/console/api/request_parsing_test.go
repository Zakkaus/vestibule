package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
)

const (
	requestParsingTestChatID int64 = -1009000000810
	statsTimezoneBoundHelper       = "VESTIBULE_STATS_TIMEZONE_BOUND_HELPER"
	statsTimezoneBoundName         = "VESTIBULE_STATS_TIMEZONE_BOUND_NAME"
)

func TestJSONRequestsRequireOneKnownBoundedDocument(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{BotToken: apiTestToken, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Authenticator: manager})
	initData := apiSignedInitData(now, 9)
	validBody := `{"init_data":` + strconv.Quote(initData) + `}`
	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
		code        string
	}{
		{"non-JSON media type", "text/plain", validBody, http.StatusUnsupportedMediaType, "json_required"},
		{"malformed document", "application/json", `{"init_data":`, http.StatusBadRequest, "invalid_json"},
		{"unknown field", "application/json", validBody[:len(validBody)-1] + `,"unrecognized":true}`, http.StatusBadRequest, "invalid_json"},
		{"second document", "application/json", validBody + `{}`, http.StatusBadRequest, "invalid_json"},
		{"body above the limit", "application/json", `{"init_data":"` + strings.Repeat("x", maxJSONBody) + `"}`, http.StatusBadRequest, "invalid_json"},
	}
	for _, testCase := range cases {
		response := postSessionJSON(server, testCase.contentType, testCase.body)
		if response.Code != testCase.status || decodeError(response) != testCase.code {
			t.Fatalf("%s let a rejected JSON request reach session creation: status=%d code=%q",
				testCase.name, response.Code, decodeError(response))
		}
	}
	accepted := postSessionJSON(server, "application/json; charset=utf-8", validBody)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("a valid JSON session request was refused: status=%d code=%q", accepted.Code, decodeError(accepted))
	}
}

func TestStatisticsRejectInvalidQueriesBeforeTheService(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{groups: []int64{requestParsingTestChatID}}
	server, cookies, _ := apiTestServer(t, checker, service, nil)
	path := "/api/chats/" + strconv.FormatInt(requestParsingTestChatID, 10) + "/stats?"
	validQuery := "from=2026-03-08&to=2026-03-10&timezone=UTC"
	cases := []struct {
		name  string
		query string
	}{
		{"missing from", "to=2026-03-10&timezone=UTC"},
		{"duplicate from", validQuery + "&from=2026-03-08"},
		{"malformed from", "from=2026-03-x&to=2026-03-10&timezone=UTC"},
		{"missing to", "from=2026-03-08&timezone=UTC"},
		{"duplicate to", validQuery + "&to=2026-03-10"},
		{"malformed to", "from=2026-03-08&to=2026-03-x&timezone=UTC"},
		{"missing timezone", "from=2026-03-08&to=2026-03-10"},
		{"duplicate timezone", validQuery + "&timezone=UTC"},
		{"unknown timezone", "from=2026-03-08&to=2026-03-10&timezone=Not_A_Zone"},
		{"timezone above the limit", "from=2026-03-08&to=2026-03-10&timezone=" + url.QueryEscape(strings.Repeat("x", 256))},
	}
	for _, testCase := range cases {
		response := getRequestWithoutPanic(t, server, cookies, path+testCase.query,
			"invalid statistics query ("+testCase.name+")")
		if response.Code != http.StatusBadRequest || decodeError(response) != "invalid_stats_query" || service.statsCalls != 0 {
			t.Fatalf("%s allowed an invalid statistics query to reach the service: status=%d code=%q calls=%d",
				testCase.name, response.Code, decodeError(response), service.statsCalls)
		}
	}
	accepted := getAuthenticatedPath(server, cookies, path+validQuery)
	if accepted.Code != http.StatusOK || service.statsCalls != 1 {
		t.Fatalf("a valid statistics query was refused: status=%d code=%q calls=%d",
			accepted.Code, decodeError(accepted), service.statsCalls)
	}
}

func TestStatisticsRejectTimezoneNamesAboveTheParserLimit(t *testing.T) {
	if os.Getenv(statsTimezoneBoundHelper) == "1" {
		timezone := os.Getenv(statsTimezoneBoundName)
		if _, err := time.LoadLocation(timezone); err != nil {
			t.Fatalf("test setup did not create a loadable timezone: %v", err)
		}
		request := httptest.NewRequest(http.MethodGet,
			"/?from=2026-03-08&to=2026-03-10&timezone="+url.QueryEscape(timezone), nil)
		if _, err := parseStatsRequest(request, requestParsingTestChatID); err == nil {
			t.Fatal("a timezone name above 255 bytes bypassed the parser bound")
		}
		return
	}
	zoneinfo, timezone := parserLimitTimezone(t)
	command := exec.Command(os.Args[0], "-test.run=^TestStatisticsRejectTimezoneNamesAboveTheParserLimit$")
	command.Env = append(os.Environ(), statsTimezoneBoundHelper+"=1", statsTimezoneBoundName+"="+timezone, "ZONEINFO="+zoneinfo)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("timezone-bound subprocess failed: %v\n%s", err, output)
	}
}

func TestChatRequestsRequireWorkingAuthenticationBeforeAuthorization(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{groups: []int64{requestParsingTestChatID}}
	server, cookies, _ := apiTestServer(t, checker, service, nil)
	path := "/api/chats/" + strconv.FormatInt(requestParsingTestChatID, 10) + "/queue"
	accepted := getAuthenticatedPath(server, cookies, path)
	if accepted.Code != http.StatusOK {
		t.Fatalf("a valid chat request was refused: status=%d code=%q", accepted.Code, decodeError(accepted))
	}
	checks := checker.counts()
	missing := getPath(server, path)
	if missing.Code != http.StatusUnauthorized || decodeError(missing) != "authentication_expired" || checker.counts() != checks {
		t.Fatalf("a chat request without a session reached authorization: status=%d code=%q checks=%+v",
			missing.Code, decodeError(missing), checker.counts())
	}
	unavailable := getRequestWithoutPanic(t, New(Config{Verification: service}), nil, path,
		"a chat request without authentication infrastructure")
	if unavailable.Code != http.StatusServiceUnavailable || decodeError(unavailable) != "authentication_unavailable" {
		t.Fatalf("a chat request ran without authentication infrastructure: status=%d code=%q",
			unavailable.Code, decodeError(unavailable))
	}
	missingConsole := getRequestWithoutPanic(t,
		New(Config{Authenticator: server.routes.Load().server.authenticator}), cookies, path,
		"a chat request without console configuration")
	if missingConsole.Code != http.StatusNotFound || decodeError(missingConsole) != "chat_not_found" || checker.counts() != checks {
		t.Fatalf("a chat request without console configuration reached authorization: status=%d code=%q checks=%+v",
			missingConsole.Code, decodeError(missingConsole), checker.counts())
	}
}

func postSessionJSON(server *Server, contentType, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func parserLimitTimezone(t *testing.T) (string, string) {
	t.Helper()
	data, err := os.ReadFile("/usr/share/zoneinfo/UTC")
	if err != nil {
		t.Fatal(err)
	}
	zoneinfo := t.TempDir()
	timezone := strings.Repeat("a/", 128) + "UTC"
	path := filepath.Join(zoneinfo, timezone)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return zoneinfo, timezone
}

func getRequestWithoutPanic(
	t *testing.T,
	server *Server,
	cookies []*http.Cookie,
	path string,
	harm string,
) (response *httptest.ResponseRecorder) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s caused the request handler to panic: %v", harm, recovered)
		}
	}()
	server.Handler().ServeHTTP(response, request)
	return response
}
