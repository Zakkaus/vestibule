package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

const statsTestChatID int64 = -1009000000701

func TestStatsRejectsUnauthorizedChatRead(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: false}
	service := &apiTestQueueService{groups: []int64{statsTestChatID}}
	server, cookies, _ := apiTestServer(t, checker, service, nil)

	response := getAuthenticatedPath(server, cookies,
		"/api/chats/-1009000000701/stats?from=2026-03-08&to=2026-03-10&timezone=America%2FNew_York")
	counts := checker.counts()
	if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" {
		t.Fatalf("unauthorized statistics response = %d %q", response.Code, decodeError(response))
	}
	if service.statsCalls != 0 || counts.cachedCalls != 1 || counts.freshCalls != 0 {
		t.Fatalf("unauthorized statistics calls = service:%d cached:%d fresh:%d",
			service.statsCalls, counts.cachedCalls, counts.freshCalls)
	}
}

func TestStatsResponseUsesDirectShapeAndCallerTimezone(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{
		groups: []int64{statsTestChatID},
		statsReport: verification.ConsoleStatsReport{
			From: "2026-03-08", To: "2026-03-10", Timezone: "America/New_York",
			Summary: verification.ConsoleStatsOutcome{Challenges: 2, Approved: 1, Expired: 1, PassRate: 0.5},
			Trend: []verification.ConsoleStatsDay{{
				Date: "2026-03-08",
				Outcome: verification.ConsoleStatsOutcome{
					Challenges: 2, Approved: 1, Expired: 1, PassRate: 0.5,
				},
			}},
			Interceptions: []verification.ConsoleStatsInterception{{Kind: "future-proof", Count: 3}},
		},
	}
	server, cookies, _ := apiTestServer(t, checker, service, nil)
	response := getAuthenticatedPath(server, cookies,
		"/api/chats/-1009000000701/stats?from=2026-03-08&to=2026-03-10&timezone=America%2FNew_York")

	var payload statsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &topLevel); err != nil {
		t.Fatal(err)
	}
	wantPayload := statsResponse{
		Range:   statsRangeResponse{From: "2026-03-08", To: "2026-03-10", Timezone: "America/New_York"},
		Summary: statsOutcomeResponse{Challenges: 2, Approved: 1, Expired: 1, PassRate: 0.5},
		Trend: []statsDayResponse{{
			Date: "2026-03-08",
			statsOutcomeResponse: statsOutcomeResponse{
				Challenges: 2, Approved: 1, Expired: 1, PassRate: 0.5,
			},
		}},
		Interceptions: []statsInterceptionResponse{{Kind: "future-proof", Count: 3}},
	}
	if response.Code != http.StatusOK {
		t.Fatalf("statistics status = %d, want 200", response.Code)
	}
	if !reflect.DeepEqual(payload, wantPayload) {
		t.Fatalf("statistics response = %+v, want %+v", payload, wantPayload)
	}
	if len(topLevel["range"]) == 0 || len(topLevel["stats"]) != 0 {
		t.Fatalf("statistics top-level keys = %v", topLevel)
	}
	if service.statsCalls != 1 {
		t.Fatalf("statistics service calls = %d, want 1", service.statsCalls)
	}
	if service.lastStats.GroupID != statsTestChatID {
		t.Fatalf("statistics chat = %d, want %d", service.lastStats.GroupID, statsTestChatID)
	}
	if service.lastStats.Location.String() != "America/New_York" {
		t.Fatalf("statistics timezone = %s", service.lastStats.Location)
	}
	if service.lastStats.From.Format("2006-01-02") != "2026-03-08" {
		t.Fatalf("statistics from = %s", service.lastStats.From)
	}
	if service.lastStats.To.Format("2006-01-02") != "2026-03-10" {
		t.Fatalf("statistics to = %s", service.lastStats.To)
	}
	if counts := checker.counts(); counts != (apiTestAdminCounts{cachedCalls: 1, telegramQueries: 1}) {
		t.Fatalf("statistics authorization calls = %+v", counts)
	}
}
