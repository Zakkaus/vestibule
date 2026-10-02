package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// httpStatusError preserves authoritative statuses such as 404 across the shared transport.
type httpStatusError struct {
	Url  string
	Code int
}

// Error describes a response that exceeded its parser limit.
func (e *httpStatusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.Url, e.Code)
}

// httpBusyError marks local saturation as a temporary lookup failure.
type httpBusyError struct {
	Url  string
	Wait time.Duration
}

// Error describes a response that exceeded its parser limit.
func (e *httpBusyError) Error() string {
	return fmt.Sprintf("GET %s: outbound HTTP limit busy for %s", e.Url, e.Wait)
}

// httpBodyTooLargeError prevents parsers from treating a valid-looking prefix as a complete reply.
type httpBodyTooLargeError struct {
	Url   string
	Limit int64
}

// Error describes a response that exceeded its parser limit.
func (e *httpBodyTooLargeError) Error() string {
	return fmt.Sprintf("GET %s: response body exceeds %d bytes", e.Url, e.Limit)
}

// httpStatusCode returns zero for failures without an HTTP response.
func httpStatusCode(err error) int {
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return 0
}

var httpClient = &http.Client{Timeout: 25 * time.Second}

// An unscoped GITHUB_TOKEN raises the public API limit.
var githubToken string

// Service owns lookup handlers and their private-query rate state.
type Service struct {
	settings *settings.Store

	cfg       *settings.Config
	mu        sync.Mutex
	queryHits map[int64][]time.Time
	warmOnce  sync.Once
	warmCtx   context.Context
	warmStop  context.CancelFunc
	warmDone  chan struct{}
}

// New constructs a lookup service from runtime settings, Telegram transport, configuration, and an optional GitHub token.
func New(store *settings.Store, cfg *settings.Config, githubAPIToken string) *Service {
	if cfg == nil {
		cfg = &settings.Config{}
	}
	configurePkg(cfg)
	configureFeedSources(cfg)
	githubToken = githubAPIToken
	warmCtx, warmStop := context.WithCancel(context.Background())
	return &Service{
		settings: store,

		cfg:       cfg,
		queryHits: map[int64][]time.Time{},
		warmCtx:   warmCtx,
		warmStop:  warmStop,
		warmDone:  make(chan struct{}),
	}
}

// Warm refreshes the package-search cache unless it is already fresh or refreshing.
func (s *Service) Warm(ctx context.Context) {
	PkgC.Refresh(ctx)
}

// DemandWarm starts the Gentoo package-cache warm-up on the first demand and is a no-op
// after that; it returns at once so a command handler is not held by the fetch. The
// warm-up runs under the service's own context rather than the caller's so a request
// ending does not abort it and Shutdown can.
func (s *Service) DemandWarm() {
	if s == nil {
		return
	}
	s.warmOnce.Do(func() {
		go func() {
			defer close(s.warmDone)
			s.Warm(s.warmCtx)
		}()
	})
}

// Shutdown aborts a running warm-up and waits for it to finish, or for ctx to expire.
// The package-search cache and its sources are package state; a warm-up left running
// past the service's lifetime would read them while the next service writes them.
func (s *Service) Shutdown(ctx context.Context) {
	if s == nil {
		return
	}
	s.warmStop()
	s.warmOnce.Do(func() { close(s.warmDone) })
	select {
	case <-s.warmDone:
	case <-ctx.Done():
	}
}

// AutoDelete returns the effective lookup cleanup duration and enabled state for one group.
func (s *Service) AutoDelete(groupID int64) (time.Duration, bool) {
	if s.settings != nil {
		if group, ok := s.settings.Settings(groupID); ok {
			duration, valid := settings.SecondsToDuration(group.LookupTTLSeconds().Value)
			return duration, group.LookupAutoDeleteEnabled().Value && valid
		}
	}
	seconds := 180
	if s.cfg.LookupTTLSeconds != nil {
		seconds = max(*s.cfg.LookupTTLSeconds, 0)
	}
	duration, valid := settings.SecondsToDuration(seconds)
	return duration, seconds > 0 && valid
}

