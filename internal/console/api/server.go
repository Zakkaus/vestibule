// Package api exposes the small HTTP surface consumed by the operator console.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const (
	maxJSONBody        = 16 << 10
	healthProbeTimeout = 2 * time.Second
)

// ConsoleService is the verification use case required by the HTTP adapter.
type ConsoleService interface {
	ConsoleGroups() []int64
	ConsoleQueue(context.Context, int64) ([]verification.ConsoleQueueEntry, error)
	SettleConsole(context.Context, verification.ConsoleSettlement) (verification.ConsoleQueueEntry, error)
	ConsoleAudit(context.Context, int64, int64) ([]verification.ConsoleAuditEntry, error)
	UndoConsoleAudit(context.Context, verification.ConsoleAuditUndo) (verification.ConsoleAuditEntry, error)
	ConsoleStats(context.Context, verification.ConsoleStatsRequest) (verification.ConsoleStatsReport, error)
}

// Config injects policy services into the HTTP adapter. The adapter owns no database access.
type Config struct {
	Authenticator        *auth.Manager
	Verification         ConsoleService
	Settings             SettingsService
	Rules                RulesService
	ProcessSettings      ProcessSettingsService
	Health               *status.Health
	Persistence          PersistenceService
	RollbackObservations RollbackObservationsService
	RollbackRejections   RollbackRejectionService
	Replacement          ReplacementService
	Release              ReleaseService
	Version              string
	ObserveOnly          bool
	Setup                SetupService
	SetupClaimed         func()
	// BotUsername is the Telegram handle this instance answers on. The screen a
	// visitor without a session lands on has to name the bot they should open,
	// and that name is different for every deployment.
	BotUsername string
}

// Server owns listener admission and HTTP handler draining separately for ordered shutdown.
type Server struct {
	authenticator        *auth.Manager
	verification         ConsoleService
	settings             SettingsService
	rules                RulesService
	processSettings      ProcessSettingsService
	health               *status.Health
	persistence          PersistenceService
	rollbackObservations RollbackObservationsService
	rollbackRejections   RollbackRejectionService
	replacement          ReplacementService
	release              ReleaseService
	version              string
	observeOnly          bool
	setup                SetupService
	setupClaimed         func()
	botUsername          string
	routes               atomic.Pointer[routeSet]
	mu                   sync.Mutex
	listener             net.Listener
	httpServer           *http.Server
}

func (s *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen console HTTP: %w", err)
	}
	httpServer := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	s.mu.Lock()
	if s.listener != nil || s.httpServer != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return errors.New("console HTTP server already started")
	}
	s.listener, s.httpServer = listener, httpServer
	s.mu.Unlock()
	go func() {
		_ = httpServer.Serve(listener)
	}()
	return nil
}

// StopAdmission closes the listener before Telegram update processing begins its shutdown.
func (s *Server) StopAdmission() error {
	if s.health != nil {
		s.health.SetTelegramReady(false)
	}
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	s.mu.Unlock()
	if listener == nil {
		return nil
	}
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("stop console HTTP admission: %w", err)
	}
	return nil
}

// Shutdown waits for in-flight HTTP handlers after admission has been removed.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.StopAdmission(); err != nil {
		return err
	}
	s.mu.Lock()
	httpServer := s.httpServer
	s.mu.Unlock()
	if httpServer == nil {
		return nil
	}
	if err := httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown console HTTP: %w", err)
	}
	return nil
}

