package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/i18n"
	storedrules "github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/Zakkaus/vestibule/migrations"
)

func TestQueueResponsesPreserveEveryVisibleChallengeDetail(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	created := now.Add(-time.Minute)
	reason := "manual refusal"
	zero := int64(0)
	tests := []struct {
		name  string
		entry verification.ConsoleQueueEntry
		want  queueResponse
	}{
		{
			name: "declined unnamed applicant",
			entry: verification.ConsoleQueueEntry{
				ID: "challenge-declined", GroupID: -1009000000201, UserID: 41,
				State: verification.ChallengeDeclined, Reason: reason, CreatedAt: created, ExpiresAt: now.Add(time.Minute),
			},
			want: queueResponse{
				ID: "challenge-declined", User: "41", GroupKey: "-1009000000201",
				Result:     resultResponse{State: verification.ChallengeDeclined, Reason: &reason},
				OccurredAt: &created, ExpiresAt: now.Add(time.Minute),
			},
		},
		{
			name: "expired pending applicant",
			entry: verification.ConsoleQueueEntry{
				ID: "challenge-pending", GroupID: -1009000000202, UserID: 42, Name: "Applicant",
				State: verification.ChallengePending, ExpiresAt: now.Add(-time.Second),
			},
			want: queueResponse{
				ID: "challenge-pending", User: "Applicant", GroupKey: "-1009000000202",
				Result: resultResponse{State: verification.ChallengePending}, ExpiresAt: now.Add(-time.Second),
				RemainingSeconds: &zero,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := queueView(test.entry, now); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("queue response hid or changed a visible challenge detail: got=%+v want=%+v", got, test.want)
			}
		})
	}
}

func TestAuditResponsesPreserveFallbackIdentityAndDeclineReason(t *testing.T) {
	settledAt := time.Date(2026, time.September, 3, 11, 0, 0, 0, time.UTC)
	reason := "spam"
	entry := verification.ConsoleAuditEntry{
		ID: "audit-declined", GroupID: -1009000000211, UserID: 43,
		State: verification.ChallengeDeclined, Reason: reason, SettledAt: settledAt,
		UndoState: verification.ConsoleUndoUnavailable,
	}
	want := auditResponse{
		ID: "audit-declined", Kind: auditKindChallenge, User: "43", GroupKey: "-1009000000211",
		Result:    resultResponse{State: verification.ChallengeDeclined, Reason: &reason},
		SettledAt: settledAt, UndoState: verification.ConsoleUndoUnavailable,
	}
	if got := auditView(entry); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit response hid who was declined or why: got=%+v want=%+v", got, want)
	}
}

func TestRuleResponsesPreserveEnabledDefinitions(t *testing.T) {
	definition := json.RawMessage(`{"kind":"invite-link","value":"allowed"}`)
	record := storedrules.Record{
		ID: "rule-1", ChatID: -1009000000221, Collection: "allowlist", Ordinal: 3,
		Enabled: true, Definition: definition,
	}
	want := ruleResponse{
		ID: "rule-1", Collection: "allowlist", Ordinal: 3, Enabled: true, Definition: definition,
	}
	if got := ruleView(record); !reflect.DeepEqual(got, want) {
		t.Fatalf("rule response lost whether a rule was enabled or what it contained: got=%+v want=%+v", got, want)
	}
}

func TestStatisticsResponsesPreserveDeclinesAndBans(t *testing.T) {
	report := verification.ConsoleStatsReport{
		From: "2026-09-01", To: "2026-09-03", Timezone: "UTC",
		Summary: verification.ConsoleStatsOutcome{Challenges: 9, Approved: 2, Declined: 3, Banned: 4, Expired: 1, PassRate: 0.25},
	}
	got := statsView(report)
	want := statsOutcomeResponse{Challenges: 9, Approved: 2, Declined: 3, Banned: 4, Expired: 1, PassRate: 0.25}
	if got.Summary != want {
		t.Fatalf("statistics response erased declined or banned outcomes: got=%+v want=%+v", got.Summary, want)
	}
}

