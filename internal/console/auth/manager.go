package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookieName          = "vestibule_console_session"
	defaultSessionTTL          = 8 * time.Hour
	defaultLinkTTL             = 10 * time.Minute
	defaultMaxEntries          = 4096
	accessibleChatsConcurrency = 8
)

var (
	ErrInitDataReplayed     = errors.New("telegram init data was already used")
	ErrSessionMissing       = errors.New("console session is missing")
	ErrSessionExpired       = errors.New("console session expired")
	ErrCSRFInvalid          = errors.New("CSRF token is invalid")
	ErrOperatorNotAllowed   = errors.New("telegram account is not an operator")
	ErrOperatorLinkExpired  = errors.New("operator link expired")
	ErrOperatorLinkRedeemed = errors.New("operator link already redeemed")
	ErrOperatorLinkInvalid  = errors.New("operator link is invalid")
	ErrSessionCapacity      = errors.New("console credential capacity reached")
	ErrAccessDenied         = errors.New("telegram account is not a group administrator")
	ErrAccessUnavailable    = errors.New("cannot verify Telegram group access")
)

// Role describes the entry flow that established a console session.
type Role string

const (
	RoleManager  Role = "manager"
	RoleOperator Role = "operator"
)

// Principal is the identity placed in an authenticated session.
type Principal struct {
	TelegramID int64
	Role       Role
}

// Session is the non-secret portion of an authenticated browser session.
type Session struct {
	Principal Principal
	ExpiresAt time.Time
	token     string
	csrf      string
}

// Grant contains a session plus its CSRF value, which is safe for the browser application to read.
type Grant struct {
	Session   Session
	CSRFToken string
}

// AccessIntent makes the authorization freshness requirement explicit at each call site.
type AccessIntent uint8

const (
	// WriteAccess always rechecks Telegram so a revoked administrator cannot mutate state.
	WriteAccess AccessIntent = iota
	// ReadAccess may reuse a recent positive Telegram result.
	ReadAccess
)

// AdminChecker provides cached-positive reads and cache-bypassing writes.
type AdminChecker interface {
	CachedAdmin(context.Context, int64, int64) (bool, error)
	FreshAdmin(context.Context, int64, int64) (bool, error)
}

// AccessAvailabilityObserver records whether group-access checks are usable.
type AccessAvailabilityObserver interface {
	RecordConsoleAccessUnavailable()
	RecordConsoleAccessVerified()
}

// Config makes all credential lifetimes and the bounded in-memory cache explicit.
type Config struct {
	BotToken        string
	InitDataTTL     time.Duration
	SessionTTL      time.Duration
	OperatorLinkTTL time.Duration
	MaxEntries      int
	Now             func() time.Time
	OperatorAllowed func(int64) bool
	AdminChecker    AdminChecker
	AccessObserver  AccessAvailabilityObserver
}

type sessionRecord struct {
	session Session
}

type linkRecord struct {
	principal Principal
	expiresAt time.Time
}

type linkTombstone struct {
	err       error
	expiresAt time.Time
}

// Manager holds only short-lived credentials. Group permissions remain in Telegram.
type Manager struct {
	botToken        string
	initDataTTL     time.Duration
	sessionTTL      time.Duration
	operatorLinkTTL time.Duration
	maxEntries      int
	now             func() time.Time
	operatorAllowed func(int64) bool
	adminChecker    AdminChecker
	accessObserver  AccessAvailabilityObserver

	mu             sync.Mutex
	sessions       map[string]sessionRecord
	sessionByUser  map[int64]string
	replayedHashes map[string]time.Time
	links          map[string]linkRecord
	linkHistory    map[string]linkTombstone
}

// New creates a bounded credential manager. It never persists identity or permissions.
func New(config Config) (*Manager, error) {
	if config.BotToken == "" {
		return nil, ErrInvalidInitData
	}
	return &Manager{
		botToken: config.BotToken, initDataTTL: durationAtMost(config.InitDataTTL, defaultSessionTTL),
		sessionTTL:      durationAtMost(config.SessionTTL, defaultSessionTTL),
		operatorLinkTTL: durationAtMost(config.OperatorLinkTTL, defaultLinkTTL),
		maxEntries:      positiveOr(config.MaxEntries, defaultMaxEntries), now: functionOr(config.Now, time.Now),
		operatorAllowed: config.OperatorAllowed, adminChecker: config.AdminChecker, accessObserver: config.AccessObserver,
		sessions: make(map[string]sessionRecord), sessionByUser: make(map[int64]string),
		replayedHashes: make(map[string]time.Time), links: make(map[string]linkRecord),
		linkHistory: make(map[string]linkTombstone),
	}, nil
}