func (s *Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/livez":
		s.live(writer)
	case request.Method == http.MethodGet && request.URL.Path == "/readyz":
		s.ready(writer, request)
	case s.setup != nil && strings.HasPrefix(request.URL.Path, "/setup/"):
		s.setupRoute(writer, request)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/enter/"):
		s.enter(writer, request)
	case strings.HasPrefix(request.URL.Path, "/api/"):
		s.apiRoute(writer, request)
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) apiRoute(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/instance":
		s.instance(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/session":
		s.currentSession(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/session":
		s.createSession(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/process/settings":
		s.readProcessSettings(writer, request)
	case strings.HasPrefix(request.URL.Path, "/api/status"):
		s.statusRoute(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/chats":
		s.chats(writer, request)
	case strings.HasPrefix(request.URL.Path, "/api/chats/"):
		s.chatRoute(writer, request)
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) statusRoute(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/status":
		s.readDiagnostics(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/status/release":
		s.readLatestRelease(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/status/upgrade":
		s.requestUpgrade(writer, request)
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) live(writer http.ResponseWriter) {
	if s.health != nil && s.health.Live() {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(writer, http.StatusServiceUnavailable, "not_live")
}

func (s *Server) ready(writer http.ResponseWriter, request *http.Request) {
	if s.health == nil {
		writeError(writer, http.StatusServiceUnavailable, "not_ready")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), healthProbeTimeout)
	defer cancel()
	if s.health.Ready(ctx) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(writer, http.StatusServiceUnavailable, "not_ready")
}

func (s *Server) currentSession(writer http.ResponseWriter, request *http.Request) {
	grant, ok := s.sessionGrant(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, newSessionResponse(grant))
}

func (s *Server) createSession(writer http.ResponseWriter, request *http.Request) {
	if s.authenticator == nil {
		writeError(writer, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	var input struct {
		InitData string `json:"init_data"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	grant, err := s.authenticator.IssueManagerSession(input.InitData)
	if err != nil {
		s.writeAuthError(writer, err)
		return
	}
	s.authenticator.SetCookies(writer, grant)
	writeJSON(writer, http.StatusCreated, newSessionResponse(grant))
}

func (s *Server) enter(writer http.ResponseWriter, request *http.Request) {
	if s.authenticator == nil || strings.Count(request.URL.Path, "/") != 2 {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	token := strings.TrimPrefix(request.URL.Path, "/enter/")
	if token == "" {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	grant, err := s.authenticator.RedeemOperatorLink(token)
	if err != nil {
		s.enterFailure(writer, request, err)
		return
	}
	s.authenticator.SetCookies(writer, grant)
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func (s *Server) enterFailure(writer http.ResponseWriter, request *http.Request, err error) {
	state := "no-session"
	switch {
	case errors.Is(err, auth.ErrOperatorLinkExpired):
		state = "expired"
	case errors.Is(err, auth.ErrOperatorLinkRedeemed):
		state = "redeemed"
	}
	http.Redirect(writer, request, "/?state="+state, http.StatusSeeOther)
}

func (s *Server) chats(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if s.verification == nil {
		writeError(writer, http.StatusServiceUnavailable, "verification_unavailable")
		return
	}
	candidates := s.verification.ConsoleGroups()
	allowed := s.authenticator.AccessibleChats(request.Context(), session, candidates)
	chats := make([]chatResponse, 0, len(allowed))
	for _, chatID := range allowed {
		chats = append(chats, chatResponse{ID: strconv.FormatInt(chatID, 10)})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"chats": chats})
}

func (s *Server) chatRoute(writer http.ResponseWriter, request *http.Request) {
	parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/api/chats/"), "/")
	if len(parts) < 2 {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	if parts[0] == "" || parts[1] == "" {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || chatID == 0 {
		writeError(writer, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	switch parts[1] {
	case "queue":
		s.queueRoute(writer, request, chatID, parts[2:])
	case "audit":
		s.auditRoute(writer, request, chatID, parts[2:])
	case "stats":
		s.statsRoute(writer, request, chatID, parts[2:])
	case "settings":
		s.settingsRoute(writer, request, chatID, parts[2:])
	case "rules":
		s.rulesRoute(writer, request, chatID, parts[2:])
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) queueRoute(
	writer http.ResponseWriter,
	request *http.Request,
	chatID int64,
	rest []string,
) {
	switch request.Method {
	case http.MethodGet:
		if len(rest) == 0 {
			s.queue(writer, request, chatID)
			return
		}
	case http.MethodPost:
		if len(rest) == 1 && rest[0] != "" {
			s.settle(writer, request, chatID, rest[0])
			return
		}
	}
	writeError(writer, http.StatusNotFound, "not_found")
}

func (s *Server) auditRoute(
	writer http.ResponseWriter,
	request *http.Request,
	chatID int64,
	rest []string,
) {
	switch request.Method {
	case http.MethodGet:
		if len(rest) == 0 {
			s.audit(writer, request, chatID)
			return
		}
	case http.MethodPost:
		if len(rest) == 2 && rest[0] != "" && rest[1] == "undo" {
			s.undoAudit(writer, request, chatID, rest[0])
			return
		}
	}
	writeError(writer, http.StatusNotFound, "not_found")
}

func (s *Server) queue(writer http.ResponseWriter, request *http.Request, chatID int64) {
	if _, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess); !ok {
		return
	}
	entries, err := s.verification.ConsoleQueue(request.Context(), chatID)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "queue_unavailable")
		return
	}
	items := make([]queueResponse, 0, len(entries))
	for _, entry := range entries {
		items = append(items, queueView(entry, time.Now()))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) settle(writer http.ResponseWriter, request *http.Request, chatID int64, challengeID string) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	var input settlementRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	entry, err := s.verification.SettleConsole(request.Context(), verification.ConsoleSettlement{
		ID: challengeID, GroupID: chatID, ActorID: session.Principal.TelegramID,
		Expected: verification.ChallengeState(input.Expected.State), Target: verification.ChallengeState(input.Result.State),
		Reason: input.Result.Reason,
	})
	if err != nil {
		s.writeSettlementError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, queueView(entry, time.Now()))
}

func (s *Server) authorizedSession(writer http.ResponseWriter, request *http.Request, chatID int64, intent auth.AccessIntent) (auth.Session, bool) {
	session, ok := s.session(writer, request)
	if !ok {
		return auth.Session{}, false
	}
	if s.verification == nil || !containsChat(s.verification.ConsoleGroups(), chatID) {
		writeError(writer, http.StatusNotFound, "chat_not_found")
		return auth.Session{}, false
	}
	if err := s.authenticator.AuthorizeChat(request.Context(), session, chatID, intent); err != nil {
		s.writeAuthorizationError(writer, err)
		return auth.Session{}, false
	}
	return session, true
}

func (s *Server) session(writer http.ResponseWriter, request *http.Request) (auth.Session, bool) {
	grant, ok := s.sessionGrant(writer, request)
	if !ok {
		return auth.Session{}, false
	}
	return grant.Session, true
}

func (s *Server) sessionGrant(writer http.ResponseWriter, request *http.Request) (auth.Grant, bool) {
	if s.authenticator == nil {
		writeError(writer, http.StatusServiceUnavailable, "authentication_unavailable")
		return auth.Grant{}, false
	}
	grant, err := s.authenticator.GrantFromRequest(request)
	if err != nil {
		if errors.Is(err, auth.ErrSessionMissing) || errors.Is(err, auth.ErrSessionExpired) {
			s.authenticator.ClearCookies(writer)
		}
		s.writeAuthError(writer, err)
		return auth.Grant{}, false
	}
	return grant, true
}

func (s *Server) writeAuthError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInitDataReplayed):
		writeError(writer, http.StatusConflict, "init_data_replayed")
	case errors.Is(err, auth.ErrSessionMissing), errors.Is(err, auth.ErrSessionExpired), errors.Is(err, auth.ErrInitDataExpired):
		writeError(writer, http.StatusUnauthorized, "authentication_expired")
	default:
		writeError(writer, http.StatusUnauthorized, "authentication_invalid")
	}
}

func (s *Server) writeAuthorizationError(writer http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrAccessDenied) {
		writeError(writer, http.StatusForbidden, "chat_access_denied")
		return
	}
	writeError(writer, http.StatusServiceUnavailable, "chat_access_unavailable")
}

func (s *Server) writeSettlementError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, verification.ErrConsoleChallengeConflict):
		writeError(writer, http.StatusConflict, "challenge_conflict")
	case errors.Is(err, verification.ErrConsoleSettlementInvalid):
		writeError(writer, http.StatusBadRequest, "invalid_settlement")
	case errors.Is(err, verification.ErrConsoleTargetProtected):
		writeError(writer, http.StatusConflict, "target_protected")
	case errors.Is(err, verification.ErrConsoleTargetUnavailable):
		writeError(writer, http.StatusServiceUnavailable, "target_unavailable")
	default:
		writeError(writer, http.StatusServiceUnavailable, "settlement_unavailable")
	}
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(writer, http.StatusUnsupportedMediaType, "json_required")
		return false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxJSONBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_json")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func containsChat(chats []int64, wanted int64) bool {
	for _, chatID := range chats {
		if chatID == wanted {
			return true
		}
	}
	return false
}

type sessionResponse struct {
	Subject struct {
		TelegramID string    `json:"telegram_id"`
		Role       auth.Role `json:"role"`
	} `json:"subject"`
	ExpiresAt time.Time `json:"expires_at"`
	CSRFToken string    `json:"csrf_token"`
}

func newSessionResponse(grant auth.Grant) sessionResponse {
	response := sessionResponse{ExpiresAt: grant.Session.ExpiresAt, CSRFToken: grant.CSRFToken}
	response.Subject.TelegramID = strconv.FormatInt(grant.Session.Principal.TelegramID, 10)
	response.Subject.Role = grant.Session.Principal.Role
	return response
}

type chatResponse struct {
	ID string `json:"id"`
}

type settlementRequest struct {
	Expected resultInput `json:"expected"`
	Result   resultInput `json:"result"`
}

type resultInput struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type resultResponse struct {
	State  verification.ChallengeState `json:"state"`
	Reason *string                     `json:"reason"`
}

type queueResponse struct {
	ID               string         `json:"id"`
	User             string         `json:"user"`
	GroupKey         string         `json:"group_key"`
	Result           resultResponse `json:"result"`
	OccurredAt       *time.Time     `json:"occurred_at"`
	ExpiresAt        time.Time      `json:"expires_at"`
	RemainingSeconds *int64         `json:"remaining_seconds"`
}

func queueView(entry verification.ConsoleQueueEntry, now time.Time) queueResponse {
	user := entry.Name
	if user == "" {
		user = strconv.FormatInt(entry.UserID, 10)
	}
	response := queueResponse{ID: entry.ID, User: user, GroupKey: strconv.FormatInt(entry.GroupID, 10),
		Result: resultResponse{State: entry.State}, ExpiresAt: entry.ExpiresAt}
	if entry.State == verification.ChallengeDeclined {
		reason := entry.Reason
		response.Result.Reason = &reason
	}
	if !entry.CreatedAt.IsZero() {
		occurredAt := entry.CreatedAt
		response.OccurredAt = &occurredAt
	}
	if entry.State == verification.ChallengePending {
		remaining := int64(entry.ExpiresAt.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		response.RemainingSeconds = &remaining
	}
	return response
}

type errorResponse struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func writeError(writer http.ResponseWriter, statusCode int, code string) {
	response := errorResponse{}
	response.Error.Code = code
	writeJSON(writer, statusCode, response)
}

func writeJSON(writer http.ResponseWriter, statusCode int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(value)
}