func TestReleaseResponsesPreserveEveryPublishedAndRollbackField(t *testing.T) {
	publishedAt := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	info := status.ReleaseInfo{
		Version: "v5.3.0", URL: "https://github.com/Zakkaus/vestibule/releases/tag/v5.3.0",
		Notes: "Visible release notes", PublishedAt: publishedAt, UpdateAvailable: true,
		Rollback: &status.ReleaseRollback{
			Available: true, Reason: migrations.RollbackCompatible,
			TargetSchemaVersion: 4, RetainedSchemaVersion: 3, MinimumRollbackSchemaVersion: 2,
		},
	}
	want := releaseResponse{
		Version: info.Version, URL: info.URL, Notes: info.Notes, PublishedAt: publishedAt, UpdateAvailable: true,
		Rollback: &releaseRollbackResponse{
			Available: true, Reason: string(migrations.RollbackCompatible),
			TargetSchemaVersion: 4, RetainedSchemaVersion: 3, MinimumRollbackSchemaVersion: 2,
		},
	}
	if got := releaseView(info); !reflect.DeepEqual(got, want) {
		t.Fatalf("release response omitted published or rollback information: got=%+v want=%+v", got, want)
	}
}

func TestSessionResponsesPreserveExpiryAndIdentifiers(t *testing.T) {
	expiresAt := time.Date(2026, time.September, 3, 20, 0, 0, 0, time.UTC)
	grant := auth.Grant{
		Session:   auth.Session{Principal: auth.Principal{TelegramID: 41, Role: auth.RoleManager}, ExpiresAt: expiresAt},
		CSRFToken: "csrf-value",
	}
	session := newSessionResponse(grant)
	if session.ExpiresAt != expiresAt || session.Subject.TelegramID != "41" ||
		session.Subject.Role != auth.RoleManager || session.CSRFToken != "csrf-value" {
		t.Fatalf("session response changed its subject, expiry, or CSRF grant: %+v", session)
	}
}