func durationAtMost(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		value = fallback
	}
	if value > defaultSessionTTL {
		return defaultSessionTTL
	}
	return value
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func functionOr(value func() time.Time, fallback func() time.Time) func() time.Time {
	if value != nil {
		return value
	}
	return fallback
}

// IssueManagerSession exchanges one valid, unused Telegram Mini App initData payload for a session.
func (m *Manager) IssueManagerSession(raw string) (Grant, error) {
	now := m.now()
	identity, err := VerifyInitData(raw, m.botToken, now, m.initDataTTL)
	if err != nil {
		return Grant{}, err
	}
	values, err := parseInitData(raw)
	if err != nil {
		return Grant{}, err
	}
	return m.issueManager(identity, values.Get("hash"), now)
}

func (m *Manager) issueManager(identity TelegramIdentity, hash string, now time.Time) (Grant, error) {
	token, csrf, err := credentialPair()
	if err != nil {
		return Grant{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	if _, replayed := m.replayedHashes[hash]; replayed {
		return Grant{}, ErrInitDataReplayed
	}
	if !m.hasSessionCapacityLocked(identity.ID) || len(m.replayedHashes) >= m.maxEntries {
		return Grant{}, ErrSessionCapacity
	}
	expiresAt := now.Add(m.sessionTTL)
	m.replayedHashes[hash] = identity.AuthDate.Add(m.initDataTTL)
	return m.createSessionLocked(Principal{TelegramID: identity.ID, Role: RoleManager}, token, csrf, expiresAt), nil
}

// IssueOperatorLink makes a single-use browser entry token for the configured owner only.
func (m *Manager) IssueOperatorLink(telegramID int64) (string, time.Time, error) {
	if m.operatorAllowed == nil || !m.operatorAllowed(telegramID) {
		return "", time.Time{}, ErrOperatorNotAllowed
	}
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	if len(m.links) >= m.maxEntries {
		return "", time.Time{}, ErrSessionCapacity
	}
	expiresAt := now.Add(m.operatorLinkTTL)
	m.links[token] = linkRecord{principal: Principal{TelegramID: telegramID, Role: RoleOperator}, expiresAt: expiresAt}
	return token, expiresAt, nil
}

// RedeemOperatorLink consumes a one-time link and creates the browser session it grants.
func (m *Manager) RedeemOperatorLink(token string) (Grant, error) {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	link, found := m.links[token]
	if !found {
		if history, known := m.linkHistory[token]; known {
			return Grant{}, history.err
		}
		return Grant{}, ErrOperatorLinkInvalid
	}
	delete(m.links, token)
	if !link.expiresAt.After(now) {
		m.rememberLinkLocked(token, ErrOperatorLinkExpired, now)
		return Grant{}, ErrOperatorLinkExpired
	}
	if !m.hasSessionCapacityLocked(link.principal.TelegramID) {
		return Grant{}, ErrSessionCapacity
	}
	credential, csrf, err := credentialPair()
	if err != nil {
		return Grant{}, err
	}
	m.rememberLinkLocked(token, ErrOperatorLinkRedeemed, now)
	return m.createSessionLocked(link.principal, credential, csrf, now.Add(m.sessionTTL)), nil
}

func (m *Manager) hasSessionCapacityLocked(telegramID int64) bool {
	_, replacing := m.sessionByUser[telegramID]
	return replacing || len(m.sessions) < m.maxEntries
}

func (m *Manager) createSessionLocked(principal Principal, token, csrf string, expiresAt time.Time) Grant {
	if oldToken := m.sessionByUser[principal.TelegramID]; oldToken != "" {
		delete(m.sessions, oldToken)
	}
	session := Session{Principal: principal, ExpiresAt: expiresAt, token: token, csrf: csrf}
	m.sessions[token] = sessionRecord{session: session}
	m.sessionByUser[principal.TelegramID] = token
	return Grant{Session: publicSession(session), CSRFToken: csrf}
}

func publicSession(session Session) Session {
	return Session{Principal: session.Principal, ExpiresAt: session.ExpiresAt, token: session.token, csrf: session.csrf}
}

// GrantFromRequest validates the HttpOnly session cookie and returns its existing browser grant.
func (m *Manager) GrantFromRequest(request *http.Request) (Grant, error) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return Grant{}, ErrSessionMissing
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	record, found := m.sessions[cookie.Value]
	if !found {
		return Grant{}, ErrSessionExpired
	}
	if !record.session.ExpiresAt.After(now) {
		m.removeSessionLocked(cookie.Value, record.session.Principal.TelegramID)
		return Grant{}, ErrSessionExpired
	}
	session := publicSession(record.session)
	return Grant{Session: session, CSRFToken: session.csrf}, nil
}

// ValidateCSRF requires the token returned to the browser application for a session-changing call.
func (m *Manager) ValidateCSRF(request *http.Request, session Session) error {
	provided := request.Header.Get("X-CSRF-Token")
	if provided == "" || session.csrf == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(session.csrf)) != 1 {
		return ErrCSRFInvalid
	}
	return nil
}