func (s *Service) IsGroup(groupID int64) bool {
	if s.settings != nil {
		return s.settings.IsGroup(groupID)
	}
	return s.cfg.IsGroup(groupID)
}

func (s *Service) lookupSettingsGroupID(chatID int64) int64 {
	if s.settings != nil && s.settings.IsGroup(chatID) {
		return chatID
	}
	return 0
}

func (s *Service) CleanupAfter(chatID int64) time.Duration {
	ttl, on := s.AutoDelete(s.lookupSettingsGroupID(chatID))
	if !on {
		return 0
	}
	return ttl
}

const privateQueryWindow = time.Minute

const privateQueryMapMax = 10000

func (s *Service) PrivateQueryPerMin() int {
	if s.cfg.PrivateQueryPerMin > 0 {
		return s.cfg.PrivateQueryPerMin
	}
	return 3
}

func (s *Service) RichEnabled(chatID int64) bool {
	if s.settings != nil {
		if group, ok := s.settings.Settings(chatID); ok {
			return group.RichMessages().Value
		}
	}
	return s.cfg.RichMessages
}

func (s *Service) GroupLanguage(groupID int64) i18n.Lang {
	if s.settings != nil {
		if group, ok := s.settings.Settings(groupID); ok {
			return i18n.FromStored(group.Lang().Value)
		}
	}
	return i18n.FromStored(s.cfg.LangForGroup(groupID))
}

// Sliding-window limits apply only to private-chat lookups.
func (s *Service) QueryRateOK(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-privateQueryWindow)
	kept := s.queryHits[userID][:0]
	for _, hit := range s.queryHits[userID] {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= s.PrivateQueryPerMin() {
		s.queryHits[userID] = kept
		return false
	}
	s.queryHits[userID] = append(kept, now)
	if len(s.queryHits) > privateQueryMapMax {
		for userID, hits := range s.queryHits {
			if len(hits) == 0 || !hits[len(hits)-1].After(cutoff) {
				delete(s.queryHits, userID)
			}
		}
		if len(s.queryHits) > privateQueryMapMax {
			s.queryHits = map[int64][]time.Time{}
		}
	}
	return true
}

// Bound JSON memory while accommodating recursive GitHub trees.
const maxJSONBytes = 32 << 20

// Every lookup and feed request shares this concurrency bound until its body closes.
const httpMaxConcurrent = 24

// Brief queueing absorbs normal fan-out without parking handlers behind 25-second requests.
const httpSlotWait = 2 * time.Second

var httpSem = make(chan struct{}, httpMaxConcurrent)

// semReleaseCloser releases its outbound slot exactly once.
type semReleaseCloser struct {
	io.ReadCloser
	once sync.Once
}

// Close releases the response body and its outbound slot exactly once.
func (s *semReleaseCloser) Close() error {
	err := s.ReadCloser.Close()
	s.once.Do(func() { <-httpSem })
	return err
}

// Saturation returns a typed temporary error instead of queueing without bound.
func acquireHTTPSlot(ctx context.Context, url string, sem chan struct{}, wait time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case sem <- struct{}{}:
		return nil
	default:
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return &httpBusyError{Url: url, Wait: wait}
	}
}

// httpGet returns only HTTP 200 responses; callers must close the body.
func httpGet(ctx context.Context, url string, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, vs := range hdr {
		for _, val := range vs {
			req.Header.Add(k, val)
		}
	}
	if err := acquireHTTPSlot(ctx, url, httpSem, httpSlotWait); err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		<-httpSem
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close() // discarding a non-200 body; close error is irrelevant (slot freed below)
		<-httpSem
		return nil, &httpStatusError{Url: url, Code: resp.StatusCode}
	}
	resp.Body = &semReleaseCloser{ReadCloser: resp.Body} // slot released when the caller closes the body
	return resp, nil
}

// GetJSON fetches a 200 JSON response into dst with the shared body and concurrency limits.
func GetJSON(ctx context.Context, url string, hdr http.Header, dst any) error {
	resp, err := httpGet(ctx, url, hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst)
}

// Reading one extra byte prevents a truncated prefix from reaching a parser.
func httpGetBody(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &httpBodyTooLargeError{Url: url, Limit: limit}
	}
	return body, nil
}