func TestChatResponsesIncludeRegisteredTitlesAndKeepMissingTitlesOptional(t *testing.T) {
	const (
		namedChatID   int64 = -1009000000232
		unnamedChatID int64 = -1009000000233
		title               = "<Moderators> 🧪"
	)
	baseline, err := settings.LoadBaseline("", &settings.Config{GroupIDs: []int64{unnamedChatID}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	registration := store.Registrations()
	registration.OwnerID = 9
	registration.RegisteredGroups = []settings.RegisteredGroup{{
		ID: namedChatID, RegisteredBy: 9, Title: title,
	}}
	if _, err = store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}

	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{groups: []int64{namedChatID, unnamedChatID}}
	server, cookies, _ := apiTestServer(t, checker, service, nil, &apiTestSettingsService{store: store})
	response := getAuthenticatedPath(server, cookies, "/api/chats")
	var body struct {
		Chats []chatResponse `json:"chats"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []chatResponse{
		{ID: strconv.FormatInt(namedChatID, 10), Title: title},
		{ID: strconv.FormatInt(unnamedChatID, 10)},
	}
	if response.Code != http.StatusOK || !reflect.DeepEqual(body.Chats, want) {
		t.Fatalf("chat list response lost its registered title or ID fallback: status=%d chats=%+v want=%+v",
			response.Code, body.Chats, want)
	}
}

func TestDiagnosticsResponsesPreserveEveryRollbackMeasurement(t *testing.T) {
	observedAt := time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC)
	first := observedAt.Add(-11 * time.Minute)
	last := observedAt.Add(-time.Minute)
	recovered := observedAt.Add(-30 * time.Second)
	challengeStreak := status.ProblemStreakSnapshot{
		FirstProblemAt: &first, LastProblemAt: &last, LastRecoveredAt: &recovered,
		ProblemEvents: 5, ObservedFor: 10*time.Minute + time.Second, ExceedsWindow: true,
	}
	consoleStreak := status.ProblemStreakSnapshot{
		FirstProblemAt: &first, LastProblemAt: &last, LastRecoveredAt: &recovered,
		ProblemEvents: 7, ObservedFor: 12*time.Minute + 2*time.Second, ExceedsWindow: true,
	}
	databaseStart := observedAt.Add(-10 * time.Minute)
	reason := "wrong_answer"
	snapshot := status.RollbackSnapshot{
		ObservedAt: observedAt,
		ChallengeDelivery: status.ChallengeDeliverySnapshot{
			ProblemStreakSnapshot: challengeStreak, FailedDeliveries: 4, DuplicateDeliveries: 3,
		},
		ConsoleAccess: consoleStreak,
		DatabaseWrites: status.DatabaseWriteSnapshot{
			WindowStart: databaseStart, WindowEnd: observedAt, TotalWrites: 200, FailedWrites: 5,
			FailureRatePercent: 2.5, ExceedsOnePercent: true,
		},
	}
	threshold := int64(status.RollbackObservationWindow / time.Second)
	wantStreak := func(source status.ProblemStreakSnapshot) diagnosticsProblemStreakResponse {
		return diagnosticsProblemStreakResponse{
			ThresholdSeconds: threshold, FirstProblemAt: source.FirstProblemAt, LastProblemAt: source.LastProblemAt,
			LastRecoveredAt: source.LastRecoveredAt, ProblemEvents: source.ProblemEvents,
			ProblemSpanSeconds: source.ObservedFor.Seconds(), ExceedsThreshold: source.ExceedsWindow,
		}
	}
	want := diagnosticsRollbackResponse{
		Rejections: diagnosticsRejectionsResponse{
			SourceAvailable: true, HumanReviewRequired: true,
			WindowSeconds: int64(verification.RollbackRejectionWindow / time.Second),
			WindowStart:   observedAt.Add(-verification.RollbackRejectionWindow), WindowEnd: observedAt,
			ByReason: []diagnosticsRejectionReasonResponse{{Reason: &reason, Count: 2}},
		},
		ChallengeDelivery: diagnosticsChallengeDeliveryResponse{
			Streak: wantStreak(challengeStreak), FailedDeliveries: 4, DuplicateDeliveries: 3,
		},
		ConsoleAccess: diagnosticsConsoleAccessResponse{
			Streak: wantStreak(consoleStreak), UnavailableAttempts: 7,
		},
		DatabaseWrites: diagnosticsDatabaseWriteResponse{
			Scope: retryStoreWriteScope, WindowSeconds: threshold, WindowStart: databaseStart, WindowEnd: observedAt,
			TotalWrites: 200, FailedWrites: 5, FailureRatePercent: 2.5, ExceedsOnePercent: true,
		},
	}
	got := diagnosticsRollbackView(snapshot, []verification.RejectionReasonCount{{Reason: &reason, Count: 2}}, true)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diagnostics response erased a rollback measurement: got=%+v want=%+v", got, want)
	}

	view := diagnosticsView("v5.3.0", false, status.HealthSnapshot{}, false,
		settings.PersistenceStatus{Configured: true, Durable: true, Writable: true}, status.ReplacementStatus{}, got)
	if view.Persistence != (diagnosticsPersistenceResponse{Configured: true, Durable: true, Writable: true}) {
		t.Fatalf("diagnostics response erased persistence state: %+v", view.Persistence)
	}
}

func TestSetupPageCarriesLocalizedClaimStateAndSecurityHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/setup/redacted", nil)
	request.Header.Set("Accept-Language", "en")
	failure := "The claim was refused safely."
	formResponse := httptest.NewRecorder()
	renderSetup(formResponse, request, http.StatusUnprocessableEntity, failure, SetupResult{})
	for name, text := range map[string]string{
		"language":      `lang="en"`,
		"title":         i18n.Messages.Bot.Setup.Title.For(i18n.LangEN),
		"description":   i18n.Messages.Bot.Setup.Description.For(i18n.LangEN),
		"token label":   i18n.Messages.Bot.Setup.TokenLabel.For(i18n.LangEN),
		"submit action": i18n.Messages.Bot.Setup.Submit.For(i18n.LangEN),
		"failure":       failure,
	} {
		if !strings.Contains(formResponse.Body.String(), text) {
			t.Fatalf("setup form omitted %s %q from the rendered claim response", name, text)
		}
	}
	// The policy admits exactly the stylesheet this binary embedded, by hash, so
	// the assertion is on that hash rather than on a copied literal that would
	// have to be re-typed on every edit to page.css.
	wantPolicy := "default-src 'none'; base-uri 'none'; form-action 'self'; style-src " + styleHash(pageStyle)
	if formResponse.Code != http.StatusUnprocessableEntity ||
		formResponse.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		formResponse.Header().Get("Content-Security-Policy") != wantPolicy ||
		formResponse.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("setup response lost its status, media type, or browser protections: status=%d headers=%v",
			formResponse.Code, formResponse.Header())
	}

	bindingURL := "https://t.me/test_bot?start=owner_test&next=1"
	claimedResponse := httptest.NewRecorder()
	renderSetup(claimedResponse, request, http.StatusOK, "", SetupResult{Claimed: true, BindingURL: bindingURL})
	for name, text := range map[string]string{
		"claimed state":  i18n.Messages.Bot.Setup.Claimed.For(i18n.LangEN),
		"binding label":  i18n.Messages.Bot.Setup.Binding.For(i18n.LangEN),
		"binding URL":    html.EscapeString(bindingURL),
		"binding action": i18n.Messages.Bot.Setup.BindingAction.For(i18n.LangEN),
	} {
		if !strings.Contains(claimedResponse.Body.String(), text) {
			t.Fatalf("claimed setup page omitted %s %q", name, text)
		}
	}
}

func TestJSONResponsesAlwaysCarryPrivateTypedHeaders(t *testing.T) {
	health := status.NewHealth(func(_ context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	server := New(Config{Health: health})
	for name, response := range map[string]*httptest.ResponseRecorder{
		"success": getPath(server, "/livez"),
		"error":   getPath(server, "/not-a-route"),
	} {
		if response.Header().Get("Cache-Control") != "no-store" ||
			response.Header().Get("X-Content-Type-Options") != "nosniff" ||
			response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s JSON response became cacheable or sniffable: headers=%v", name, response.Header())
		}
	}
}

func TestStatsServiceErrorsKeepTheirPublicMeaning(t *testing.T) {
	const chatID int64 = -1009000000242
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid range", verification.ErrConsoleStatsInvalid, http.StatusBadRequest, "invalid_stats_query"},
		{"unavailable aggregation", errors.New("private database detail"), http.StatusServiceUnavailable, "stats_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			checker := &apiTestAdminChecker{allowed: true}
			service := &apiTestQueueService{groups: []int64{chatID}, statsErr: test.err}
			server, cookies, _ := apiTestServer(t, checker, service, nil)
			response := getAuthenticatedPath(server, cookies,
				"/api/chats/-1009000000242/stats?from=2026-09-01&to=2026-09-03&timezone=UTC")
			if response.Code != test.status || decodeError(response) != test.code || service.statsCalls != 1 ||
				strings.Contains(response.Body.String(), "private") {
				t.Fatalf("statistics error changed public meaning: status=%d code=%q calls=%d body=%q",
					response.Code, decodeError(response), service.statsCalls, response.Body.String())
			}
		})
	}
}

func TestUnexpectedUpgradeFailuresStayUnavailableAndPrivate(t *testing.T) {
	replacement := &apiTestReplacementService{snapshot: status.ReplacementStatus{UnitAvailable: true}}
	server, cookies := diagnosticsTestServer(
		t, auth.RoleOperator, diagnosticsHealth(), &apiTestPersistenceService{}, replacement,
	)
	csrf := replacementCSRFToken(t, server, cookies)
	if response := postUpgrade(server, cookies, csrf, `{"version":"v5.4.0"}`); response.Code != http.StatusAccepted {
		t.Fatalf("valid upgrade request was refused: status=%d code=%q", response.Code, decodeError(response))
	}
	replacement.request = errors.New("private host detail")
	response := postUpgrade(server, cookies, csrf, `{"version":"v5.4.1"}`)
	if response.Code != http.StatusServiceUnavailable || decodeError(response) != "upgrade_unavailable" ||
		len(replacement.requests) != 1 || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("unexpected host failure changed public meaning or retried: status=%d code=%q requests=%v body=%q",
			response.Code, decodeError(response), replacement.requests, response.Body.String())
	}
}

func TestAPIErrorResponsesKeepStablePublicCodes(t *testing.T) {
	server := &Server{}
	tests := []struct {
		name   string
		render func(http.ResponseWriter, error)
		err    error
		status int
		code   string
	}{
		{"invalid authentication", server.writeAuthError, errors.New("private auth detail"), http.StatusUnauthorized, "authentication_invalid"},
		{"settlement conflict", server.writeSettlementError, fmt.Errorf("wrapped: %w", verification.ErrConsoleChallengeConflict), http.StatusConflict, "challenge_conflict"},
		{"invalid settlement", server.writeSettlementError, fmt.Errorf("wrapped: %w", verification.ErrConsoleSettlementInvalid), http.StatusBadRequest, "invalid_settlement"},
		{"protected target", server.writeSettlementError, fmt.Errorf("wrapped: %w", verification.ErrConsoleTargetProtected), http.StatusConflict, "target_protected"},
		{"unavailable target", server.writeSettlementError, fmt.Errorf("wrapped: %w", verification.ErrConsoleTargetUnavailable), http.StatusServiceUnavailable, "target_unavailable"},
		{"unknown settlement failure", server.writeSettlementError, errors.New("private settlement detail"), http.StatusServiceUnavailable, "settlement_unavailable"},
		{"invalid audit", writeAuditError, fmt.Errorf("wrapped: %w", verification.ErrConsoleAuditInvalid), http.StatusBadRequest, "invalid_audit"},
		{"missing audit", writeAuditError, fmt.Errorf("wrapped: %w", verification.ErrConsoleAuditNotFound), http.StatusNotFound, "audit_not_found"},
		{"audit cannot be undone", writeAuditError, fmt.Errorf("wrapped: %w", verification.ErrConsoleAuditNotUndoable), http.StatusConflict, "audit_not_undoable"},
		{"audit conflict", writeAuditError, fmt.Errorf("wrapped: %w", verification.ErrConsoleAuditConflict), http.StatusConflict, "audit_conflict"},
		{"unknown audit failure", writeAuditError, errors.New("private audit detail"), http.StatusServiceUnavailable, "audit_unavailable"},
		{"settings conflict", writeSettingsError, fmt.Errorf("wrapped: %w", settings.ErrSettingsConflict), http.StatusConflict, "settings_conflict"},
		{"unknown settings group", writeSettingsError, fmt.Errorf("wrapped: %w", settings.ErrUnknownGroup), http.StatusNotFound, "chat_not_found"},
		{"unknown settings failure", writeSettingsError, errors.New("private settings detail"), http.StatusServiceUnavailable, "settings_unavailable"},
		{"invalid rule", writeRulesError, fmt.Errorf("wrapped: %w", storedrules.ErrRuleInvalid), http.StatusBadRequest, "invalid_rule"},
		{"rule conflict", writeRulesError, fmt.Errorf("wrapped: %w", storedrules.ErrRuleConflict), http.StatusConflict, "rule_conflict"},
		{"missing rule", writeRulesError, fmt.Errorf("wrapped: %w", storedrules.ErrRuleNotFound), http.StatusNotFound, "rule_not_found"},
		{"unknown rule chat", writeRulesError, fmt.Errorf("wrapped: %w", storedrules.ErrRuleChatNotFound), http.StatusNotFound, "chat_not_found"},
		{"unknown rule failure", writeRulesError, errors.New("private rule detail"), http.StatusServiceUnavailable, "rules_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.render(response, test.err)
			if response.Code != test.status || decodeError(response) != test.code ||
				strings.Contains(response.Body.String(), "private") {
				t.Fatalf("public API error changed from status=%d code=%q to status=%d code=%q body=%q",
					test.status, test.code, response.Code, decodeError(response), response.Body.String())
			}
		})
	}
}