func (m *Manager) accessUnavailable() error {
	if m.accessObserver != nil {
		m.accessObserver.RecordConsoleAccessUnavailable()
	}
	return ErrAccessUnavailable
}

func (m *Manager) accessVerified() {
	if m.accessObserver != nil {
		m.accessObserver.RecordConsoleAccessVerified()
	}
}

// AuthorizeChat verifies the session principal with the freshness required by intent.
func (m *Manager) AuthorizeChat(ctx context.Context, session Session, chatID int64, intent AccessIntent) error {
	if m.adminChecker == nil || session.Principal.TelegramID <= 0 || chatID == 0 {
		return m.accessUnavailable()
	}
	var allowed bool
	var err error
	switch intent {
	case WriteAccess:
		allowed, err = m.adminChecker.FreshAdmin(ctx, chatID, session.Principal.TelegramID)
	case ReadAccess:
		allowed, err = m.adminChecker.CachedAdmin(ctx, chatID, session.Principal.TelegramID)
	default:
		return m.accessUnavailable()
	}
	if err != nil {
		return m.accessUnavailable()
	}
	m.accessVerified()
	if !allowed {
		return ErrAccessDenied
	}
	return nil
}

// AccessibleChats filters configured groups through bounded cached-positive membership reads.
func (m *Manager) AccessibleChats(ctx context.Context, session Session, candidates []int64) []int64 {
	permitted := make([]bool, len(candidates))
	jobs := make(chan int)
	workerCount := min(len(candidates), accessibleChatsConcurrency)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				permitted[index] = m.AuthorizeChat(ctx, session, candidates[index], ReadAccess) == nil
			}
		}()
	}
	for index := range candidates {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	allowed := make([]int64, 0, len(candidates))
	for index, chatID := range candidates {
		if permitted[index] {
			allowed = append(allowed, chatID)
		}
	}
	return allowed
}

// SetCookies writes the opaque session cookie; the CSRF value is returned in Grant.
func (m *Manager) SetCookies(writer http.ResponseWriter, grant Grant) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: grant.Session.token, Path: "/", Expires: grant.Session.ExpiresAt,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// ClearCookies expires the browser session cookie after an authentication failure.
func (m *Manager) ClearCookies(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", Expires: time.Unix(1, 0), MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func (m *Manager) pruneLocked(now time.Time) {
	for token, record := range m.sessions {
		if !record.session.ExpiresAt.After(now) {
			m.removeSessionLocked(token, record.session.Principal.TelegramID)
		}
	}
	for hash, expiresAt := range m.replayedHashes {
		if !expiresAt.After(now) {
			delete(m.replayedHashes, hash)
		}
	}
	for token, link := range m.links {
		if !link.expiresAt.After(now) {
			delete(m.links, token)
			m.rememberLinkLocked(token, ErrOperatorLinkExpired, now)
		}
	}
	for token, history := range m.linkHistory {
		if !history.expiresAt.After(now) {
			delete(m.linkHistory, token)
		}
	}
}

func (m *Manager) removeSessionLocked(token string, telegramID int64) {
	delete(m.sessions, token)
	if m.sessionByUser[telegramID] == token {
		delete(m.sessionByUser, telegramID)
	}
}

func (m *Manager) rememberLinkLocked(token string, err error, now time.Time) {
	if _, known := m.linkHistory[token]; !known && len(m.linkHistory) >= m.maxEntries {
		for oldest := range m.linkHistory {
			delete(m.linkHistory, oldest)
			break
		}
	}
	m.linkHistory[token] = linkTombstone{err: err, expiresAt: now.Add(m.sessionTTL)}
}

func credentialPair() (string, string, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", err
	}
	return token, csrf, nil
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
